//go:build darwin

// Package cocoa is the AppKit driver for macOS.
//
// Like the Linux drivers it links nothing: AppKit is reached through the
// Objective-C runtime with purego (objc_msgSend, and classes of our own
// registered at run time for the target, the delegates and the views), so
// the executable stays CGO_ENABLED=0.
//
// AppKit belongs to the main thread. This package locks the main goroutine
// to it in init, and Init refuses any other thread: goWidgets.NewApp has to
// be called from main (or, in a test, from TestMain's goroutine).
//
// Layout (§2.3): the content view is a flipped NSView — origin at the top
// left, like core's — and every control is its direct subview, placed with
// setFrame:. No Auto Layout, no stack views.
package cocoa

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
)

func init() {
	// The main goroutine starts on the main thread; locking it here, in a
	// package init, keeps it there — the only way AppKit can be used from Go.
	runtime.LockOSThread()
	core.RegisterDriver("cocoa", func() core.PlatformDriver {
		current = &driver{}
		return current
	})
}

// current is the driver most recently created, kept for the test hooks.
var current *driver

// AppKit constants used here.
const (
	styleTitled         = 1 << 0
	styleClosable       = 1 << 1
	styleMiniaturizable = 1 << 2
	styleResizable      = 1 << 3
	backingBuffered     = 2

	eventKeyDown          = 10
	eventKeyUp            = 11
	eventRightMouseUp     = 4
	eventApplicationDef   = 15
	modifierControl       = 1 << 18
	activationRegular     = 0
	controlStateOn        = 1
	bezelBorder           = 2
	alertFirstButton      = 1000
	modalResponseOK       = 1
	variableStatusItemLen = -1.0
)

// Process-wide AppKit state: one NSApplication and one set of our classes,
// shared by every driver instance (a second NewApp, the next test).
var (
	initOnce sync.Once
	initErr  error
	nsApp    objc.ID
	target   objc.ID // our GoWidgetsTarget: actions, delegates, data source
	mainLoop uintptr // CFRunLoopRef of the main thread
	wakeSrc  uintptr // our run loop source: wake-ups and the quit request

	// quitGen is the number of the main loop run asked to end, 0 for none.
	// The request is kept, not consumed: until the loop has actually
	// returned, every wake-up asks again (a modal session in the way ends
	// first, then the loop).
	quitGen atomic.Uint64
	loopGen atomic.Uint64
	pending atomic.Bool // the pump is wanted
)

type driver struct {
	win *window

	pump   func()
	pumpMu sync.Mutex
}

func (d *driver) Name() string { return "cocoa" }

func (d *driver) Capabilities() core.Caps {
	return core.Caps{
		NativeControls:  true,
		Clipboard:       true,
		FileDialog:      true,
		Menus:           true,
		Canvas:          true,
		SmoothAnimation: true,
		TrayIcon:        true, // NSStatusItem; see tray_darwin.go
		MaxCallbacks:    2000,
	}
}

func (d *driver) Init() error {
	initOnce.Do(func() { initErr = start() })
	if initErr != nil {
		return initErr
	}
	if pthreadMainNP() != 1 {
		return fmt.Errorf("cocoa: AppKit needs the main thread; call goWidgets.NewApp from main()")
	}
	return nil
}

// start loads AppKit and builds what every window shares.
func start() error {
	if err := loadFrameworks(); err != nil {
		return fmt.Errorf("AppKit not available: %w", err)
	}
	if pthreadMainNP() != 1 {
		return fmt.Errorf("cocoa: AppKit needs the main thread; call goWidgets.NewApp from main()")
	}
	// An outer pool for whatever is autoreleased before the run loop — the
	// application builds its window before Run. NSApp's loop drains a pool
	// of its own per event.
	alloc("NSAutoreleasePool")

	if err := registerClasses(); err != nil {
		return err
	}
	nsApp = msg(cls("NSApplication"), "sharedApplication")
	msg(nsApp, "setActivationPolicy:", activationRegular)
	buildMainMenu()

	target = alloc("GoWidgetsTarget")

	// The wake-up is a run loop source of our own (version 0: signalled by
	// hand), served in the modes loadFrameworks chose. CFRunLoopSourceSignal
	// and CFRunLoopWakeUp are safe from any thread, and several signals
	// before the loop gets round to it are one perform — one pump, which
	// drains the whole queue anyway.
	const performSlot = 9 // CFRunLoopSourceContext.perform
	wakeSourceContext[performSlot] = purego.NewCallback(func(info uintptr) uintptr {
		perform()
		return 0
	})
	mainLoop = cfRunLoopGetMain()
	wakeSrc = cfSourceCreate(0, 0, &wakeSourceContext)
	if wakeSrc == 0 {
		return fmt.Errorf("cocoa: CFRunLoopSourceCreate failed")
	}
	for _, m := range runLoopModes {
		cfRunLoopAddSrc(mainLoop, wakeSrc, m)
	}
	return nil
}

