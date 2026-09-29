//go:build linux

// Package qt is the Qt Widgets driver, for Qt 6 and Qt 5 alike.
//
// Like the GTK driver it links nothing: the Qt libraries are opened at run
// time and every function is called through purego, so the executable stays
// CGO_ENABLED=0 and starts on a machine without Qt — Init fails and core
// moves on (§3.3). Qt is C++ without a C API; abi_linux.go is how that is
// bridged, and the only file that knows the two majors apart.
//
// Layout (§2.3): widgets are children of the window placed with setGeometry,
// no QLayout anywhere. Core owns layout and hands down absolute rectangles.
package qt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets/core"
)

func init() {
	// "qt" takes whichever major the machine has, newest first; "qt6" and
	// "qt5" pin one, for goWidgets_BACKEND and for tests.
	core.RegisterDriver("qt", func() core.PlatformDriver { return newDriver(6, 5) })
	core.RegisterDriver("qt6", func() core.PlatformDriver { return newDriver(6) })
	core.RegisterDriver("qt5", func() core.PlatformDriver { return newDriver(5) })
}

func newDriver(majors ...int) *driver {
	current = &driver{majors: majors}
	return current
}

// current is the driver most recently created, kept for the test hooks.
var current *driver

// Version reports the Qt major the driver bound to, 0 before Init. A test
// hook, and the answer to "which Qt did I get".
func Version() int { return ver }

// WindowHandle returns the QWidget* of the current window, or 0. A test hook,
// like the GTK driver's.
func WindowHandle() uintptr {
	if current == nil || current.win == nil {
		return 0
	}
	return current.win.handle
}

// WidgetHandle returns the QWidget* of the first widget of the kind in the
// current window, or 0. A test hook.
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

// Qt entry points. Names are the mangled C++ symbols; see abi_linux.go.
var (
	qVersion         func() string
	qSetAttribute    func(attr int32, on bool)
	qAppCtor         func(this unsafe.Pointer, argc *int32, argv unsafe.Pointer, flags int32)
	qAppExec         func() int32
	qAppExit         func(code int32)
	qQuitOnLastClose func(on bool)
	qPostEvent       func(receiver uintptr, ev unsafe.Pointer, priority int32)
	qEventCtor       func(this unsafe.Pointer, kind int32)
	qInstallFilter   func(obj uintptr, filter uintptr)
	qDeleteLater     func(obj uintptr)
	qLibraryPath     func(which int32) qstring
	qFocusWidget     func() uintptr

	qWidgetCtor    func(this unsafe.Pointer, parent uintptr, flags int32)
	qWidgetTitle   func(w uintptr, title *qstring)
	qWidgetResize  func(w uintptr, size *qSize)
	qWidgetGeom    func(w uintptr, r *qRect)
	qWidgetShow    func(w uintptr)
	qWidgetHide    func(w uintptr)
	qWidgetEnable  func(w uintptr, on bool)
	qWidgetFocus   func(w uintptr, reason int32)
	qWidgetWindow  func(w uintptr) uintptr
	qSetTabOrder   func(first, second uintptr)
	qWindowHandle  func(w uintptr) uintptr
	qWindowDPR     func(win uintptr) float64
	qContentsRect  func(w uintptr) qRect
	qLabelCtor     func(this unsafe.Pointer, parent uintptr, flags int32)
	qLabelText     func(l uintptr, text *qstring)
	qPushButtonNew func(this unsafe.Pointer, parent uintptr)
	qButtonText    func(b uintptr, text *qstring)
	qCheckBoxNew   func(this unsafe.Pointer, parent uintptr)
	qSetChecked    func(b uintptr, on bool)
	qLineEditNew   func(this unsafe.Pointer, parent uintptr)
	qLineEditSet   func(e uintptr, text *qstring)
	qLineEditText  func(e uintptr) qstring

	sigClicked signal
	sigToggled signal
	sigEdited  signal // QLineEdit::textChanged(QString)
	sigReturn  signal // QLineEdit::returnPressed()

	slotSizeHint    int
	slotMinSizeHint int
)

// qSize and qRect mirror QSize and QRect. QRect stores the far edges
// inclusively: x2 = x + width - 1.
type (
	qSize struct{ W, H int32 }
	qRect struct{ X1, Y1, X2, Y2 int32 }
)

