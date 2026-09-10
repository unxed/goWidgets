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

func init() {
	core.RegisterDriver("gtk", func() core.PlatformDriver {
		current = &driver{}
		return current
	})
}

// current is the driver most recently created, kept for WindowHandle.
var current *driver

// WindowHandle returns the GtkWindow* of the current window, or 0. It exists
// for tests that inject native events; an application has no use for it.
// Tests that need it live in their own package directory: GTK keeps
// process-global state, and two applications in one test binary crash.
func WindowHandle() uintptr {
	if current == nil || current.win == nil {
		return 0
	}
	return current.win.handle
}

// WidgetHandle returns the GtkWidget* of the first widget of the given kind
// in the current window, or 0. A test hook like WindowHandle.
func WidgetHandle(kind core.WidgetKind) uintptr { return NthWidgetHandle(kind, 0) }

// NthWidgetHandle is WidgetHandle for the i-th widget of the kind.
func NthWidgetHandle(kind core.WidgetKind, i int) uintptr {
	if current == nil || current.win == nil {
		return 0
	}
	for h := core.Handle(1); h <= current.win.nextH; h++ {
		if n := current.win.nodes[h]; n != nil && n.kind == kind {
			if i == 0 {
				return n.handle
			}
			i--
		}
	}
	return 0
}

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
	gtkEntryNew     func() uintptr
	gtkGrabFocus    func(w uintptr)
	gtkDialogNew    func() uintptr
	gtkDialogArea   func(d uintptr) uintptr
	gtkDialogAddBtn func(d uintptr, text string, response int32) uintptr
	gtkDialogDefRsp func(d uintptr, response int32)
	gtkDialogRun    func(d uintptr) int32
	gtkDialogResp   func(d uintptr, response int32)
	gtkWinTransient func(win, parent uintptr)
	gtkWinSetModal  func(win uintptr, modal int32)
	gtkWinResizable func(win uintptr, resizable int32)
	gtkContBorder   func(container uintptr, width uint32)
	gtkLabelWrap    func(label uintptr, wrap int32)
	gDgettext       func(domain, msgid string) string
	gFree           func(p *byte)
	gtkChooserNew   func(action int32) uintptr
	gtkChooserGet   func(chooser uintptr) *byte
	gtkChooserSet   func(chooser uintptr, filename string) int32
	gtkChooserName  func(chooser uintptr, name string)
	gtkChooserFilt  func(chooser uintptr, filter uintptr)
	gtkChooserOverw func(chooser uintptr, confirm int32)
	gtkFilterNew    func() uintptr
	gtkFilterName   func(filter uintptr, name string)
	gtkFilterAddPat func(filter uintptr, pattern string)
	gtkBoxPack      func(box, child uintptr, expand, fill int32, padding uint32)
	gtkComboNew     func() uintptr
	gtkComboNewEnt  func() uintptr
	gtkComboAppend  func(cb uintptr, text string)
	gtkComboClear   func(cb uintptr)
	gtkComboActive  func(cb uintptr) int32
	gtkComboSetAct  func(cb uintptr, index int32)
	gtkComboText    func(cb uintptr) *byte
	gtkListNew      func() uintptr
	gtkListInsert   func(box, child uintptr, position int32)
	gtkListSelect   func(box, row uintptr)
	gtkListUnselect func(box uintptr)
	gtkListRowAt    func(box uintptr, index int32) uintptr
	gtkListRowIndex func(row uintptr) int32
	gtkListSelRow   func(box uintptr) uintptr
	gtkListActivate func(box uintptr, single int32)
	gtkMarginStart  func(w uintptr, margin int32)
	gtkMarginEnd    func(w uintptr, margin int32)
	gtkEntrySetText func(e uintptr, text string)
	gtkEntryGetText func(e uintptr) string
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

const (
	listRows   = 8  // natural height of a list box, in rows
	scrollbarW = 16 // room for the overlay scrollbar beside a list
)

