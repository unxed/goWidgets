//go:build linux

// Package gtk is the GTK 3 driver.
//
// Every symbol is resolved at run time out of the system GTK; nothing is linked
// into the binary, so the executable stays CGO_ENABLED=0 and still starts on a
// machine without GTK — Init just fails and core falls through to the next
// driver (§3.3).
//
// Layout note (§2.3): the container is GtkFixed, not GtkBox. Core owns layout
// and hands down absolute rectangles; using a native sizer here would put two
// layout engines in charge of the same widgets.
package gtk

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets/core"
)

func init() { core.RegisterDriver("gtk", func() core.PlatformDriver { return &driver{} }) }

var (
	gtkInitCheck    func(argc, argv uintptr) int32
	gtkMain         func()
	gtkMainQuit     func()
	gtkWindowNew    func(kind int32) uintptr
	gtkWinSetTitle  func(win uintptr, title string)
	gtkWinSetSize   func(win uintptr, w, h int32)
	gtkContAdd      func(container, child uintptr)
	gtkFixedNew     func() uintptr
	gtkFixedPut     func(fixed, child uintptr, x, y int32)
	gtkFixedMove    func(fixed, child uintptr, x, y int32)
	gtkSetSizeReq   func(w uintptr, width, height int32)
	gtkButtonNew    func(label string) uintptr
	gtkButtonLabel  func(button uintptr, label string)
	gtkCheckNew     func(label string) uintptr
	gtkToggleGet    func(w uintptr) int32
	gtkToggleSet    func(w uintptr, active int32)
	gtkLabelNew     func(text string) uintptr
	gtkLabelText    func(label uintptr, text string)
	gtkLabelXAlign  func(label uintptr, x float32)
	gtkTextViewNew  func() uintptr
	gtkTextViewBuf  func(tv uintptr) uintptr
	gtkTextViewEdit func(tv uintptr, editable int32)
	gtkTextViewWrap func(tv uintptr, mode int32)
	gtkTextBufSet   func(buf uintptr, text string, length int32)
	gtkScrollNew    func(h, v uintptr) uintptr
	gtkScrollPolicy func(sw uintptr, h, v int32)
	gtkTextViewMono func(tv uintptr, mono int32)
	gtkWidgetShow   func(w uintptr)
	gtkWidgetHide   func(w uintptr)
	gtkWidgetDestr  func(w uintptr)
	gtkSetSensit    func(w uintptr, sensitive int32)
	gtkPreferred    func(w uintptr, min, nat unsafe.Pointer)
	gtkScaleFactor  func(w uintptr) int32
	gtkAllocW       func(w uintptr) int32
	gtkAllocH       func(w uintptr) int32
	gSignalConnect  func(inst uintptr, sig string, handler, data, destroy uintptr, flags uint32) uint64
	gIdleAdd        func(fn, data uintptr) uint32
)

// requisition mirrors GtkRequisition.
type requisition struct{ Width, Height int32 }

const windowToplevel = 0

type driver struct {
	win *window

	cbClicked      uintptr
	cbToggled      uintptr
	cbKeyPress     uintptr
	cbKeyRelease   uintptr
	cbTrayActivate uintptr
	cbTrayPopup    uintptr
	cbMenuItem     uintptr
	cbDelete       uintptr
	cbAlloc        uintptr
	cbWake         uintptr
	cbQuit         uintptr
	pump           func()
	pumpMu         sync.Mutex
	initedOnce     bool
}

func (d *driver) Name() string { return "gtk" }

func (d *driver) Capabilities() core.Caps {
	return core.Caps{
		NativeControls:  true,
		Clipboard:       true,
		FileDialog:      true,
		Menus:           true,
		SmoothAnimation: true,
		TrayIcon:        true, // GtkStatusIcon; see the note in tray_linux.go
		MaxCallbacks:    2000, // purego's callback pool
	}
}

func dlopenAny(names ...string) (uintptr, error) {
	var last error
	for _, n := range names {
		h, err := purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		last = err
	}
	return 0, last
}

