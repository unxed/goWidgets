//go:build linux

package qt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego"
)

// This file is the whole C++ story. Qt has no C API, so every call below is
// a call into an Itanium-mangled symbol, made the way a C++ compiler would
// make it:
//
//   - a member function takes `this` as its first argument;
//   - a class returned by value that is not trivially copyable (QString,
//     QIcon, QFont, QMetaObject::Connection) comes back through a hidden
//     pointer. Declaring the Go result as a struct larger than 16 bytes makes
//     the FFI use exactly that mechanism on both amd64 (rdi) and arm64 (x8),
//     whatever the real size of the class — see qvalue;
//   - a trivially copyable struct (QRect, QSize) comes back in registers,
//     like in C;
//   - a const reference is a pointer.
//
// Everything version-specific — the few names whose mangling differs between
// Qt 5 and Qt 6, the offsets of the event fields read here — sits in this
// file. Qt freezes the layout of its public classes for a whole major
// version (inline accessors read the fields directly, so 6.2-compiled code
// has to keep working against 6.8), which is what makes reading them by
// offset sound.

// ver is the major version the driver bound to: 5 or 6. Set once in load.
var ver int

// layout holds the offsets that differ between Qt 5 and Qt 6. They were read
// off objects built by a C++ probe against both versions' headers.
type layout struct {
	evType       uintptr // QEvent::t (ushort)
	evAccept     uintptr // byte holding QEvent::m_accept
	evAcceptBit  byte
	keyMods      uintptr // QInputEvent::modState
	keyText      uintptr // QKeyEvent::txt (QString)
	keyKey       uintptr // QKeyEvent::k
	resizeSize   uintptr // QResizeEvent::s (QSize)
	wheelDelta   uintptr // QWheelEvent::angleD/m_angleDelta QPoint
	translations int32   // QLibraryInfo::TranslationsPath
	plugins      int32   // QLibraryInfo::PluginsPath
}

type qPoint struct{ X, Y int32 }

var (
	qt5Layout = layout{evType: 16, evAccept: 18, evAcceptBit: 0x04, keyMods: 20, keyText: 32,
		keyKey: 40, resizeSize: 20, wheelDelta: 72, translations: 11, plugins: 6}
	qt6Layout = layout{evType: 8, evAccept: 12, evAcceptBit: 0x01, keyMods: 32, keyText: 40,
		keyKey: 64, resizeSize: 16, wheelDelta: 88, translations: 10, plugins: 6}
	lay layout
)

// QEvent::Type values used here; identical in Qt 5 and 6.
const (
	evMouseButtonPress   = 2
	evMouseButtonRelease = 3
	evMouseDoubleClick   = 4
	evMouseMove          = 5
	evKeyPress           = 6
	evKeyRelease         = 7
	evResize             = 14
	evClose              = 19
	evWheel              = 31
	evUser               = 1000 // QEvent::User: our wake-up
	evUserQuit           = 1001 // leave the main loop
)

// cptr turns an address that came from C (dlsym, a vtable slot) into an
// unsafe.Pointer. Reinterpreting the variable's storage instead of
// converting the integer is deliberate: the address is not Go memory, the
// GC ignores it either way, and vet's unsafeptr check is about Go memory.
func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

// qvalue receives a C++ class returned by value. Its size only has to exceed
// 16 bytes (so the FFI returns it through memory, as C++ does for classes
// with a destructor) and to hold the largest class returned here: QString
// and QVariant in Qt 6, 24 and 32 bytes.
type qvalue struct {
	w [6]uintptr
}

// qstring is a QString object, in Go memory. Qt 5 uses only d (a pointer to
// a QArrayData header followed by the UTF-16 text at d+offset); Qt 6 is
// {d, ptr, size}. The spare words make it a qvalue-sized return slot too.
type qstring struct {
	d    unsafe.Pointer
	ptr  unsafe.Pointer
	size int64
	_    [3]uintptr
}