type driver struct {
	win *window

	cbClicked      uintptr
	cbToggled      uintptr
	cbEntryChanged uintptr
	cbComboChanged uintptr
	cbRowSelected  uintptr
	cbRowActivated uintptr
	cbEntryEnter   uintptr
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
	purego.RegisterLibFunc(&gtkEntryNew, lib, "gtk_entry_new")
	purego.RegisterLibFunc(&gtkGrabFocus, lib, "gtk_widget_grab_focus")
	purego.RegisterLibFunc(&gtkDialogNew, lib, "gtk_dialog_new")
	purego.RegisterLibFunc(&gtkDialogArea, lib, "gtk_dialog_get_content_area")
	purego.RegisterLibFunc(&gtkDialogAddBtn, lib, "gtk_dialog_add_button")
	purego.RegisterLibFunc(&gtkDialogDefRsp, lib, "gtk_dialog_set_default_response")
	purego.RegisterLibFunc(&gtkDialogRun, lib, "gtk_dialog_run")
	purego.RegisterLibFunc(&gtkDialogResp, lib, "gtk_dialog_response")
	purego.RegisterLibFunc(&gtkWinTransient, lib, "gtk_window_set_transient_for")
	purego.RegisterLibFunc(&gtkWinSetModal, lib, "gtk_window_set_modal")
	purego.RegisterLibFunc(&gtkWinResizable, lib, "gtk_window_set_resizable")
	purego.RegisterLibFunc(&gtkContBorder, lib, "gtk_container_set_border_width")
	purego.RegisterLibFunc(&gtkLabelWrap, lib, "gtk_label_set_line_wrap")
	purego.RegisterLibFunc(&gDgettext, glib, "g_dgettext")
	purego.RegisterLibFunc(&gFree, glib, "g_free")
	purego.RegisterLibFunc(&gtkChooserNew, lib, "gtk_file_chooser_widget_new")
	purego.RegisterLibFunc(&gtkChooserGet, lib, "gtk_file_chooser_get_filename")
	purego.RegisterLibFunc(&gtkChooserSet, lib, "gtk_file_chooser_set_filename")
	purego.RegisterLibFunc(&gtkChooserName, lib, "gtk_file_chooser_set_current_name")
	purego.RegisterLibFunc(&gtkChooserFilt, lib, "gtk_file_chooser_add_filter")
	purego.RegisterLibFunc(&gtkChooserOverw, lib, "gtk_file_chooser_set_do_overwrite_confirmation")
	purego.RegisterLibFunc(&gtkFilterNew, lib, "gtk_file_filter_new")
	purego.RegisterLibFunc(&gtkFilterName, lib, "gtk_file_filter_set_name")
	purego.RegisterLibFunc(&gtkFilterAddPat, lib, "gtk_file_filter_add_pattern")
	purego.RegisterLibFunc(&gtkBoxPack, lib, "gtk_box_pack_start")
	purego.RegisterLibFunc(&gtkComboNew, lib, "gtk_combo_box_text_new")
	purego.RegisterLibFunc(&gtkComboNewEnt, lib, "gtk_combo_box_text_new_with_entry")
	purego.RegisterLibFunc(&gtkComboAppend, lib, "gtk_combo_box_text_append_text")
	purego.RegisterLibFunc(&gtkComboClear, lib, "gtk_combo_box_text_remove_all")
	purego.RegisterLibFunc(&gtkComboActive, lib, "gtk_combo_box_get_active")
	purego.RegisterLibFunc(&gtkComboSetAct, lib, "gtk_combo_box_set_active")
	purego.RegisterLibFunc(&gtkComboText, lib, "gtk_combo_box_text_get_active_text")
	purego.RegisterLibFunc(&gtkListNew, lib, "gtk_list_box_new")
	purego.RegisterLibFunc(&gtkListInsert, lib, "gtk_list_box_insert")
	purego.RegisterLibFunc(&gtkListSelect, lib, "gtk_list_box_select_row")
	purego.RegisterLibFunc(&gtkListUnselect, lib, "gtk_list_box_unselect_all")
	purego.RegisterLibFunc(&gtkListRowAt, lib, "gtk_list_box_get_row_at_index")
	purego.RegisterLibFunc(&gtkListRowIndex, lib, "gtk_list_box_row_get_index")
	purego.RegisterLibFunc(&gtkListSelRow, lib, "gtk_list_box_get_selected_row")
	purego.RegisterLibFunc(&gtkListActivate, lib, "gtk_list_box_set_activate_on_single_click")
	purego.RegisterLibFunc(&gtkMarginStart, lib, "gtk_widget_set_margin_start")
	purego.RegisterLibFunc(&gtkMarginEnd, lib, "gtk_widget_set_margin_end")
	purego.RegisterLibFunc(&gtkEntrySetText, lib, "gtk_entry_set_text")
	purego.RegisterLibFunc(&gtkEntryGetText, lib, "gtk_entry_get_text")
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
	// The event arrives as a typed pointer: the FFI layer converts the C
	// address itself, so no uintptr→unsafe.Pointer cast is needed here (and
	// vet's unsafeptr check has nothing to object to).
	d.cbKeyPress = purego.NewCallback(func(widget uintptr, event *gdkEventKey, data uintptr) uintptr {
		d.emitKey(event, true)
		return 0
	})
	d.cbKeyRelease = purego.NewCallback(func(widget uintptr, event *gdkEventKey, data uintptr) uintptr {
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
	// GtkEditable "changed" and GtkEntry "activate" both hand over the widget;
	// the text is read back from it, so the event carries the whole field.
	d.cbEntryChanged = purego.NewCallback(func(w, data uintptr) uintptr {
		emit(core.BackendEvent{Kind: core.EventTextChanged, H: core.Handle(data), Text: gtkEntryGetText(w)})
		return 0
	})
	// GtkComboBox "changed" fires for a pick from the list and for typing in
	// the entry alike; the active index tells the two apart (-1 while typing).
	d.cbComboChanged = purego.NewCallback(func(w, data uintptr) uintptr {
		text := cString(gtkComboText(w))
		if i := gtkComboActive(w); i >= 0 {
			emit(core.BackendEvent{Kind: core.EventSelected, H: core.Handle(data), Int: int(i), Text: text})
		} else {
			emit(core.BackendEvent{Kind: core.EventTextChanged, H: core.Handle(data), Text: text})
		}
		return 0
	})
	// GtkListBox hands over the row (NULL when the selection is cleared);
	// the index comes from the row, the text from what was inserted.
	d.cbRowSelected = purego.NewCallback(func(box, row, data uintptr) uintptr {
		h := core.Handle(data)
		i := -1
		if row != 0 {
			i = int(gtkListRowIndex(row))
		}
		emit(core.BackendEvent{Kind: core.EventSelected, H: h, Int: i, Text: d.itemText(h, i)})
		return 0
	})
	d.cbRowActivated = purego.NewCallback(func(box, row, data uintptr) uintptr {
		h := core.Handle(data)
		i := int(gtkListRowIndex(row))
		emit(core.BackendEvent{Kind: core.EventItemActivated, H: h, Int: i, Text: d.itemText(h, i)})
		return 0
	})
	d.cbEntryEnter = purego.NewCallback(func(w, data uintptr) uintptr {
		emit(core.BackendEvent{Kind: core.EventActivated, H: core.Handle(data), Text: gtkEntryGetText(w)})
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
func (d *driver) emitKey(event *gdkEventKey, down bool) {
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
	items  []string // list box rows, for event text
}

// itemText is a list node's i-th item, or "".
func (d *driver) itemText(h core.Handle, i int) string {
	if d.win == nil {
		return ""
	}
	if n := d.win.nodes[h]; n != nil && i >= 0 && i < len(n.items) {
		return n.items[i]
	}
	return ""
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
	case core.KindEdit:
		g = gtkEntryNew()
	case core.KindComboBox:
		g = gtkComboNewEnt() // the dropdown-only variant is swapped in by SetBool
	case core.KindListBox:
		// A list box inside a scrolled window: the scroller is what the
		// layout sizes, the list holds the rows. Selection by click, no
		// activate-on-single-click, so "activated" stays a double-click.
		lb := gtkListNew()
		gtkListActivate(lb, 0)
		gSignalConnect(lb, "row-selected", w.drv.cbRowSelected, uintptr(h), 0, 0)
		gSignalConnect(lb, "row-activated", w.drv.cbRowActivated, uintptr(h), 0, 0)
		sw := gtkScrollNew(0, 0)
		const policyNever, policyAutomatic = 2, 1
		gtkScrollPolicy(sw, policyNever, policyAutomatic)
		gtkContAdd(sw, lb)
		gtkWidgetShow(lb)
		w.nodes[h] = &node{handle: sw, inner: lb, kind: kind}
		gtkFixedPut(w.fixed, sw, 0, 0)
		gtkWidgetShow(sw)
		return h, nil
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
	case core.KindEdit:
		gSignalConnect(g, "changed", w.drv.cbEntryChanged, uintptr(h), 0, 0)
		gSignalConnect(g, "activate", w.drv.cbEntryEnter, uintptr(h), 0, 0)
	case core.KindComboBox:
		gSignalConnect(g, "changed", w.drv.cbComboChanged, uintptr(h), 0, 0)
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
	case core.KindEdit:
		// gtk_entry_set_text emits "changed" only when the text differs, so
		// the property's own write does not come back as an edit.
		gtkEntrySetText(n.handle, v)
	case core.KindComboBox, core.KindListBox:
		// Text is not a property of these; the list holds items.
	default:
		gtkLabelText(n.handle, v)
	}
}

// Dialog builds the box from gtk_dialog_new and gtk_dialog_add_button —
// the non-variadic part of the API; gtk_message_dialog_new takes a printf
// format and varargs, which is not something to push through an FFI. Button
// labels are GTK's own translated ones (g_dgettext on its "gtk30" domain), so
// the box reads like every other GTK dialog on the machine.
//
// gtk_dialog_run spins GTK's loop until a response, so the application's
// queue keeps draining underneath (Wake is g_idle_add).
func (w *window) Dialog(kind core.DialogKind, title, text string) core.DialogResult {
	const (
		responseAccept = 1
		responseReject = 2
	)
	d := gtkDialogNew()
	gtkWinSetTitle(d, title)
	gtkWinTransient(d, w.handle)
	gtkWinSetModal(d, 1)
	gtkWinResizable(d, 0)

	area := gtkDialogArea(d)
	gtkContBorder(area, 12)
	lbl := gtkLabelNew(text)
	gtkLabelWrap(lbl, 1)
	gtkLabelXAlign(lbl, 0)
	gtkContAdd(area, lbl)
	gtkWidgetShow(lbl)

	switch kind {
	case core.DialogConfirm:
		gtkDialogAddBtn(d, gDgettext("gtk30", "_Cancel"), responseReject)
		gtkDialogAddBtn(d, gDgettext("gtk30", "_OK"), responseAccept)
	case core.DialogYesNo:
		gtkDialogAddBtn(d, gDgettext("gtk30", "_No"), responseReject)
		gtkDialogAddBtn(d, gDgettext("gtk30", "_Yes"), responseAccept)
	default:
		gtkDialogAddBtn(d, gDgettext("gtk30", "_OK"), responseAccept)
	}
	gtkDialogDefRsp(d, responseAccept)

	dialogMu.Lock()
	dialogCurrent = d
	dialogMu.Unlock()
	resp := gtkDialogRun(d)
	dialogMu.Lock()
	dialogCurrent = 0
	dialogMu.Unlock()
	gtkWidgetDestr(d)

	// Anything but an explicit accept — reject, Escape, delete-event — is
	// the cautious answer.
	switch kind {
	case core.DialogConfirm:
		if resp == responseAccept {
			return core.DialogOK
		}
		return core.DialogCancel
	case core.DialogYesNo:
		if resp == responseAccept {
			return core.DialogYes
		}
		return core.DialogNo
	}
	return core.DialogOK
}

// FileDialog is a GtkDialog around a GtkFileChooserWidget. The ready-made
// gtk_file_chooser_dialog_new takes its buttons as varargs, so it is built
// here from the non-variadic parts instead; the result is the same GTK file
// chooser, with GTK's own translated Open/Save/Cancel.
func (w *window) FileDialog(save bool, title, suggested string, filters []core.FileFilter) (string, bool) {
	const (
		actionOpen     = 0
		actionSave     = 1
		responseAccept = 1
		responseReject = 2
	)
	d := gtkDialogNew()
	gtkWinSetTitle(d, title)
	gtkWinTransient(d, w.handle)
	gtkWinSetModal(d, 1)
	gtkWinSetSize(d, 720, 520)

	action, accept := int32(actionOpen), "_Open"
	if save {
		action, accept = actionSave, "_Save"
	}
	chooser := gtkChooserNew(action)
	for _, f := range filters {
		ff := gtkFilterNew()
		gtkFilterName(ff, f.Name)
		for _, p := range f.Patterns {
			gtkFilterAddPat(ff, p)
		}
		gtkChooserFilt(chooser, ff) // the chooser takes ownership
	}
	if save {
		gtkChooserOverw(chooser, 1)
		if suggested != "" {
			gtkChooserName(chooser, suggested)
		}
	}
	gtkBoxPack(gtkDialogArea(d), chooser, 1, 1, 0)
	gtkWidgetShow(chooser)

	gtkDialogAddBtn(d, gDgettext("gtk30", "_Cancel"), responseReject)
	gtkDialogAddBtn(d, gDgettext("gtk30", accept), responseAccept)
	gtkDialogDefRsp(d, responseAccept)

	dialogMu.Lock()
	dialogCurrent, chooserCurrent = d, chooser
	dialogMu.Unlock()
	resp := gtkDialogRun(d)
	path := ""
	if resp == responseAccept {
		path = cString(gtkChooserGet(chooser))
	}
	dialogMu.Lock()
	dialogCurrent, chooserCurrent = 0, 0
	dialogMu.Unlock()
	gtkWidgetDestr(d)
	return path, path != ""
}

// cString copies a NUL-terminated string GTK allocated and frees it.
func cString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	s := string(unsafe.Slice(p, n))
	gFree(p)
	return s
}

var (
	dialogMu       sync.Mutex
	dialogCurrent  uintptr
	chooserCurrent uintptr
)

// SelectFile points the running file dialog at a path (a test hook).
func SelectFile(path string) bool {
	dialogMu.Lock()
	c := chooserCurrent
	dialogMu.Unlock()
	return c != 0 && gtkChooserSet(c, path) != 0
}

// DialogHandle returns the GtkDialog* currently running, or 0. A test hook:
// a test answers the modal box with gtk_dialog_response through it.
func DialogHandle() uintptr {
	dialogMu.Lock()
	defer dialogMu.Unlock()
	return dialogCurrent
}

// RespondDialog answers a running dialog (gtk_dialog_response). A test hook.
func RespondDialog(d uintptr, response int32) { gtkDialogResp(d, response) }

// Focus: gtk_widget_grab_focus works before the window is shown too — it
// records the toplevel's focus widget, which takes effect on map.
func (w *window) Focus(h core.Handle) {
	if n := w.nodes[h]; n != nil {
		if n.kind == core.KindListBox {
			// A GtkListBox cannot take focus itself (can-focus is FALSE);
			// its rows can. Before the window is shown, grabbing on the
			// list does not stick and GTK then picks another widget
			// (observed: the button). Grab on the selected row, or the first.
			row := gtkListSelRow(n.inner)
			if row == 0 {
				row = gtkListRowAt(n.inner, 0)
			}
			if row != 0 {
				gtkGrabFocus(row)
			}
			return
		}
		if n.inner != 0 {
			// The scroller is what the layout holds; the keys go to what
			// is inside it.
			gtkGrabFocus(n.inner)
			return
		}
		gtkGrabFocus(n.handle)
	}
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropDropdownOnly:
		// GtkComboBoxText decides at construction whether it has an entry,
		// so the widget is rebuilt; this happens before items or layout.
		if n.kind == core.KindComboBox && v {
			gtkWidgetDestr(n.handle)
			n.handle = gtkComboNew()
			gSignalConnect(n.handle, "changed", w.drv.cbComboChanged, uintptr(h), 0, 0)
			gtkFixedPut(w.fixed, n.handle, 0, 0)
			gtkWidgetShow(n.handle)
		}
		return
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

func (w *window) SetInt(h core.Handle, p core.PropKey, v int) {
	n := w.nodes[h]
	if n == nil || p != core.PropSelected {
		return
	}
	switch n.kind {
	case core.KindComboBox:
		gtkComboSetAct(n.handle, int32(v))
	case core.KindListBox:
		if v < 0 {
			gtkListUnselect(n.inner)
		} else if row := gtkListRowAt(n.inner, int32(v)); row != 0 {
			gtkListSelect(n.inner, row)
		}
	}
}

func (w *window) SetList(h core.Handle, p core.PropKey, items []string) {
	n := w.nodes[h]
	if n == nil || p != core.PropItems {
		return
	}
	switch n.kind {
	case core.KindComboBox:
		gtkComboClear(n.handle)
		for _, it := range items {
			gtkComboAppend(n.handle, it)
		}
	case core.KindListBox:
		for row := gtkListRowAt(n.inner, 0); row != 0; row = gtkListRowAt(n.inner, 0) {
			gtkWidgetDestr(row)
		}
		n.items = append([]string(nil), items...)
		for _, it := range items {
			lbl := gtkLabelNew(it)
			gtkLabelXAlign(lbl, 0)
			gtkMarginStart(lbl, 6)
			gtkMarginEnd(lbl, 6)
			gtkWidgetShow(lbl)
			gtkListInsert(n.inner, lbl, -1) // wrapped in a GtkListBoxRow by GTK
		}
	}
}

// MeasureIntrinsic asks GTK itself, so theme padding and the user's font are
// respected — the whole reason §5.1 makes this a backend responsibility.
func (w *window) MeasureIntrinsic(h core.Handle, avail core.Size) (min, natural core.Size) {
	n := w.nodes[h]
	if n == nil {
		return core.Size{}, core.Size{}
	}
	var mn, nat requisition
	gtkPreferred(n.handle, unsafe.Pointer(&mn), unsafe.Pointer(&nat))
	if n.kind == core.KindListBox {
		// A scrolled window's natural size is barely more than its minimum;
		// the list inside knows the rows. Natural is up to listRows of
		// them; the minimum stays the scroller's, so the list can shrink.
		var lmn, lnat requisition
		gtkPreferred(n.inner, unsafe.Pointer(&lmn), unsafe.Pointer(&lnat))
		h := float64(lnat.Height)
		if cnt := len(n.items); cnt > listRows {
			h = h / float64(cnt) * listRows
		}
		if h < float64(nat.Height) {
			h = float64(nat.Height)
		}
		return core.Size{W: float64(mn.Width), H: float64(mn.Height)},
			core.Size{W: float64(lnat.Width) + scrollbarW, H: h}
	}
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