func (d *driver) Init() error {
	if d.initedOnce {
		return nil
	}
	lib, err := dlopenAny("libgtk-3.so.0", "libgtk-3.so")
	if err != nil {
		return fmt.Errorf("GTK 3 not available: %w", err)
	}
	gobj, err := dlopenAny("libgobject-2.0.so.0", "libgobject-2.0.so")
	if err != nil {
		return fmt.Errorf("GObject not available: %w", err)
	}
	glib, err := dlopenAny("libglib-2.0.so.0", "libglib-2.0.so")
	if err != nil {
		return fmt.Errorf("GLib not available: %w", err)
	}

	purego.RegisterLibFunc(&gtkInitCheck, lib, "gtk_init_check")
	purego.RegisterLibFunc(&gtkMain, lib, "gtk_main")
	purego.RegisterLibFunc(&gtkMainQuit, lib, "gtk_main_quit")
	purego.RegisterLibFunc(&gtkWindowNew, lib, "gtk_window_new")
	purego.RegisterLibFunc(&gtkWinSetTitle, lib, "gtk_window_set_title")
	purego.RegisterLibFunc(&gtkWinSetSize, lib, "gtk_window_set_default_size")
	purego.RegisterLibFunc(&gtkContAdd, lib, "gtk_container_add")
	purego.RegisterLibFunc(&gtkFixedNew, lib, "gtk_fixed_new")
	purego.RegisterLibFunc(&gtkFixedPut, lib, "gtk_fixed_put")
	purego.RegisterLibFunc(&gtkFixedMove, lib, "gtk_fixed_move")
	purego.RegisterLibFunc(&gtkSetSizeReq, lib, "gtk_widget_set_size_request")
	purego.RegisterLibFunc(&gtkButtonNew, lib, "gtk_button_new_with_label")
	purego.RegisterLibFunc(&gtkButtonLabel, lib, "gtk_button_set_label")
	purego.RegisterLibFunc(&gtkCheckNew, lib, "gtk_check_button_new_with_label")
	purego.RegisterLibFunc(&gtkToggleGet, lib, "gtk_toggle_button_get_active")
	purego.RegisterLibFunc(&gtkToggleSet, lib, "gtk_toggle_button_set_active")
	purego.RegisterLibFunc(&gtkLabelNew, lib, "gtk_label_new")
	purego.RegisterLibFunc(&gtkLabelText, lib, "gtk_label_set_text")
	purego.RegisterLibFunc(&gtkLabelXAlign, lib, "gtk_label_set_xalign")
	purego.RegisterLibFunc(&gtkTextViewNew, lib, "gtk_text_view_new")
	purego.RegisterLibFunc(&gtkTextViewBuf, lib, "gtk_text_view_get_buffer")
	purego.RegisterLibFunc(&gtkTextViewEdit, lib, "gtk_text_view_set_editable")
	purego.RegisterLibFunc(&gtkTextViewWrap, lib, "gtk_text_view_set_wrap_mode")
	purego.RegisterLibFunc(&gtkTextBufSet, lib, "gtk_text_buffer_set_text")
	purego.RegisterLibFunc(&gtkScrollNew, lib, "gtk_scrolled_window_new")
	purego.RegisterLibFunc(&gtkScrollPolicy, lib, "gtk_scrolled_window_set_policy")
	purego.RegisterLibFunc(&gtkTextViewMono, lib, "gtk_text_view_set_monospace")
	purego.RegisterLibFunc(&gtkWidgetShow, lib, "gtk_widget_show")
	purego.RegisterLibFunc(&gtkWidgetHide, lib, "gtk_widget_hide")
	purego.RegisterLibFunc(&gtkWidgetDestr, lib, "gtk_widget_destroy")
	purego.RegisterLibFunc(&gtkSetSensit, lib, "gtk_widget_set_sensitive")
	purego.RegisterLibFunc(&gtkPreferred, lib, "gtk_widget_get_preferred_size")
	purego.RegisterLibFunc(&gtkScaleFactor, lib, "gtk_widget_get_scale_factor")
	purego.RegisterLibFunc(&gtkAllocW, lib, "gtk_widget_get_allocated_width")
	purego.RegisterLibFunc(&gtkAllocH, lib, "gtk_widget_get_allocated_height")
	purego.RegisterLibFunc(&gSignalConnect, gobj, "g_signal_connect_data")
	purego.RegisterLibFunc(&gIdleAdd, glib, "g_idle_add")
	d.registerTray(lib, gobj)

	if gtkInitCheck(0, 0) == 0 {
		return fmt.Errorf("gtk_init_check failed (no display?)")
	}

	// A handful of static trampolines serve every widget. Per-widget identity
	// travels in the gpointer `data` slot as an integer token — never a Go
	// pointer, which the runtime forbids handing to C.
	d.cbClicked = purego.NewCallback(func(w, data uintptr) uintptr {
		emit(core.BackendEvent{Kind: core.EventClicked, H: core.Handle(data)})
		return 0
	})
	// The handler signature is gboolean(GtkWidget*, GdkEventKey*, gpointer);
	// returning 0 lets the key travel on to the focused control, so adding a
	// listener never steals typing from a widget.
	d.cbKeyPress = purego.NewCallback(func(widget, event, data uintptr) uintptr {
		d.emitKey(event, true)
		return 0
	})
	d.cbKeyRelease = purego.NewCallback(func(widget, event, data uintptr) uintptr {
		d.emitKey(event, false)
		return 0
	})
	d.cbTrayActivate = purego.NewCallback(func(icon, data uintptr) uintptr {
		trayEmit(core.BackendEvent{Kind: core.EventTrayActivated})
		return 0
	})
	// popup-menu carries (icon, button, activate_time, data).
	d.cbTrayPopup = purego.NewCallback(func(icon, button, when, data uintptr) uintptr {
		trayMu.Lock()
		t := trayCurrent
		trayMu.Unlock()
		if t != nil {
			t.popup()
		}
		return 0
	})
	d.cbMenuItem = purego.NewCallback(func(item, data uintptr) uintptr {
		trayEmit(core.BackendEvent{Kind: core.EventMenuItem, H: core.Handle(data)})
		return 0
	})
	d.cbToggled = purego.NewCallback(func(w, data uintptr) uintptr {
		emit(core.BackendEvent{
			Kind: core.EventToggled,
			H:    core.Handle(data),
			Bool: gtkToggleGet(w) != 0,
		})
		return 0
	})
	d.cbDelete = purego.NewCallback(func(w, ev, data uintptr) uintptr {
		emit(core.BackendEvent{Kind: core.EventCloseRequested})
		return 1 // TRUE: never destroy on our own; core decides and calls Close
	})
	// The GtkAllocation pointer is ignored on purpose: dereferencing a uintptr
	// that came from C means converting it to a Go pointer, which go vet flags
	// and which would be genuinely unsafe if the struct ever held pointers.
	// GTK's own accessors carry the same information.
	d.cbAlloc = purego.NewCallback(func(w, allocPtr, data uintptr) uintptr {
		emit(core.BackendEvent{
			Kind: core.EventResized,
			Size: core.Size{W: float64(gtkAllocW(w)), H: float64(gtkAllocH(w))},
		})
		return 0
	})
	d.cbWake = purego.NewCallback(func(data uintptr) uintptr {
		d.pumpMu.Lock()
		p := d.pump
		d.pumpMu.Unlock()
		if p != nil {
			p()
		}
		return 0 // G_SOURCE_REMOVE
	})
	d.cbQuit = purego.NewCallback(func(data uintptr) uintptr {
		gtkMainQuit()
		return 0
	})

	d.initedOnce = true
	return nil
}