// postWakeEvent puts an empty event in the queue: stop: and
// stopModalWithCode: take effect only after the next event.
func postWakeEvent() {
	ev := msg(cls("NSEvent"), "otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:",
		uint64(eventApplicationDef), nsPoint{}, uint64(0), float64(0), int64(0), objc.ID(0), int16(0), int64(0), int64(0))
	msg(nsApp, "postEvent:atStart:", ev, true)
}

// signal wakes the main thread's run loop to perform. Any thread.
func signal() {
	cfSourceSignal(wakeSrc)
	cfRunLoopWakeUp(mainLoop)
}

// perform runs on the main thread when the source was signalled.
func perform() {
	if g := quitGen.Load(); g != 0 && g == loopGen.Load() {
		if msg(nsApp, "modalWindow") != 0 {
			// A modal session is in the way: end it, as if cancelled; the
			// main loop is asked again on the next round.
			const modalResponseAbort = -1001
			msg(nsApp, "stopModalWithCode:", int64(modalResponseAbort))
		} else {
			msg(nsApp, "stop:", objc.ID(0))
		}
		postWakeEvent() // either takes effect after the next event
	}
	if pending.Swap(false) {
		if d := current; d != nil {
			d.runPump()
		}
	}
}

// buildMainMenu gives the application the menu bar every Mac application
// has. The Edit menu is not decoration: Cmd+C, Cmd+V and friends in a text
// field are key equivalents of its items, and without them do nothing.
func buildMainMenu() {
	bar := alloc("NSMenu")
	appItem := alloc("NSMenuItem")
	msg(bar, "addItem:", appItem)
	appMenu := alloc("NSMenu")
	msg(appMenu, "addItemWithTitle:action:keyEquivalent:", nsString("Quit"), sel("terminate:"), nsString("q"))
	msg(appItem, "setSubmenu:", appMenu)

	editItem := alloc("NSMenuItem")
	msg(bar, "addItem:", editItem)
	edit := msg(msg(cls("NSMenu"), "alloc"), "initWithTitle:", nsString("Edit"))
	for _, it := range []struct{ title, action, key string }{
		{"Undo", "undo:", "z"},
		{"Redo", "redo:", "Z"},
		{"", "", ""},
		{"Cut", "cut:", "x"},
		{"Copy", "copy:", "c"},
		{"Paste", "paste:", "v"},
		{"Select All", "selectAll:", "a"},
	} {
		if it.title == "" {
			msg(edit, "addItem:", msg(cls("NSMenuItem"), "separatorItem"))
			continue
		}
		msg(edit, "addItemWithTitle:action:keyEquivalent:", nsString(it.title), sel(it.action), nsString(it.key))
	}
	msg(editItem, "setSubmenu:", edit)
	msg(nsApp, "setMainMenu:", bar)
}

// events is package-level because AppKit callbacks carry no Go context;
// there is one window per process.
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

func (d *driver) runPump() {
	d.pumpMu.Lock()
	p := d.pump
	d.pumpMu.Unlock()
	if p != nil {
		p()
	}
}

