// Package headless is the reference driver required by Phase 0 (§8).
//
// It has no window system at all. Its value is that every measurement is
// deterministic and every ApplyLayout batch is recorded, so the layout pipeline
// can be golden-tested in CI on any machine — this is the main self-check tool
// for an LLM changing layout code (§9.1).
package headless

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/goWidgets/core"
)

func init() { core.RegisterDriver("headless", func() core.PlatformDriver { return &driver{} }) }

// Metrics are the fixed, documented numbers the fake platform reports. Real
// backends read them from theme and font metrics; here they are constants so a
// golden file means the same thing on every machine.
const (
	CharWidth   = 7.0  // DIP per character
	LineHeight  = 16.0 // DIP per text line
	LabelPadY   = 4.0
	ButtonPadX  = 16.0
	ButtonPadY  = 10.0
	CheckBoxBox = 18.0 // the indicator itself
	CheckBoxGap = 6.0
	EntryW      = 160.0 // natural width of a text field: a choice, not a measurement
	EntryMinW   = 40.0
	EntryPadY   = 4.0
	DefaultW    = 640.0
	DefaultH    = 480.0
	DefaultDPIS = 1.0
)

type driver struct {
	win  *window
	wake chan struct{}
}

func (d *driver) Name() string { return "headless" }

func (d *driver) Init() error {
	d.wake = make(chan struct{}, 1)
	return nil
}

func (d *driver) Capabilities() core.Caps {
	return core.Caps{NativeControls: false, MaxCallbacks: 1 << 30}
}

func (d *driver) CreateWindow(spec core.WindowSpec) (core.BackendWindow, error) {
	if spec.Size.W == 0 {
		spec.Size = core.Size{W: DefaultW, H: DefaultH}
	}
	w := &window{
		drv:    d,
		title:  spec.Title,
		size:   spec.Size,
		nodes:  map[core.Handle]*node{},
		events: make(chan core.BackendEvent, 64),
		nextH:  1,
	}
	w.nodes[w.nextH] = &node{kind: 0, visible: true} // root
	w.root = w.nextH
	d.win = w
	current = w
	// The initial size has to reach core the same way a real resize would.
	w.events <- core.BackendEvent{Kind: core.EventResized, H: w.root, Size: spec.Size}
	return w, nil
}

func (d *driver) RunMainLoop(ctx context.Context, pump func()) error {
	pump()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-d.wake:
			pump()
		}
	}
}

func (d *driver) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// CreateTray: a driver with no window system has no status area either.
func (d *driver) CreateTray(core.TraySpec) (core.BackendTray, error) { return nil, core.ErrNoTray }

func (d *driver) Shutdown() {}

type node struct {
	kind    core.WidgetKind
	text    string
	enabled bool
	visible bool
	checked bool
	parent  core.Handle
}

type window struct {
	drv        *driver
	title      string
	size       core.Size
	root       core.Handle
	nextH      core.Handle
	nodes      map[core.Handle]*node
	events     chan core.BackendEvent
	focused    core.Handle
	textWrites int

	mu       sync.Mutex
	log      []string
	measures int
}

func (w *window) SetTitle(s string)                { w.title = s }
func (w *window) Show()                            {}
func (w *window) Close()                           { close(w.events) }
func (w *window) Scale() core.ScaleInfo            { return core.ScaleInfo{Scale: DefaultDPIS, FontScale: 1} }
func (w *window) RootHandle() core.Handle          { return w.root }
func (w *window) Events() <-chan core.BackendEvent { return w.events }

func (w *window) CreateWidget(kind core.WidgetKind, parent core.Handle) (core.Handle, error) {
	w.nextH++
	h := w.nextH
	w.nodes[h] = &node{kind: kind, parent: parent, enabled: true, visible: true}
	return h, nil
}

func (w *window) DestroyWidget(h core.Handle) { delete(w.nodes, h) }

func (w *window) SetParent(child, parent core.Handle, index int) {
	if n := w.nodes[child]; n != nil {
		n.parent = parent
	}
}

func (w *window) SetString(h core.Handle, p core.PropKey, v string) {
	if n := w.nodes[h]; n != nil && p == core.PropText {
		n.text = v
		w.textWrites++
		// Both real platforms report a programmatic write to a text field as
		// a change (GTK "changed", Win32 EN_CHANGE); so does this one, so the
		// core's handling of that echo is what the tests exercise.
		if n.kind == core.KindEntry {
			select {
			case w.events <- core.BackendEvent{Kind: core.EventTextChanged, H: h, Text: v}:
			default:
			}
		}
	}
}

// TextWrites counts every SetString the core has issued: a platform-side
// edit must not produce one.
func TextWrites() int {
	if current == nil {
		return 0
	}
	return current.textWrites
}

// Focus records the request; Focused reports it.
func (w *window) Focus(h core.Handle) { w.focused = h }

// Focused returns the text of the widget that last asked for focus, and
// whether any did.
func Focused() (text string, ok bool) {
	if current == nil || current.focused == 0 {
		return "", false
	}
	if n := current.nodes[current.focused]; n != nil {
		return n.text, true
	}
	return "", false
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropEnabled:
		n.enabled = v
	case core.PropVisible:
		n.visible = v
	case core.PropChecked:
		n.checked = v
	}
}

func (w *window) SetFloat(core.Handle, core.PropKey, float64) {}