func (r *resolver) bindAll() {
	r.fn(&qStringCtor, v("_ZN7QStringC1EPK5QChari", "_ZN7QStringC1EPK5QCharx"))
	r.fn(&qArrayDealloc, v("_ZN10QArrayData10deallocateEPS_mm", "_ZN10QArrayData10deallocateEPS_xx"))
	r.fn(&cxxNew, "_Znwm")
	r.fn(&qObjectCtor, "_ZN7QObjectC1EPS_")
	r.fn(&qConnectImpl, "_ZN7QObject11connectImplEPKS_PPvS1_S3_PN9QtPrivate15QSlotObjectBaseEN2Qt14ConnectionTypeEPKiPK11QMetaObject")
	r.fn(&qConnectionDtor, "_ZN11QMetaObject10ConnectionD1Ev")

	r.fn(&qVersion, "qVersion")
	r.fn(&qSetAttribute, "_ZN16QCoreApplication12setAttributeEN2Qt20ApplicationAttributeEb")
	r.fn(&qAppCtor, "_ZN12QApplicationC1ERiPPci")
	r.fn(&qAppExec, "_ZN12QApplication4execEv")
	r.fn(&qAppExit, "_ZN16QCoreApplication4exitEi")
	r.fn(&qQuitOnLastClose, "_ZN15QGuiApplication25setQuitOnLastWindowClosedEb")
	r.fn(&qPostEvent, "_ZN16QCoreApplication9postEventEP7QObjectP6QEventi")
	r.fn(&qEventCtor, "_ZN6QEventC1ENS_4TypeE")
	r.fn(&qInstallFilter, "_ZN7QObject18installEventFilterEPS_")
	r.fn(&qDeleteLater, "_ZN7QObject11deleteLaterEv")
	r.fn(&qLibraryPath, v("_ZN12QLibraryInfo8locationENS_15LibraryLocationE", "_ZN12QLibraryInfo4pathENS_11LibraryPathE"))
	r.fn(&qFocusWidget, "_ZN12QApplication11focusWidgetEv")

	r.fn(&qWidgetCtor, "_ZN7QWidgetC1EPS_6QFlagsIN2Qt10WindowTypeEE")
	r.fn(&qWidgetTitle, "_ZN7QWidget14setWindowTitleERK7QString")
	r.fn(&qWidgetResize, "_ZN7QWidget6resizeERK5QSize")
	r.fn(&qWidgetGeom, "_ZN7QWidget11setGeometryERK5QRect")
	r.fn(&qWidgetShow, "_ZN7QWidget4showEv")
	r.fn(&qWidgetHide, "_ZN7QWidget4hideEv")
	r.fn(&qWidgetEnable, "_ZN7QWidget10setEnabledEb")
	r.fn(&qWidgetFocus, "_ZN7QWidget8setFocusEN2Qt11FocusReasonE")
	r.fn(&qWidgetWindow, "_ZNK7QWidget6windowEv")
	r.fn(&qSetTabOrder, "_ZN7QWidget11setTabOrderEPS_S0_")
	r.fn(&qWindowHandle, "_ZNK7QWidget12windowHandleEv")
	r.fn(&qWindowDPR, "_ZNK7QWindow16devicePixelRatioEv")
	r.fn(&qContentsRect, "_ZNK7QWidget12contentsRectEv")
	r.fn(&qLabelCtor, "_ZN6QLabelC1EP7QWidget6QFlagsIN2Qt10WindowTypeEE")
	r.fn(&qLabelText, "_ZN6QLabel7setTextERK7QString")
	r.fn(&qPushButtonNew, "_ZN11QPushButtonC1EP7QWidget")
	r.fn(&qButtonText, "_ZN15QAbstractButton7setTextERK7QString")

	r.fn(&qCheckBoxNew, "_ZN9QCheckBoxC1EP7QWidget")
	r.fn(&qSetChecked, "_ZN15QAbstractButton10setCheckedEb")

	sigClicked = r.signal("15QAbstractButton", "_ZN15QAbstractButton7clickedEb")
	sigToggled = r.signal("15QAbstractButton", "_ZN15QAbstractButton7toggledEb")
	r.fn(&qLineEditNew, "_ZN9QLineEditC1EP7QWidget")
	r.fn(&qLineEditSet, "_ZN9QLineEdit7setTextERK7QString")
	r.fn(&qLineEditText, "_ZNK9QLineEdit4textEv")
	sigEdited = r.signal("9QLineEdit", "_ZN9QLineEdit11textChangedERK7QString")
	sigReturn = r.signal("9QLineEdit", "_ZN9QLineEdit13returnPressedEv")

	if slotSizeHint = vslot("_ZTV7QWidget", "_ZNK7QWidget8sizeHintEv", 64); slotSizeHint < 0 {
		r.missing = append(r.missing, "QWidget::sizeHint slot")
	}
	if slotMinSizeHint = vslot("_ZTV7QWidget", "_ZNK7QWidget15minimumSizeHintEv", 64); slotMinSizeHint < 0 {
		r.missing = append(r.missing, "QWidget::minimumSizeHint slot")
	}
}

