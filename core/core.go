package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/unxed/goWidgets/vreactive"
)

// Node is the internal representation of a widget (never exposed to users).
type Node struct {
	H        Handle
	Kind     WidgetKind
	Parent   Handle
	Children []Handle

	Visible bool
	Bounds  Rect

	// Cached measurement, invalidated by dirtyMeasure.
	minSize, natSize Size
	measured         bool
	// PrefHeight, when >0, overrides the measured height. A text view has no
	// natural height worth having — it is a viewport whose size the caller
	// chooses — so this is how a log area gets to be tall.
	PrefHeight float64

	OnClicked func()
	OnToggled func(bool)
}

// registered drivers, filled by backend packages through init().
var (
	regMu    sync.Mutex
	registry = map[string]func() PlatformDriver{}
)

// RegisterDriver is called from a backend's init(). Later duplicate
// registrations of the same name are ignored, so a build-tagged backend can be
// linked twice without exploding.
func RegisterDriver(name string, mk func() PlatformDriver) {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[name]; !dup {
		registry[name] = mk
	}
}

// DefaultOrder is the per-platform fallback chain (§3.3). Backends append
// themselves; the list is filtered against what is actually registered.
var DefaultOrder = []string{"win32", "cocoa", "web", "gtk", "ebiten", "headless"}

// App is the engine instance: one driver, one window tree, one update queue.
type App struct {
	driver PlatformDriver
	win    BackendWindow
	info   DriverInfo

	nodes  map[Handle]*Node
	nextH  Handle
	root   Handle
	scope  *vreactive.Scope
	layout func(*App) []BoundsChange

	queueMu sync.Mutex
	queue   []func()

	dirtyLayout bool
	cancel      context.CancelFunc
	onClose     closeHandler
	onKey       func(KeyEvent)
	tray        BackendTray
}

// ErrNoDriver reports that no backend could be initialised.
var ErrNoDriver = errors.New("core: no usable platform driver")

// NewApp selects a driver per §3.3 and initialises it.
//
// Selection order: the goWidgets_BACKEND environment variable (a hard choice —
// failure is an error, never a silent fallback), then DefaultOrder.
func NewApp() (*App, error) {
	runtime.LockOSThread()
	vreactive.BindMainThread()

	a := &App{
		nodes: map[Handle]*Node{},
		scope: vreactive.NewScope(),
	}
	a.layout = stackLayout

	regMu.Lock()
	defer regMu.Unlock()

	if forced := os.Getenv("goWidgets_BACKEND"); forced != "" {
		mk, ok := registry[forced]
		if !ok {
			return nil, fmt.Errorf("%w: goWidgets_BACKEND=%q is not linked into this binary", ErrNoDriver, forced)
		}
		d := mk()
		a.info.Attempts = append(a.info.Attempts, "forced "+forced)
		if err := d.Init(); err != nil {
			return nil, fmt.Errorf("%w: forced driver %q: %v", ErrNoDriver, forced, err)
		}
		a.adopt(d)
		return a, nil
	}

	for _, name := range DefaultOrder {
		mk, ok := registry[name]
		if !ok {
			continue
		}
		d := mk()
		if err := d.Init(); err != nil {
			a.info.Attempts = append(a.info.Attempts, name+": "+err.Error())
			continue
		}
		a.info.Attempts = append(a.info.Attempts, name+": ok")
		a.adopt(d)
		return a, nil
	}
	return nil, fmt.Errorf("%w (tried: %v)", ErrNoDriver, a.info.Attempts)
}

func (a *App) adopt(d PlatformDriver) {
	a.driver = d
	a.info.Name = d.Name()
	a.info.Caps = d.Capabilities()
}

// Diagnostics reports which driver won and what was tried (§3.3.4).
func (a *App) Diagnostics() DriverInfo { return a.info }

// Scope is the application-wide subscription owner.
func (a *App) Scope() *vreactive.Scope { return a.scope }