// events is package-level because GTK callbacks carry no Go context; there is
// exactly one driver instance per process.
var (
	evMu     sync.Mutex
	evTarget chan core.BackendEvent
)

func emit(ev core.BackendEvent) {
	evMu.Lock()
	ch := evTarget
	evMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- ev:
	default: // never block the UI thread on a slow consumer
	}
}

func (d *driver) RunMainLoop(ctx context.Context, pump func()) error {
	d.pumpMu.Lock()
	d.pump = pump
	d.pumpMu.Unlock()

	pump()
	go func() {
		<-ctx.Done()
		gIdleAdd(d.cbQuit, 0) // g_idle_add is thread-safe; gtk_main_quit is not
	}()
	gtkMain()
	return nil
}

// Wake is called from arbitrary goroutines. g_idle_add is one of the few GLib
// entry points documented as thread-safe, which is why Post/QueueUpdate is
// built on it.
func (d *driver) Wake() {
	if d.cbWake != 0 {
		gIdleAdd(d.cbWake, 0)
	}
}

// emitKey turns a GdkEventKey into a backend event on the current window.
func (d *driver) emitKey(event uintptr, down bool) {
	vk, ch, state, ok := keyFromGdk(event, down)
	if !ok {
		return
	}
	emit(core.BackendEvent{
		Kind: core.EventKey,
		Key: core.KeyEvent{
			VirtualKeyCode:  vk,
			Char:            ch,
			KeyDown:         down,
			ControlKeyState: state,
			RepeatCount:     1,
		},
	})
}