// Process-wide Qt state. A process gets one QApplication, and one Qt major:
// a second driver instance (a second NewApp) reuses them.
var (
	qapp      uintptr // QApplication*
	helper    uintptr // our QObject: wake-ups, and the application's event filter
	origEvent uintptr // QObject::event, for everything the helper does not handle
)

type driver struct {
	majors []int
	win    *window

	pump   func()
	pumpMu sync.Mutex
}

func (d *driver) Name() string { return "qt" }

func (d *driver) Capabilities() core.Caps {
	return core.Caps{
		NativeControls:  true,
		Clipboard:       true,
		FileDialog:      true,
		Menus:           true,
		SmoothAnimation: true,
		MaxCallbacks:    2000, // purego's callback pool
	}
}

func (d *driver) Init() error {
	if qapp != 0 {
		for _, m := range d.majors {
			if m == ver {
				return nil
			}
		}
		return fmt.Errorf("Qt %d is already running in this process", ver)
	}
	var errs []string
	for _, m := range d.majors {
		err := d.start(m)
		if err == nil {
			return nil
		}
		errs = append(errs, err.Error())
	}
	return errors.New(strings.Join(errs, "; "))
}

// start binds one Qt major and brings up its QApplication.
func (d *driver) start(major int) error {
	if err := load(major); err != nil {
		return err
	}
	if err := displayReachable(takeString(qLibraryPath(lay.plugins))); err != nil {
		return fmt.Errorf("Qt %d: %w", major, err)
	}
	if ver == 5 {
		// Qt 6 always lays out in device-independent pixels; Qt 5 does so
		// only when asked, before the application exists. Core speaks DIP.
		const aaEnableHighDpiScaling = 20
		qSetAttribute(aaEnableHighDpiScaling, true)
	}

	// QApplication keeps argc by reference and argv by pointer for its whole
	// life, so both live in memory Go will never move or free.
	argc := (*int32)(cxxNew(8))
	*argc = 1
	name := append([]byte("goWidgets"), 0)
	arg0 := cxxNew(uintptr(len(name)))
	copy(unsafe.Slice((*byte)(arg0), len(name)), name)
	argv := cxxNew(16)
	*(*unsafe.Pointer)(argv) = arg0
	*(*uintptr)(unsafe.Add(argv, 8)) = 0

	app := cxxNew(64) // sizeof(QApplication) is 16 in both majors
	qAppCtor(app, argc, argv, compiledVersion())
	qapp = uintptr(app)
	// Core decides what closing the window means; hiding it for the tray,
	// or a dialog closing last, must not end the loop behind its back.
	qQuitOnLastClose(false)

	initSlots()
	d.initHelper()
	qInstallFilter(qapp, helper)
	loadTranslations()
	return nil
}

// compiledVersion is QT_VERSION as the QApplication constructor wants it:
// the flags argument is the version the application was built against, and
// Qt switches a few compatibility behaviours on it. The runtime's own
// version is the honest answer for a binary built against no headers.
func compiledVersion() int32 {
	parts := strings.SplitN(qVersion(), ".", 3)
	n := int32(0)
	for i := 0; i < 3; i++ {
		x := 0
		if i < len(parts) {
			x, _ = strconv.Atoi(parts[i])
		}
		n = n<<8 | int32(x&0xff)
	}
	return n
}