// OpenWindow creates the single top-level window of iteration 1.
func (a *App) OpenWindow(spec WindowSpec) error {
	w, err := a.driver.CreateWindow(spec)
	if err != nil {
		return err
	}
	a.win = w
	a.root = w.RootHandle()
	// The content area is a node like any other: without it there is no layout
	// parent and stackLayout has nothing to iterate. Bounds are seeded from the
	// spec so the first frame is correct even before the backend's initial
	// resize event arrives.
	a.nodes[a.root] = &Node{
		H:       a.root,
		Visible: true,
		Bounds:  Rect{W: spec.Size.W, H: spec.Size.H},
	}

	// §4.4 hands events over a channel. Reading it from a helper goroutine and
	// re-entering through QueueUpdate keeps the contract intact and still lands
	// every handler on the UI thread (§4.5.2).
	go func() {
		for ev := range w.Events() {
			ev := ev
			a.QueueUpdate(func() { a.dispatch(ev) })
		}
	}()
	return nil
}

// Window exposes the backend window to the public API layer.
func (a *App) Window() BackendWindow { return a.win }

// NewNode creates a widget in the backend and registers it.
func (a *App) NewNode(kind WidgetKind) (*Node, error) {
	h, err := a.win.CreateWidget(kind, a.root)
	if err != nil {
		return nil, err
	}
	n := &Node{H: h, Kind: kind, Parent: a.root, Visible: true}
	a.nodes[h] = n
	root := a.nodes[a.root]
	if root != nil {
		root.Children = append(root.Children, h)
	}
	a.dirtyLayout = true
	return n, nil
}

// Node looks up a registered node.
func (a *App) Node(h Handle) *Node { return a.nodes[h] }

// Invalidate marks the layout dirty; the next frame recomputes it.
func (a *App) Invalidate() {
	a.dirtyLayout = true
	if n := a.nodes[a.root]; n != nil {
		for _, ch := range n.Children {
			if c := a.nodes[ch]; c != nil {
				c.measured = false
			}
		}
	}
	a.driver.Wake()
}

// QueueUpdate schedules f on the UI thread from any goroutine (§4.5.3).
func (a *App) QueueUpdate(f func()) {
	if f == nil {
		return
	}
	a.queueMu.Lock()
	a.queue = append(a.queue, f)
	a.queueMu.Unlock()
	a.driver.Wake()
}

// DrainQueue runs everything QueueUpdate has collected, then advances one
// frame. Backends call this from their main loop, on the UI thread only.
func (a *App) DrainQueue() {
	for {
		a.queueMu.Lock()
		if len(a.queue) == 0 {
			a.queueMu.Unlock()
			break
		}
		f := a.queue[0]
		a.queue = a.queue[1:]
		a.queueMu.Unlock()
		vreactive.Batch(f)
	}
	a.Frame()
}

// PumpOnce advances the pipeline by one step without entering a native loop:
// it runs at most the whole update queue and then one frame. It reports whether
// anything actually happened, so a caller can loop until idle. Used by tests
// and by hosts that own their own event loop.
func (a *App) PumpOnce() bool {
	a.queueMu.Lock()
	pending := len(a.queue)
	a.queueMu.Unlock()
	dirty := a.dirtyLayout
	if pending == 0 && !dirty {
		return false
	}
	a.DrainQueue()
	return true
}

// Frame is the normative pipeline of §6, reduced to what iteration 1 needs:
// drain → (re)measure → solve → apply. Nothing happens when nothing is dirty,
// which is what keeps idle CPU at zero.
func (a *App) Frame() {
	if !a.dirtyLayout || a.win == nil {
		return
	}
	a.dirtyLayout = false
	changes := a.layout(a)
	if len(changes) > 0 {
		a.win.ApplyLayout(changes)
	}
}

func (a *App) dispatch(ev BackendEvent) {
	switch ev.Kind {
	case EventClicked:
		if n := a.nodes[ev.H]; n != nil && n.OnClicked != nil {
			n.OnClicked()
		}
	case EventToggled:
		if n := a.nodes[ev.H]; n != nil && n.OnToggled != nil {
			n.OnToggled(ev.Bool)
		}
	case EventResized:
		if n := a.nodes[a.root]; n != nil {
			n.Bounds = Rect{W: ev.Size.W, H: ev.Size.H}
		}
		a.dirtyLayout = true
	case EventScaleChanged:
		for _, n := range a.nodes {
			n.measured = false
		}
		a.dirtyLayout = true
	case EventKey:
		if a.onKey != nil {
			a.onKey(ev.Key)
		}
	case EventCloseRequested:
		if a.onClose != nil && !a.onClose() {
			return
		}
		a.Quit()
	}
}