func (d *driver) RunMainLoop(ctx context.Context, pump func()) error {
	d.pumpMu.Lock()
	d.pump = pump
	d.pumpMu.Unlock()

	pump()
	gen := loopGen.Add(1)
	returned := make(chan struct{})
	defer close(returned)
	go func() {
		<-ctx.Done()
		quitGen.Store(gen)
		// Keep asking until the loop has returned: a request that lands
		// in a modal session only ends that session (the same lesson as
		// the Win32 driver's).
		for {
			signal()
			select {
			case <-returned:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	msg(nsApp, "run")
	return nil
}

// Wake asks for the pump on the main thread, in the main loop or in
// whatever modal loop is running.
func (d *driver) Wake() {
	if wakeSrc == 0 {
		return
	}
	pending.Store(true)
	signal()
}

// Shutdown takes the window off screen. An application exits right after,
// but a test binary builds the next application in the same process.
func (d *driver) Shutdown() {
	if d.win != nil {
		msg(d.win.handle, "orderOut:", objc.ID(0))
	}
}

// ------------------------------------------------------------------ window

type node struct {
	view          objc.ID // what the layout places: the control, or its scroll view
	inner         objc.ID // the table or text view inside a scroll view
	kind          core.WidgetKind
	items         []string // list box rows, combo box items
	canvasButtons uint8
}

type window struct {
	drv           *driver
	handle        objc.ID // our NSWindow subclass
	content       objc.ID // the flipped content view
	root          core.Handle
	nextH         core.Handle
	nodes         map[core.Handle]*node
	byView        map[objc.ID]core.Handle
	events        chan core.BackendEvent
	canvasCapture core.Handle
}

func (d *driver) CreateWindow(spec core.WindowSpec) (core.BackendWindow, error) {
	rect := nsRect{W: spec.Size.W, H: spec.Size.H}
	h := msg(msg(cls("GoWidgetsWindow"), "alloc"), "initWithContentRect:styleMask:backing:defer:",
		rect, uint64(styleTitled|styleClosable|styleMiniaturizable|styleResizable), uint64(backingBuffered), false)
	if h == 0 {
		return nil, fmt.Errorf("cocoa: NSWindow was not created")
	}
	msg(h, "setReleasedWhenClosed:", false)
	msg(h, "setTitle:", nsString(spec.Title))
	content := msg(msg(cls("GoWidgetsView"), "alloc"), "initWithFrame:", rect)
	msg(h, "setContentView:", content)
	msg(h, "setDelegate:", target)
	// Tab follows the layout, top to bottom and left to right — what
	// ADR-0008 asks of every backend, and what AppKit does on its own.
	msg(h, "setAutorecalculatesKeyViewLoop:", true)
	msg(h, "setAcceptsMouseMovedEvents:", true)
	msg(h, "center")

	w := &window{
		drv:     d,
		handle:  h,
		content: content,
		nodes:   map[core.Handle]*node{},
		byView:  map[objc.ID]core.Handle{},
		events:  make(chan core.BackendEvent, 128),
		nextH:   1,
	}
	w.root = w.nextH
	w.nodes[w.root] = &node{view: content}

	evMu.Lock()
	evTarget = w.events
	evMu.Unlock()
	d.win = w
	return w, nil
}

func (w *window) SetTitle(s string) { msg(w.handle, "setTitle:", nsString(s)) }

func (w *window) Show() {
	msg(w.handle, "makeKeyAndOrderFront:", objc.ID(0))
	msg(nsApp, "activateIgnoringOtherApps:", true)
}

func (w *window) Close()                           { msg(w.handle, "orderOut:", objc.ID(0)) }
func (w *window) RootHandle() core.Handle          { return w.root }
func (w *window) Events() <-chan core.BackendEvent { return w.events }

// Scale: AppKit lays out in points, which are core's DIP; the backing scale
// is reported for whoever needs the physical density.
func (w *window) Scale() core.ScaleInfo {
	s := msgT[float64](w.handle, "backingScaleFactor")
	if s <= 0 {
		s = 1
	}
	return core.ScaleInfo{Scale: s, FontScale: 1}
}

// contentSize is the content view's size, in points.
func (w *window) contentSize() core.Size {
	r := msgT[nsRect](w.content, "frame")
	return core.Size{W: r.W, H: r.H}
}

func (w *window) CreateWidget(kind core.WidgetKind, parent core.Handle) (core.Handle, error) {
	w.nextH++
	h := w.nextH
	n := &node{kind: kind}
	switch kind {
	case core.KindLabel:
		n.view = msg(cls("NSTextField"), "labelWithString:", nsString(""))
	case core.KindButton:
		n.view = msg(cls("NSButton"), "buttonWithTitle:target:action:", nsString(""), target, sel("fire:"))
	case core.KindCheckBox:
		n.view = msg(cls("NSButton"), "checkboxWithTitle:target:action:", nsString(""), target, sel("fire:"))
	case core.KindEdit:
		n.view = msg(cls("NSTextField"), "textFieldWithString:", nsString(""))
		w.textField(n.view)
	case core.KindComboBox:
		// With a text field by default, as GTK's; PropDropdownOnly swaps in
		// an NSPopUpButton before any item is added.
		n.view = alloc("NSComboBox")
		msg(n.view, "autorelease")
		msg(n.view, "setUsesDataSource:", false)
		msg(n.view, "setCompletes:", false)
		w.textField(n.view)
	case core.KindListBox:
		n.view, n.inner = newList()
	case core.KindTextView:
		n.view, n.inner = newTextView()
	case core.KindImageView:
		n.view = msg(msg(cls("NSImageView"), "alloc"), "initWithFrame:", nsRect{})
		msg(n.view, "autorelease")
		msg(n.view, "setImageScaling:", uint64(3)) // NSImageScaleProportionallyUpOrDown
	case core.KindCanvas:
		n.view = msg(msg(cls("NSImageView"), "alloc"), "initWithFrame:", nsRect{})
		msg(n.view, "autorelease")
		msg(n.view, "setImageScaling:", uint64(0))   // NSImageScaleAxesIndependently; frame is already DPI-scaled
		msg(n.view, "setImageAlignment:", uint64(0)) // NSImageAlignCenter
	default:
		return 0, fmt.Errorf("cocoa: unsupported widget kind %v", kind)
	}
	if n.view == 0 {
		return 0, fmt.Errorf("cocoa: %v was not created", kind)
	}
	w.add(h, n)
	return h, nil
}

// add puts a node's view into the content view and records it.
func (w *window) add(h core.Handle, n *node) {
	msg(n.view, "retain") // ours until DestroyWidget
	msg(w.content, "addSubview:", n.view)
	w.nodes[h] = n
	w.byView[n.view] = h
	if n.inner != 0 {
		w.byView[n.inner] = h
	}
}

// textField wires a text field (or combo box) to the target: typing is
// controlTextDidChange:, Enter is the action. The action is Enter only —
// not also "editing ended" on Tab, which would read as an activation.
func (w *window) textField(f objc.ID) {
	msg(f, "setDelegate:", target)
	msg(f, "setTarget:", target)
	msg(f, "setAction:", sel("fire:"))
	msg(msg(f, "cell"), "setSendsActionOnEndEditing:", false)
}

// newList is a one-column, headerless NSTableView in a scroll view, fed by
// our data source. Selection is by click; a double-click is the "open".
func newList() (scroll, table objc.ID) {
	scroll = alloc("NSScrollView")
	msg(scroll, "autorelease")
	msg(scroll, "setHasVerticalScroller:", true)
	msg(scroll, "setBorderType:", uint64(bezelBorder))
	table = alloc("NSTableView")
	msg(table, "autorelease")
	col := msg(msg(cls("NSTableColumn"), "alloc"), "initWithIdentifier:", nsString("item"))
	msg(col, "autorelease")
	msg(table, "addTableColumn:", col)
	msg(table, "setHeaderView:", objc.ID(0))
	const lastColumnOnly = 5 // NSTableViewLastColumnOnlyAutoresizingStyle
	msg(table, "setColumnAutoresizingStyle:", uint64(lastColumnOnly))
	msg(table, "setDataSource:", target)
	msg(table, "setDelegate:", target)
	msg(table, "setTarget:", target)
	msg(table, "setDoubleAction:", sel("open:"))
	msg(scroll, "setDocumentView:", table)
	return scroll, table
}

// newTextView is a read-only log, as in the other drivers: the user's
// fixed-width font, no wrapping (long lines scroll sideways).
func newTextView() (scroll, text objc.ID) {
	scroll = msg(cls("NSTextView"), "scrollableTextView")
	text = msg(scroll, "documentView")
	msg(text, "setEditable:", false)
	size := msgT[float64](cls("NSFont"), "systemFontSize")
	msg(text, "setFont:", msg(cls("NSFont"), "userFixedPitchFontOfSize:", size))
	huge := nsSize{W: 1e7, H: 1e7}
	container := msg(text, "textContainer")
	msg(container, "setWidthTracksTextView:", false)
	msg(container, "setContainerSize:", huge)
	msg(text, "setHorizontallyResizable:", true)
	msg(text, "setMaxSize:", huge)
	msg(scroll, "setHasHorizontalScroller:", true)
	msg(scroll, "setBorderType:", uint64(bezelBorder)) // framed, like the list
	return scroll, text
}

func (w *window) DestroyWidget(h core.Handle) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	msg(n.view, "removeFromSuperview")
	delete(w.byView, n.view)
	delete(w.byView, n.inner)
	msg(n.view, "release")
	delete(w.nodes, h)
}

func (w *window) SetParent(child, parent core.Handle, index int) {}

func (w *window) SetString(h core.Handle, p core.PropKey, v string) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	if p == core.PropImagePath && n.kind == core.KindImageView {
		var image objc.ID
		if v != "" {
			image = msg(cls("NSImage"), "imageWithContentsOfFile:", nsString(v))
		}
		msg(n.view, "setImage:", image)
		return
	}
	if p != core.PropText {
		return
	}
	switch n.kind {
	case core.KindButton, core.KindCheckBox:
		msg(n.view, "setTitle:", nsString(v))
	case core.KindLabel, core.KindEdit:
		msg(n.view, "setStringValue:", nsString(v))
	case core.KindComboBox:
		if !w.isPopUp(n) {
			msg(n.view, "setStringValue:", nsString(v))
		}
	case core.KindTextView:
		msg(n.inner, "setString:", nsString(v))
	}
}

// isPopUp tells the dropdown-only combo box (NSPopUpButton) from the one
// with a text field (NSComboBox).
func (w *window) isPopUp(n *node) bool {
	return msgT[bool](n.view, "isKindOfClass:", cls("NSPopUpButton"))
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropDropdownOnly:
		if n.kind == core.KindComboBox && v && !w.isPopUp(n) {
			// NSComboBox always has its field; a pure list is another
			// class. Swapped before any item or layout, as GTK does.
			w.DestroyWidget(h)
			popup := alloc("NSPopUpButton")
			msg(popup, "autorelease")
			msg(popup, "setTarget:", target)
			msg(popup, "setAction:", sel("fire:"))
			w.add(h, &node{view: popup, kind: core.KindComboBox})
		}
	case core.PropEnabled:
		if n.inner != 0 {
			if n.kind == core.KindTextView {
				msg(n.inner, "setSelectable:", v)
			} else {
				msg(n.inner, "setEnabled:", v)
			}
			return
		}
		msg(n.view, "setEnabled:", v)
	case core.PropVisible:
		msg(n.view, "setHidden:", !v)
	case core.PropChecked:
		if n.kind == core.KindCheckBox {
			state := int64(0)
			if v {
				state = controlStateOn
			}
			msg(n.view, "setState:", state)
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
		if w.isPopUp(n) {
			msg(n.view, "selectItemAtIndex:", int64(v))
			return
		}
		if v < 0 {
			if i := int64(msg(n.view, "indexOfSelectedItem")); i >= 0 {
				msg(n.view, "deselectItemAtIndex:", i)
			}
			msg(n.view, "setStringValue:", nsString(""))
			return
		}
		msg(n.view, "selectItemAtIndex:", int64(v))
		if v < len(n.items) {
			msg(n.view, "setStringValue:", nsString(n.items[v]))
		}
	case core.KindListBox:
		if v < 0 {
			msg(n.inner, "deselectAll:", objc.ID(0))
			return
		}
		set := msg(cls("NSIndexSet"), "indexSetWithIndex:", uint64(v))
		msg(n.inner, "selectRowIndexes:byExtendingSelection:", set, false)
		msg(n.inner, "scrollRowToVisible:", int64(v))
	}
}

func (w *window) SetList(h core.Handle, p core.PropKey, items []string) {
	n := w.nodes[h]
	if n == nil || p != core.PropItems {
		return
	}
	n.items = append([]string(nil), items...)
	switch n.kind {
	case core.KindComboBox:
		msg(n.view, "removeAllItems")
		if w.isPopUp(n) {
			// Through the menu: addItemsWithTitles: drops duplicates.
			menu := msg(n.view, "menu")
			for _, it := range items {
				msg(menu, "addItemWithTitle:action:keyEquivalent:", nsString(it), objc.SEL(0), nsString(""))
			}
			msg(n.view, "selectItemAtIndex:", int64(-1))
			return
		}
		msg(n.view, "addItemsWithObjectValues:", nsArray(items))
		msg(n.view, "setStringValue:", nsString(""))
	case core.KindListBox:
		msg(n.inner, "reloadData")
		msg(n.inner, "deselectAll:", objc.ID(0))
	}
}

// Focus: makeFirstResponder: works on a window that is not on screen yet,
// and initialFirstResponder covers the moment it comes up.
func (w *window) Focus(h core.Handle) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	v := n.view
	if n.inner != 0 {
		v = n.inner
	}
	msg(w.handle, "setInitialFirstResponder:", v)
	msg(w.handle, "makeFirstResponder:", v)
}