// initHelper makes the QObject that receives our posted wake-ups and filters
// the application's events. There is no moc here to subclass QObject with,
// so the subclass is made by hand: a copy of QObject's vtable with event()
// and eventFilter() pointing at Go, installed as the object's vptr. The
// other ten slots stay QObject's own.
func (d *driver) initHelper() {
	obj := cxxNew(64)
	qObjectCtor(obj, 0)

	const words = 14 // offset-to-top, typeinfo, 12 virtuals: sizeof(_ZTV7QObject) = 0x70
	const slotEvent, slotEventFilter = 5, 6
	orig := unsafe.Add(*(*unsafe.Pointer)(obj), -16)
	vt := cxxNew(words * 8)
	copy(unsafe.Slice((*uintptr)(vt), words), unsafe.Slice((*uintptr)(orig), words))
	slots := unsafe.Add(vt, 16)
	origEvent = *(*uintptr)(unsafe.Add(slots, slotEvent*8))

	cbEvent := purego.NewCallback(func(this unsafe.Pointer, ev unsafe.Pointer) uintptr {
		switch evType(ev) {
		case evUser:
			d.runPump()
			return 1
		case evUserQuit:
			qAppExit(0)
			return 1
		}
		r, _, _ := purego.SyscallN(origEvent, uintptr(this), uintptr(ev))
		return r & 0xff
	})
	cbFilter := purego.NewCallback(func(this unsafe.Pointer, watched uintptr, ev unsafe.Pointer) uintptr {
		if d.filter(watched, ev) {
			return 1
		}
		return 0
	})
	*(*uintptr)(unsafe.Add(slots, slotEvent*8)) = cbEvent
	*(*uintptr)(unsafe.Add(slots, slotEventFilter*8)) = cbFilter
	*(*unsafe.Pointer)(obj) = slots
	helper = uintptr(obj)
}

// post queues an event of the given type to the helper. postEvent is
// documented thread-safe, which is what Wake is built on; Qt deletes the
// event after delivery, hence operator new.
func post(kind int32) {
	if helper == 0 {
		return
	}
	ev := cxxNew(64) // sizeof(QEvent) is 24 in Qt 5, 16 in Qt 6
	qEventCtor(ev, kind)
	qPostEvent(helper, ev, 0)
}

func (d *driver) runPump() {
	d.pumpMu.Lock()
	p := d.pump
	d.pumpMu.Unlock()
	if p != nil {
		p()
	}
}

// filter sees every event of the application before its receiver does. It
// answers true to swallow one.
func (d *driver) filter(watched uintptr, ev unsafe.Pointer) bool {
	w := d.win
	if w == nil {
		return false
	}
	switch evType(ev) {
	case evClose:
		if watched == w.handle {
			// Never close on our own: core decides and calls Close. The
			// event is ignored as well as swallowed, because an accepted
			// close event is what tells QWidget to go ahead.
			emit(core.BackendEvent{Kind: core.EventCloseRequested})
			evIgnore(ev)
			return true
		}
	case evResize:
		if watched == w.handle {
			s := (*qSize)(unsafe.Add(ev, lay.resizeSize))
			emit(core.BackendEvent{Kind: core.EventResized, Size: core.Size{W: float64(s.W), H: float64(s.H)}})
		}
	case evKeyPress, evKeyRelease:
		// Every key of the window, whichever control has focus — as GTK's
		// key-press-event on the toplevel. A key event travels from the
		// QWindow to the focus widget and on up through the parents that
		// ignore it, and the application filter sees each stop; the focus
		// widget's is the one to report. Keys of a dialog are not ours.
		focus := qFocusWidget()
		if (focus != 0 && watched == focus && qWidgetWindow(focus) == w.handle) ||
			(focus == 0 && watched == w.handle) {
			emit(core.BackendEvent{Kind: core.EventKey, Key: keyFromQt(ev, evType(ev) == evKeyPress)})
		}
	}
	return false
}

// events is package-level because Qt callbacks carry no Go context; there is
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
		post(evUserQuit) // exit() belongs on the UI thread
	}()
	qAppExec()
	return nil
}

// Wake posts a wake-up to the helper; Qt delivers it on the UI thread, in
// the main loop or in whatever modal loop is running.
func (d *driver) Wake() { post(evUser) }

func (d *driver) Shutdown() {}

func (d *driver) CreateTray(spec core.TraySpec) (core.BackendTray, error) {
	return nil, core.ErrNoTray
}