// qArrayData5 is the header of Qt 5's QArrayData.
type qArrayData5 struct {
	ref    int32
	size   int32
	alloc  uint32
	_      uint32
	offset int64
}

var (
	qStringCtor     func(s *qstring, unicode *uint16, size int64)
	qArrayDealloc   func(d unsafe.Pointer, objectSize, alignment uintptr)
	cxxNew          func(size uintptr) unsafe.Pointer
	qObjectCtor     func(this unsafe.Pointer, parent uintptr)
	qConnectImpl    func(sender uintptr, signal *[2]uintptr, receiver uintptr, slotPtr uintptr, slot unsafe.Pointer, kind int32, types uintptr, meta uintptr) qvalue
	qConnectionDtor func(c *qvalue)
)

// newQString builds a QString holding s. The caller frees it.
func newQString(s string) *qstring {
	q := &qstring{}
	u := utf16.Encode([]rune(s))
	if len(u) == 0 {
		// A null QChar pointer makes a null string; Qt treats it as empty.
		qStringCtor(q, nil, 0)
		return q
	}
	qStringCtor(q, &u[0], int64(len(u)))
	return q
}

// String reads the text out of a QString.
func (q *qstring) String() string {
	var p unsafe.Pointer
	n := 0
	if ver == 6 {
		p, n = q.ptr, int(q.size)
	} else if q.d != nil {
		h := (*qArrayData5)(q.d)
		p, n = unsafe.Add(q.d, h.offset), int(h.size)
	}
	if p == nil || n <= 0 {
		return ""
	}
	return string(utf16.Decode(unsafe.Slice((*uint16)(p), n)))
}

// free releases the reference this QString holds — what ~QString, which is
// inline and therefore not a symbol anyone exports, would do.
func (q *qstring) free() {
	if q.d == nil {
		return
	}
	ref := (*int32)(q.d)
	if ver == 5 {
		switch c := atomic.LoadInt32(ref); c {
		case -1: // static data (the shared null and empty strings)
			q.d = nil
			return
		case 0: // unsharable: this was the only owner
		default:
			if atomic.AddInt32(ref, -1) != 0 {
				q.d = nil
				return
			}
		}
	} else if atomic.AddInt32(ref, -1) != 0 {
		q.d = nil
		return
	}
	qArrayDealloc(q.d, 2, 8) // sizeof(char16_t), alignof(QArrayData)
	q.d = nil
}

// qstr runs f with a temporary QString holding s.
func qstr(s string, f func(*qstring)) {
	q := newQString(s)
	f(q)
	q.free()
}

// takeString reads a QString returned by value and releases it.
func takeString(q qstring) string {
	s := q.String()
	q.free()
	return s
}

// ------------------------------------------------------------- libraries

type qtLibs struct {
	core, gui, widgets uintptr
}

var libs qtLibs

// load binds the Qt of the given major version. It resolves every symbol
// before anything is called, so a missing one is an Init error rather than a
// crash halfway through building a window.
func load(major int) error {
	names := map[int][3]string{
		5: {"libQt5Core.so.5", "libQt5Gui.so.5", "libQt5Widgets.so.5"},
		6: {"libQt6Core.so.6", "libQt6Gui.so.6", "libQt6Widgets.so.6"},
	}[major]
	var hs [3]uintptr
	for i, n := range names {
		// RTLD_LOCAL: two Qt majors must never interpose on each other,
		// should both end up loaded after a failed attempt.
		h, err := purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return fmt.Errorf("Qt %d not available: %w", major, err)
		}
		hs[i] = h
	}
	libs = qtLibs{core: hs[0], gui: hs[1], widgets: hs[2]}
	ver = major
	if major == 5 {
		lay = qt5Layout
	} else {
		lay = qt6Layout
	}
	r := resolver{}
	r.bindAll()
	if len(r.missing) > 0 {
		return fmt.Errorf("Qt %d: missing symbols: %s", major, strings.Join(r.missing, ", "))
	}
	return nil
}