func (w *window) Dialog(kind core.DialogKind, title, text string) core.DialogResult {
	return runAlert(kind, title, text)
}

func (w *window) FileDialog(save bool, title, suggested string, filters []core.FileFilter) (string, bool) {
	return runPanel(save, title, suggested, filters)
}

// listRows is the natural height of a list box, in rows, as in the other
// drivers.
const listRows = 8

// MeasureIntrinsic asks AppKit: intrinsicContentSize, in alignment-rect
// terms, from the control's own font and bezel. Where a control has no
// intrinsic width (a text field, a combo box) the width is a choice, as it
// is in the headless driver; a list is as wide as its widest item.
func (w *window) MeasureIntrinsic(h core.Handle, avail core.Size) (min, natural core.Size) {
	n := w.nodes[h]
	if n == nil {
		return core.Size{}, core.Size{}
	}
	var s nsSize
	if n.inner == 0 {
		s = msgT[nsSize](n.view, "intrinsicContentSize")
	}
	const noMetric = -1 // NSViewNoIntrinsicMetric
	switch n.kind {
	case core.KindEdit:
		return core.Size{W: 40, H: s.H}, core.Size{W: 160, H: s.H}
	case core.KindComboBox:
		if s.W == noMetric || s.W <= 0 {
			wd := 100.0
			for _, it := range n.items {
				wd = math.Max(wd, textWidth(n.view, it)+40)
			}
			s.W = wd
		}
		return s2c(s), s2c(s)
	case core.KindListBox:
		rowH := msgT[float64](n.inner, "rowHeight") + msgT[nsSize](n.inner, "intercellSpacing").H
		wd := 60.0
		for _, it := range n.items {
			wd = math.Max(wd, textWidth(n.inner, it)+24)
		}
		rows := math.Min(math.Max(float64(len(n.items)), 1), listRows)
		return core.Size{W: 40, H: rowH + 4}, core.Size{W: wd, H: rowH*rows + 4}
	case core.KindTextView:
		return core.Size{W: 40, H: 40}, core.Size{W: 200, H: 80}
	}
	if s.W < 0 {
		s.W = 0
	}
	if s.H < 0 {
		s.H = 0
	}
	return s2c(s), s2c(s)
}