func (d *driver) Shutdown() {}

func (d *driver) CreateWindow(spec core.WindowSpec) (core.BackendWindow, error) {
	h := gtkWindowNew(windowToplevel)
	if h == 0 {
		return nil, fmt.Errorf("gtk_window_new returned NULL")
	}
	gtkWinSetTitle(h, spec.Title)
	gtkWinSetSize(h, int32(spec.Size.W), int32(spec.Size.H))

	fixed := gtkFixedNew()
	gtkContAdd(h, fixed)

	w := &window{
		drv:    d,
		handle: h,
		fixed:  fixed,
		nodes:  map[core.Handle]*node{},
		events: make(chan core.BackendEvent, 128),
		nextH:  1,
	}
	w.root = w.nextH
	w.nodes[w.root] = &node{handle: fixed}

	evMu.Lock()
	evTarget = w.events
	evMu.Unlock()

	gSignalConnect(h, "delete-event", d.cbDelete, 0, 0, 0)
	gSignalConnect(h, "key-press-event", d.cbKeyPress, 0, 0, 0)
	gSignalConnect(h, "key-release-event", d.cbKeyRelease, 0, 0, 0)
	gSignalConnect(fixed, "size-allocate", d.cbAlloc, 0, 0, 0)

	gtkWidgetShow(fixed)
	d.win = w
	return w, nil
}

type node struct {
	handle uintptr // the widget placed in the layout (scrolled window for a text view)
	inner  uintptr // the text view itself, when different from handle
	kind   core.WidgetKind
}

type window struct {
	drv    *driver
	handle uintptr
	fixed  uintptr
	root   core.Handle
	nextH  core.Handle
	nodes  map[core.Handle]*node
	events chan core.BackendEvent
}

func (w *window) SetTitle(s string)                { gtkWinSetTitle(w.handle, s) }
func (w *window) Show()                            { gtkWidgetShow(w.handle) }
func (w *window) Close()                           { gtkWidgetHide(w.handle) }
func (w *window) RootHandle() core.Handle          { return w.root }
func (w *window) Events() <-chan core.BackendEvent { return w.events }

// Scale: GTK already works in logical pixels, so DIP and GTK coordinates
// coincide and the backend does not rescale. The factor is reported for users
// who need to know the physical density.
func (w *window) Scale() core.ScaleInfo {
	s := float64(gtkScaleFactor(w.handle))
	if s <= 0 {
		s = 1
	}
	return core.ScaleInfo{Scale: s, FontScale: 1}
}

