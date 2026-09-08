//go:build linux

package gtk

import (
	"unsafe"

	"github.com/unxed/winkeys"
)

// GTK delivers key-press-event with a GdkEventKey. The struct is read by
// offset rather than bound as a type: purego cannot describe a C struct, and
// the layout of this one has been stable for the whole of GTK 3.
type gdkEventKey struct {
	kind      int32   // GdkEventType
	_         int32   // padding to pointer alignment
	window    uintptr // GdkWindow *
	sendEvent int8
	_         [3]int8
	time      uint32
	state     uint32 // GdkModifierType
	keyval    uint32
	length    int32
	_         int32
	str       uintptr // gchar *
	hardware  uint16
	group     uint8
	bits      uint8
}

// GdkModifierType bits used here.
const (
	gdkShiftMask   = 1 << 0
	gdkLockMask    = 1 << 1 // caps lock
	gdkControlMask = 1 << 2
	gdkMod1Mask    = 1 << 3 // alt
)

// gdkKeyToVK translates a GDK keyval into a Win32 virtual key code.
//
// Only the keys a GUI actually dispatches on are mapped: everything printable
// arrives as a character anyway, and an unmapped key still reports its
// character so nothing is silently lost.
var gdkKeyToVK = map[uint32]uint16{
	0xff1b: winkeys.VK_ESCAPE,
	0xff0d: winkeys.VK_RETURN,
	0xff8d: winkeys.VK_RETURN, // keypad enter
	0xff09: winkeys.VK_TAB,
	0xff08: winkeys.VK_BACK,
	0xffff: winkeys.VK_DELETE,
	0xff63: winkeys.VK_INSERT,
	0xff50: winkeys.VK_HOME,
	0xff57: winkeys.VK_END,
	0xff55: winkeys.VK_PRIOR,
	0xff56: winkeys.VK_NEXT,
	0xff51: winkeys.VK_LEFT,
	0xff52: winkeys.VK_UP,
	0xff53: winkeys.VK_RIGHT,
	0xff54: winkeys.VK_DOWN,
	0x0020: winkeys.VK_SPACE,
	0xffbe: winkeys.VK_F1,
	0xffbf: winkeys.VK_F2,
	0xffc0: winkeys.VK_F3,
	0xffc1: winkeys.VK_F4,
	0xffc2: winkeys.VK_F5,
	0xffc3: winkeys.VK_F6,
	0xffc4: winkeys.VK_F7,
	0xffc5: winkeys.VK_F8,
	0xffc6: winkeys.VK_F9,
	0xffc7: winkeys.VK_F10,
	0xffc8: winkeys.VK_F11,
	0xffc9: winkeys.VK_F12,
}

// keyFromGdk builds a KeyEvent from a GdkEventKey pointer.
func keyFromGdk(p uintptr, down bool) (vk uint16, ch rune, state uint32, ok bool) {
	if p == 0 {
		return 0, 0, 0, false
	}
	ev := (*gdkEventKey)(unsafe.Pointer(p))

	if v, found := gdkKeyToVK[ev.keyval]; found {
		vk = v
	} else {
		switch {
		case ev.keyval >= 'a' && ev.keyval <= 'z':
			vk = uint16(ev.keyval - 'a' + 'A') // virtual keys are the uppercase letters
		case ev.keyval >= 'A' && ev.keyval <= 'Z',
			ev.keyval >= '0' && ev.keyval <= '9':
			vk = uint16(ev.keyval)
		}
	}
	// Printable keyvals below 0xff00 are their own Unicode code point in the
	// Latin-1 range, which covers the characters a shortcut is built from.
	if ev.keyval < 0xff00 {
		ch = rune(ev.keyval)
	}

	if ev.state&gdkShiftMask != 0 {
		state |= uint32(winkeys.ShiftPressed)
	}
	if ev.state&gdkControlMask != 0 {
		state |= uint32(winkeys.LeftCtrlPressed)
	}
	if ev.state&gdkMod1Mask != 0 {
		state |= uint32(winkeys.LeftAltPressed)
	}
	if ev.state&gdkLockMask != 0 {
		state |= uint32(winkeys.CapsLockOn)
	}
	return vk, ch, state, true
}