func s2c(s nsSize) core.Size { return core.Size{W: s.W, H: s.H} }

// textWidth measures a string in the control's font.
func textWidth(control objc.ID, s string) float64 {
	font := msg(control, "font")
	if font == 0 {
		font = msg(cls("NSFont"), "systemFontOfSize:", msgT[float64](cls("NSFont"), "systemFontSize"))
	}
	attrs := msg(cls("NSDictionary"), "dictionaryWithObject:forKey:", font, nsString("NSFont"))
	return math.Ceil(msgT[nsSize](nsString(s), "sizeWithAttributes:", attrs).W)
}

// ApplyLayout sets frames. Core's rectangles are alignment rects — the
// control's visual edges, which is also what intrinsicContentSize measures;
// frameForAlignmentRect: adds whatever a bezel's shadow needs around them.
func (w *window) ApplyLayout(changes []core.BoundsChange) {
	for _, c := range changes {
		n := w.nodes[c.H]
		if n == nil {
			continue
		}
		if !c.Visible {
			msg(n.view, "setHidden:", true)
			continue
		}
		r := nsRect{X: math.Round(c.R.X), Y: math.Round(c.R.Y)}
		r.W = math.Round(c.R.X+c.R.W) - r.X
		r.H = math.Round(c.R.Y+c.R.H) - r.Y
		msg(n.view, "setFrame:", msgT[nsRect](n.view, "frameForAlignmentRect:", r))
		msg(n.view, "setHidden:", false)
		if n.kind == core.KindListBox {
			msg(n.inner, "sizeLastColumnToFit")
		}
	}
}

// itemText is a list node's i-th item, or "".
func (w *window) itemText(h core.Handle, i int) string {
	if n := w.nodes[h]; n != nil && i >= 0 && i < len(n.items) {
		return n.items[i]
	}
	return ""
}