func (d *driver) CreateWindow(spec core.WindowSpec) (core.BackendWindow, error) {
	obj := cxxNew(64)
	qWidgetCtor(obj, 0, 0)
	h := uintptr(obj)
	qstr(spec.Title, func(s *qstring) { qWidgetTitle(h, s) })
	qWidgetResize(h, &qSize{W: int32(spec.Size.W), H: int32(spec.Size.H)})

	w := &window{
		drv:    d,
		handle: h,
		nodes:  map[core.Handle]*node{},
		events: make(chan core.BackendEvent, 128),
		nextH:  1,
	}
	w.root = w.nextH
	w.nodes[w.root] = &node{handle: h}

	evMu.Lock()
	evTarget = w.events
	evMu.Unlock()

	d.win = w
	return w, nil
}

type node struct {
	handle uintptr // the QWidget placed in the layout
	kind   core.WidgetKind
	rect   qRect // last geometry applied, for the tab order
	placed bool
}

type window struct {
	drv    *driver
	handle uintptr
	root   core.Handle
	nextH  core.Handle
	nodes  map[core.Handle]*node
	events chan core.BackendEvent
}

func (w *window) SetTitle(s string) {
	qstr(s, func(q *qstring) { qWidgetTitle(w.handle, q) })
}
func (w *window) Show()                            { qWidgetShow(w.handle) }
func (w *window) Close()                           { qWidgetHide(w.handle) }
func (w *window) RootHandle() core.Handle          { return w.root }
func (w *window) Events() <-chan core.BackendEvent { return w.events }

// Scale: Qt works in device-independent pixels (Qt 5 is told to in start),
// so core's DIP and Qt's coordinates coincide. The ratio is reported for
// whoever needs the physical density.
func (w *window) Scale() core.ScaleInfo {
	s := 1.0
	if wh := qWindowHandle(w.handle); wh != 0 {
		if r := qWindowDPR(wh); r > 0 {
			s = r
		}
	}
	return core.ScaleInfo{Scale: s, FontScale: 1}
}

func (w *window) CreateWidget(kind core.WidgetKind, parent core.Handle) (core.Handle, error) {
	w.nextH++
	h := w.nextH

	obj := cxxNew(128) // the largest of these is 48 bytes in Qt 5
	switch kind {
	case core.KindLabel:
		qLabelCtor(obj, w.handle, 0)
	case core.KindButton:
		qPushButtonNew(obj, w.handle)
	case core.KindCheckBox:
		qCheckBoxNew(obj, w.handle)
	case core.KindEdit:
		qLineEditNew(obj, w.handle)
	default:
		return 0, fmt.Errorf("qt: unsupported widget kind %v", kind)
	}
	g := uintptr(obj)
	w.nodes[h] = &node{handle: g, kind: kind}

	switch kind {
	case core.KindButton:
		connect(g, sigClicked, func(*[4]unsafe.Pointer) {
			emit(core.BackendEvent{Kind: core.EventClicked, H: h})
		})
	case core.KindCheckBox:
		// toggled(bool) fires for setChecked too, like GTK's "toggled";
		// core tells the program's own writes apart by value.
		connect(g, sigToggled, func(a *[4]unsafe.Pointer) {
			emit(core.BackendEvent{Kind: core.EventToggled, H: h, Bool: *(*bool)(a[1])})
		})
	case core.KindEdit:
		// textChanged, not textEdited: like GTK's "changed" it also reports
		// setText, and core drops the program's own text by value. The
		// argument is the whole new text.
		connect(g, sigEdited, func(a *[4]unsafe.Pointer) {
			emit(core.BackendEvent{Kind: core.EventTextChanged, H: h, Text: (*qstring)(a[1]).String()})
		})
		connect(g, sigReturn, func(*[4]unsafe.Pointer) {
			emit(core.BackendEvent{Kind: core.EventActivated, H: h, Text: takeString(qLineEditText(g))})
		})
	}
	qWidgetShow(g)
	return h, nil
}

func (w *window) DestroyWidget(h core.Handle) {
	if n := w.nodes[h]; n != nil {
		qDeleteLater(n.handle)
		delete(w.nodes, h)
	}
}

func (w *window) SetParent(child, parent core.Handle, index int) {
	// A single container, as in the GTK driver.
}