// MeasureIntrinsic uses the constants above so that a golden file is stable
// across machines, fonts and locales.
func (w *window) MeasureIntrinsic(h core.Handle, avail core.Size) (min, natural core.Size) {
	w.mu.Lock()
	w.measures++
	w.mu.Unlock()
	n := w.nodes[h]
	if n == nil {
		return core.Size{}, core.Size{}
	}
	textW := float64(len([]rune(n.text))) * CharWidth
	switch n.kind {
	case core.KindButton:
		s := core.Size{W: textW + 2*ButtonPadX, H: LineHeight + 2*ButtonPadY}
		return core.Size{W: 2 * ButtonPadX, H: s.H}, s
	case core.KindTextView:
		// A fixed, tall rectangle — the layout gives it a definite height.
		return core.Size{W: 0, H: 120}, core.Size{W: textW, H: 120}
	case core.KindCheckBox:
		s := core.Size{W: CheckBoxBox + CheckBoxGap + textW, H: LineHeight + 2*LabelPadY}
		return core.Size{W: CheckBoxBox, H: s.H}, s
	case core.KindEntry:
		h := LineHeight + 2*EntryPadY
		return core.Size{W: EntryMinW, H: h}, core.Size{W: EntryW, H: h}
	default:
		s := core.Size{W: textW, H: LineHeight + 2*LabelPadY}
		return core.Size{W: 0, H: s.H}, s
	}
}

func (w *window) ApplyLayout(changes []core.BoundsChange) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range changes {
		n := w.nodes[c.H]
		kind := "root"
		text := ""
		if n != nil {
			kind, text = n.kind.String(), n.text
		}
		w.log = append(w.log, fmt.Sprintf("%s(%q) x=%.1f y=%.1f w=%.1f h=%.1f visible=%v",
			kind, text, c.R.X, c.R.Y, c.R.W, c.R.H, c.Visible))
	}
}

// GoldenLog returns every ApplyLayout entry recorded so far, one per line.
func (w *window) GoldenLog() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.log, "\n")
}

// The reference driver keeps a package-level pointer to the window it created.
// That is deliberate: headless exists for tests, and this lets a test reach the
// golden log without goWidgets having to expose Handles, which would leak core
// types into the public API for no other reason.
var current *window

// Golden returns every ApplyLayout entry recorded by the current headless
// window, one per line.
func Golden() string {
	if current == nil {
		return ""
	}
	return current.GoldenLog()
}

// Reset clears the recorded layout log.
func Reset() {
	if current == nil {
		return
	}
	current.mu.Lock()
	current.log = nil
	current.mu.Unlock()
}

// MeasureCount reports how many times core has asked the platform for an
// intrinsic size. It makes "did this change trigger a re-measure?" directly
// assertable instead of inferring it from rectangles that may not move.
func MeasureCount() int {
	if current == nil {
		return 0
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	return current.measures
}

// Resize delivers a window resize the way the platform would, so a test can
// check that the layout follows the window.
func Resize(w, h float64) {
	if current == nil {
		return
	}
	current.size = core.Size{W: w, H: h}
	current.events <- core.BackendEvent{Kind: core.EventResized, H: current.root, Size: current.size}
}

// TypeIntoEntry replaces the contents of the first text field, as if the
// user had typed, and reports whether there is one.
func TypeIntoEntry(text string) bool {
	if current == nil {
		return false
	}
	for h, n := range current.nodes {
		if n.kind == core.KindEntry {
			n.text = text
			current.events <- core.BackendEvent{Kind: core.EventTextChanged, H: h, Text: text}
			return true
		}
	}
	return false
}

// PressEnterInEntry presses Enter in the first text field.
func PressEnterInEntry() bool {
	if current == nil {
		return false
	}
	for h, n := range current.nodes {
		if n.kind == core.KindEntry {
			current.events <- core.BackendEvent{Kind: core.EventActivated, H: h, Text: n.text}
			return true
		}
	}
	return false
}

// EntryText reports what the platform holds in the first text field.
func EntryText() (text string, found bool) {
	if current == nil {
		return "", false
	}
	for _, n := range current.nodes {
		if n.kind == core.KindEntry {
			return n.text, true
		}
	}
	return "", false
}

// ToggleByText flips the first check box whose label matches, as if the user
// had clicked it, and reports whether one was found.
func ToggleByText(text string) bool {
	if current == nil {
		return false
	}
	for h, n := range current.nodes {
		if n.text == text && n.kind == core.KindCheckBox {
			n.checked = !n.checked
			current.events <- core.BackendEvent{
				Kind: core.EventToggled, H: h, Bool: n.checked,
			}
			return true
		}
	}
	return false
}

// CheckedByText reports the stored state of a check box, so a test can verify
// that a property change really reached the platform.
func CheckedByText(text string) (checked, found bool) {
	if current == nil {
		return false, false
	}
	for _, n := range current.nodes {
		if n.text == text && n.kind == core.KindCheckBox {
			return n.checked, true
		}
	}
	return false, false
}

// ClickByText injects a click on the first widget whose text matches, as if the
// user had pressed it. It reports whether such a widget was found.
func ClickByText(text string) bool {
	if current == nil {
		return false
	}
	for h, n := range current.nodes {
		if n.text == text {
			current.events <- core.BackendEvent{Kind: core.EventClicked, H: h}
			return true
		}
	}
	return false
}

// Embedded: there is no panel here, and never will be.
func (noTray) Embedded() bool { return false }

// noTray exists only to keep the BackendTray interface satisfiable in tests
// that construct one; CreateTray on this driver always fails.
type noTray struct{}

func (noTray) SetTooltip(string)                {}
func (noTray) SetMenu([]core.MenuItem)          {}
func (noTray) Destroy()                         {}
func (noTray) Events() <-chan core.BackendEvent { return nil }