// Run enters the driver's main loop.
func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.dirtyLayout = true
	defer a.driver.Shutdown()
	return a.driver.RunMainLoop(ctx, a.DrainQueue)
}

// Quit leaves the main loop.
func (a *App) Quit() {
	if a.cancel != nil {
		a.cancel()
	}
	a.driver.Wake()
}

// closeHandler vetoes a window close by returning false.
type closeHandler func() bool

// SetCloseHandler installs the veto handler used by Window.Closing (§4.3).
func (a *App) SetCloseHandler(h closeHandler) { a.onClose = h }

// stackLayout is the Phase-1 layout: a vertical stack of full-width rows, each
// at its natural height, 8 DIP padding. Cassowary replaces this in Phase 2; the
// signature is already the one the solver will use.
func stackLayout(a *App) []BoundsChange {
	root := a.nodes[a.root]
	if root == nil {
		return nil
	}
	const pad = 8.0
	avail := Size{W: root.Bounds.W - 2*pad, H: root.Bounds.H}
	if avail.W < 0 {
		avail.W = 0
	}

	var changes []BoundsChange
	y := pad
	for _, ch := range root.Children {
		n := a.nodes[ch]
		if n == nil {
			continue
		}
		if !n.Visible {
			// A hidden row leaves the flow entirely rather than holding an
			// empty gap, which is what callers expect from Visible=false.
			if r := (Rect{}); r != n.Bounds {
				n.Bounds = r
				changes = append(changes, BoundsChange{H: n.H, R: r, Visible: false})
			}
			continue
		}
		if !n.measured {
			n.minSize, n.natSize = a.win.MeasureIntrinsic(n.H, avail)
			n.measured = true
		}
		height := n.natSize.H
		if n.PrefHeight > 0 {
			height = n.PrefHeight
		}
		r := Rect{X: pad, Y: y, W: avail.W, H: height}
		if r != n.Bounds {
			n.Bounds = r
			changes = append(changes, BoundsChange{H: n.H, R: r, Visible: n.Visible})
		}
		y += height + pad
	}
	return changes
}

// OpenTray creates the status-area icon and starts routing its events.
func (a *App) OpenTray(tooltip string, menu []MenuItem, on func(EventKind, Handle)) error {
	t, err := a.driver.CreateTray(TraySpec{Tooltip: tooltip, Menu: menu})
	if err != nil {
		return err
	}
	a.tray = t

	// Same shape as window events: read on a helper goroutine, re-enter through
	// QueueUpdate so every handler still runs on the UI thread (§4.5.2).
	go func() {
		for ev := range t.Events() {
			ev := ev
			a.QueueUpdate(func() { on(ev.Kind, ev.H) })
		}
	}()
	return nil
}

// SetTrayTooltip updates the status-area tooltip, if there is an icon.
func (a *App) SetTrayTooltip(s string) {
	if a.tray != nil {
		a.tray.SetTooltip(s)
	}
}

// SetTrayMenu replaces the status-area menu, if there is an icon.
func (a *App) SetTrayMenu(items []MenuItem) {
	if a.tray != nil {
		a.tray.SetMenu(items)
	}
}

// TrayEmbedded reports whether a panel accepted the status-area icon.
func (a *App) TrayEmbedded() bool { return a.tray != nil && a.tray.Embedded() }

// SetPrefHeight fixes a node's height, overriding measurement. Used for a text
// view, whose height is a choice, not a property of its content.
func (a *App) SetPrefHeight(n *Node, h float64) {
	n.PrefHeight = h
	a.Invalidate()
}

// SetKeyHandler installs the handler for keystrokes on the main window.
func (a *App) SetKeyHandler(fn func(KeyEvent)) { a.onKey = fn }