// resolver looks symbols up in the widgets library and everything it pulls
// in (Gui, Core, libstdc++), remembering what was not found.
type resolver struct{ missing []string }

// sym returns the address of the first of names that exists, or 0.
func (r *resolver) sym(names ...string) uintptr {
	for _, n := range names {
		if a, err := purego.Dlsym(libs.widgets, n); err == nil && a != 0 {
			return a
		}
	}
	r.missing = append(r.missing, names[0])
	return 0
}

// fn binds a Go function variable to the first of names that exists.
func (r *resolver) fn(fptr any, names ...string) {
	if a := r.sym(names...); a != 0 {
		purego.RegisterFunc(fptr, a)
	}
}

func (r *resolver) optionalFn(fptr any, name string) bool {
	a, err := purego.Dlsym(libs.widgets, name)
	if err != nil || a == 0 {
		return false
	}
	purego.RegisterFunc(fptr, a)
	return true
}

// v picks the Qt 5 or the Qt 6 spelling of a name.
func v(qt5, qt6 string) string {
	if ver == 5 {
		return qt5
	}
	return qt6
}

// ------------------------------------------------------------- vtables

// vslot finds the slot index of a virtual function in a class's vtable, by
// looking for the function's own address among the slots. That keeps the
// indices out of the source: they differ between Qt 5 and Qt 6 and are
// nobody's API.
func vslot(vtable string, fn string, limit int) int {
	vt, err := purego.Dlsym(libs.widgets, vtable)
	if err != nil || vt == 0 {
		return -1
	}
	f, err := purego.Dlsym(libs.widgets, fn)
	if err != nil || f == 0 {
		return -1
	}
	// An Itanium vtable symbol starts with offset-to-top and the typeinfo
	// pointer; the object's vptr points past them, at slot 0.
	slots := unsafe.Add(cptr(vt), 16)
	for i := 0; i < limit; i++ {
		if *(*uintptr)(unsafe.Add(slots, i*8)) == f {
			return i
		}
	}
	return -1
}

// vfunc is the implementation of virtual slot i for the object obj.
func vfunc(obj uintptr, i int) uintptr {
	vptr := *(*unsafe.Pointer)(cptr(obj))
	return *(*uintptr)(unsafe.Add(vptr, i*8))
}

// vcall calls virtual slot i of obj with the given arguments and returns the
// integer result register.
func vcall(obj uintptr, i int, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(vfunc(obj, i), append([]uintptr{obj}, args...)...)
	return r
}

// ------------------------------------------------------------- signals

// A connection is QObject::connectImpl with a slot object we build by hand.
// QtPrivate::QSlotObjectBase is {QAtomicInt ref; ImplFn impl} in both
// majors, and impl(which, this, receiver, args, ret) is a plain function
// pointer — so a purego callback can be one. The signal is named the way
// the functor overload of connect names it: a pointer to member function
// ({address, 0} for a non-virtual one), which the sender's moc code compares
// against its own signals to find the index.

type slotFunc func(args *[4]unsafe.Pointer)

var (
	slotImpl  uintptr
	slotsByID = map[uintptr]slotFunc{}
)

func initSlots() {
	// which: 0 Destroy, 1 Call, 2 Compare.
	slotImpl = purego.NewCallback(func(which int32, self unsafe.Pointer, receiver uintptr, args *[4]unsafe.Pointer, ret *bool) uintptr {
		id := uintptr(self)
		switch which {
		case 0:
			delete(slotsByID, id) // the slot object itself is a few leaked bytes
		case 1:
			if f := slotsByID[id]; f != nil {
				f(args)
			}
		case 2:
			if ret != nil {
				*ret = false
			}
		}
		return 0
	})
}

// signal names one signal of one class.
type signal struct {
	fn   uintptr // the signal's function
	meta uintptr // Class::staticMetaObject
}