func (w *window) SetString(h core.Handle, p core.PropKey, v string) {
	n := w.nodes[h]
	if n == nil || p != core.PropText {
		return
	}
	qstr(v, func(s *qstring) {
		switch n.kind {
		case core.KindButton, core.KindCheckBox:
			qButtonText(n.handle, s) // both are QAbstractButtons
		case core.KindLabel:
			qLabelText(n.handle, s)
		case core.KindEdit:
			// setText with the text the field holds is a no-op, so the
			// platform's own edit coming round does not move the caret.
			qLineEditSet(n.handle, s)
		}
	})
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropEnabled:
		qWidgetEnable(n.handle, v)
	case core.PropVisible:
		if v {
			qWidgetShow(n.handle)
		} else {
			qWidgetHide(n.handle)
		}
	case core.PropChecked:
		if n.kind == core.KindCheckBox {
			qSetChecked(n.handle, v)
		}
	}
}

func (w *window) SetFloat(core.Handle, core.PropKey, float64) {}

func (w *window) SetInt(h core.Handle, p core.PropKey, v int) {}

func (w *window) SetList(h core.Handle, p core.PropKey, items []string) {}

// Focus: setFocus on a widget of a window that is not shown yet records it
// as the window's focus child, which takes effect when the window activates.
func (w *window) Focus(h core.Handle) {
	if n := w.nodes[h]; n != nil {
		const otherFocusReason = 7
		qWidgetFocus(n.handle, otherFocusReason)
	}
}

func (w *window) Dialog(kind core.DialogKind, title, text string) core.DialogResult {
	return core.DialogCancel
}

func (w *window) FileDialog(save bool, title, suggested string, filters []core.FileFilter) (string, bool) {
	return "", false
}

// sizeHint calls a virtual QSize-returning getter; QSize comes back packed in
// one register. An invalid size (-1) reads as zero.
func sizeHint(obj uintptr, slot int) core.Size {
	r := vcall(obj, slot)
	wd, ht := int32(uint32(r)), int32(uint32(r>>32))
	return core.Size{W: float64(max(wd, 0)), H: float64(max(ht, 0))}
}

// MeasureIntrinsic asks the widget itself: sizeHint is the natural size and
// minimumSizeHint the least it can do with, both from the style in use and
// the user's font — the reason §5.1 makes this a backend job.
func (w *window) MeasureIntrinsic(h core.Handle, avail core.Size) (min, natural core.Size) {
	n := w.nodes[h]
	if n == nil {
		return core.Size{}, core.Size{}
	}
	natural = sizeHint(n.handle, slotSizeHint)
	min = sizeHint(n.handle, slotMinSizeHint)
	if min.W > natural.W {
		min.W = natural.W
	}
	if min.H > natural.H {
		min.H = natural.H
	}
	return min, natural
}

func (w *window) ApplyLayout(changes []core.BoundsChange) {
	moved := false
	for _, c := range changes {
		n := w.nodes[c.H]
		if n == nil {
			continue
		}
		if !c.Visible {
			qWidgetHide(n.handle)
			continue
		}
		// Round the edges, not the extent, so that neighbours which touch in
		// DIP still touch in pixels.
		x0, y0 := int32(math.Round(c.R.X)), int32(math.Round(c.R.Y))
		x1, y1 := int32(math.Round(c.R.X+c.R.W)), int32(math.Round(c.R.Y+c.R.H))
		r := qRect{X1: x0, Y1: y0, X2: x1 - 1, Y2: y1 - 1}
		qWidgetGeom(n.handle, &r)
		qWidgetShow(n.handle)
		if !n.placed || n.rect != r {
			moved = true
		}
		n.rect, n.placed = r, true
	}
	if moved {
		w.sortTabOrder()
	}
}

// sortTabOrder makes Tab follow the layout — top to bottom, then left to
// right — as GtkFixed does on its own and the Win32 driver does by hand
// (ADR-0008). Qt's default is creation order, which in the showcase sends
// Tab from the field past the list that grows under it.
func (w *window) sortTabOrder() {
	var ns []*node
	for h, n := range w.nodes {
		if h != w.root && n.placed {
			ns = append(ns, n)
		}
	}
	sort.Slice(ns, func(i, j int) bool {
		a, b := ns[i].rect, ns[j].rect
		if a.Y1 != b.Y1 {
			return a.Y1 < b.Y1
		}
		return a.X1 < b.X1
	})
	for i := 1; i < len(ns); i++ {
		qSetTabOrder(ns[i-1].handle, ns[i].handle)
	}
}

// loadTranslations is filled in with the dialogs, whose button texts need it.
func loadTranslations() {}