func (w *window) CreateWidget(kind core.WidgetKind, parent core.Handle) (core.Handle, error) {
	w.nextH++
	h := w.nextH

	var g uintptr
	switch kind {
	case core.KindButton:
		g = gtkButtonNew("")
	case core.KindCheckBox:
		g = gtkCheckNew("")
	case core.KindLabel:
		g = gtkLabelNew("")
		gtkLabelXAlign(g, 0) // left-aligned, like every other toolkit's label
	case core.KindTextView:
		// A text view goes inside a scrolled window: the scroller is what the
		// layout places and sizes, the view is what holds the text. Read-only
		// with word wrap, since this shows a log, not an editor.
		// Monospace, and no wrapping. A log is columns — time, source, message —
		// and a proportional font destroys the columns while wrapping destroys
		// the line structure, so the two together make it unreadable. Long
		// lines get a horizontal scrollbar instead of being folded.
		const wrapNone = 0
		tv := gtkTextViewNew()
		gtkTextViewEdit(tv, 0)
		gtkTextViewWrap(tv, wrapNone)
		if gtkTextViewMono != nil {
			gtkTextViewMono(tv, 1)
		}
		sw := gtkScrollNew(0, 0)
		const policyAutomatic = 1
		gtkScrollPolicy(sw, policyAutomatic, policyAutomatic)
		gtkContAdd(sw, tv)
		gtkWidgetShow(tv)
		w.nodes[h] = &node{handle: sw, inner: tv, kind: kind}
		gtkFixedPut(w.fixed, sw, 0, 0)
		gtkWidgetShow(sw)
		return h, nil
	default:
		return 0, fmt.Errorf("gtk: unsupported widget kind %v", kind)
	}
	if g == 0 {
		return 0, fmt.Errorf("gtk: widget constructor returned NULL")
	}
	w.nodes[h] = &node{handle: g, kind: kind}

	switch kind {
	case core.KindButton:
		gSignalConnect(g, "clicked", w.drv.cbClicked, uintptr(h), 0, 0)
	case core.KindCheckBox:
		gSignalConnect(g, "toggled", w.drv.cbToggled, uintptr(h), 0, 0)
	}
	gtkFixedPut(w.fixed, g, 0, 0)
	gtkWidgetShow(g)
	return h, nil
}

func (w *window) DestroyWidget(h core.Handle) {
	if n := w.nodes[h]; n != nil {
		gtkWidgetDestr(n.handle)
		delete(w.nodes, h)
	}
}

func (w *window) SetParent(child, parent core.Handle, index int) {
	// Iteration 1 has a single container; reparenting arrives with Phase 5.
}

func (w *window) SetString(h core.Handle, p core.PropKey, v string) {
	n := w.nodes[h]
	if n == nil || p != core.PropText {
		return
	}
	switch n.kind {
	case core.KindTextView:
		gtkTextBufSet(gtkTextViewBuf(n.inner), v, -1)
	case core.KindButton, core.KindCheckBox:
		gtkButtonLabel(n.handle, v) // GtkCheckButton is a GtkButton subclass
	default:
		gtkLabelText(n.handle, v)
	}
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropEnabled:
		b := int32(0)
		if v {
			b = 1
		}
		gtkSetSensit(n.handle, b)
	case core.PropVisible:
		if v {
			gtkWidgetShow(n.handle)
		} else {
			gtkWidgetHide(n.handle)
		}
	case core.PropChecked:
		if n.kind == core.KindCheckBox {
			b := int32(0)
			if v {
				b = 1
			}
			gtkToggleSet(n.handle, b)
		}
	}
}

func (w *window) SetFloat(core.Handle, core.PropKey, float64) {}

// MeasureIntrinsic asks GTK itself, so theme padding and the user's font are
// respected — the whole reason §5.1 makes this a backend responsibility.
func (w *window) MeasureIntrinsic(h core.Handle, avail core.Size) (min, natural core.Size) {
	n := w.nodes[h]
	if n == nil {
		return core.Size{}, core.Size{}
	}
	var mn, nat requisition
	gtkPreferred(n.handle, unsafe.Pointer(&mn), unsafe.Pointer(&nat))
	return core.Size{W: float64(mn.Width), H: float64(mn.Height)},
		core.Size{W: float64(nat.Width), H: float64(nat.Height)}
}

func (w *window) ApplyLayout(changes []core.BoundsChange) {
	for _, c := range changes {
		n := w.nodes[c.H]
		if n == nil {
			continue
		}
		if !c.Visible {
			gtkWidgetHide(n.handle)
			continue
		}
		gtkSetSizeReq(n.handle, int32(c.R.W), int32(c.R.H))
		gtkFixedMove(w.fixed, n.handle, int32(c.R.X), int32(c.R.Y))
		gtkWidgetShow(n.handle)
	}
}