func (r *resolver) signal(class, mangled string) signal {
	return signal{
		fn:   r.sym(mangled),
		meta: r.sym("_ZN" + class + "16staticMetaObjectE"),
	}
}

// connect attaches f to sig on sender. The connection lives as long as the
// sender; Qt tells the slot object when it goes (Destroy).
func connect(sender uintptr, sig signal, f slotFunc) {
	so := cxxNew(16)
	*(*int32)(so) = 1
	*(*uintptr)(unsafe.Add(so, 8)) = slotImpl
	slotsByID[uintptr(so)] = f
	pmf := [2]uintptr{sig.fn, 0}
	const autoConnection = 0
	c := qConnectImpl(sender, &pmf, sender, 0, so, autoConnection, 0, sig.meta)
	qConnectionDtor(&c)
}

// ------------------------------------------------------------- events

// evType reads QEvent::type().
func evType(ev unsafe.Pointer) uint16 { return *(*uint16)(unsafe.Add(ev, lay.evType)) }

// evIgnore clears QEvent's accepted flag — QEvent::ignore(), which is inline.
func evIgnore(ev unsafe.Pointer) {
	b := (*byte)(unsafe.Add(ev, lay.evAccept))
	*b &^= lay.evAcceptBit
}

// ------------------------------------------------------------- environment

// displayReachable guards QApplication's constructor, which does not fail —
// it aborts the process — when it cannot open a display or find a platform
// plugin. Checking first is what lets Init fail over to the next driver.
func displayReachable(pluginDir string) error {
	platform := os.Getenv("QT_QPA_PLATFORM")
	wayland := os.Getenv("WAYLAND_DISPLAY")
	x11 := os.Getenv("DISPLAY")
	switch {
	case platform != "":
		// The user picked a platform on purpose (offscreen, vnc…): trust it.
		return nil
	case wayland == "" && x11 == "":
		return fmt.Errorf("no display (neither DISPLAY nor WAYLAND_DISPLAY is set)")
	}
	if os.Getenv("QT_QPA_PLATFORM_PLUGIN_PATH") != "" || os.Getenv("QT_PLUGIN_PATH") != "" || pluginDir == "" {
		return nil
	}
	plugins, _ := filepath.Glob(filepath.Join(pluginDir, "platforms", "*.so"))
	has := func(prefix string) bool {
		for _, p := range plugins {
			if strings.HasPrefix(filepath.Base(p), prefix) {
				return true
			}
		}
		return false
	}
	if (x11 != "" && has("libqxcb") && xReachable(x11) == nil) || (wayland != "" && has("libqwayland")) {
		return nil
	}
	if x11 != "" && has("libqxcb") {
		return xReachable(x11)
	}
	return fmt.Errorf("no Qt %d platform plugin for this display in %s", ver, filepath.Join(pluginDir, "platforms"))
}

// xReachable opens and closes an X connection through libxcb — the library
// Qt's xcb plugin connects with — for the same reason: Qt aborts on a
// DISPLAY it cannot open. A real connection, unlike a look at the socket,
// also covers authorisation and remote displays.
func xReachable(display string) error {
	xcb, err := purego.Dlopen("libxcb.so.1", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return fmt.Errorf("libxcb not available: %w", err)
	}
	var (
		xcbConnect    func(display *byte, screen *int32) uintptr
		xcbHasError   func(c uintptr) int32
		xcbDisconnect func(c uintptr)
	)
	purego.RegisterLibFunc(&xcbConnect, xcb, "xcb_connect")
	purego.RegisterLibFunc(&xcbHasError, xcb, "xcb_connection_has_error")
	purego.RegisterLibFunc(&xcbDisconnect, xcb, "xcb_disconnect")
	name := append([]byte(display), 0)
	c := xcbConnect(&name[0], nil)
	defer xcbDisconnect(c) // xcb_disconnect also frees a failed connection
	if c == 0 || xcbHasError(c) != 0 {
		return fmt.Errorf("X display %s is not reachable", display)
	}
	return nil
}
