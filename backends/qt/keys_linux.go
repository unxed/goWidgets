//go:build linux

package qt

import (
	"unsafe"

	"github.com/unxed/goWidgets/core"
	"github.com/unxed/winkeys"
)

// Qt::Key values that are not their own character. Letters and digits are
// already their uppercase ASCII codes, which is what a virtual key code is.
var qtKeyToVK = map[uint32]uint16{
	0x01000000: winkeys.VK_ESCAPE,
	0x01000001: winkeys.VK_TAB,
	0x01000002: winkeys.VK_TAB, // Backtab: Shift+Tab, the modifier says which
	0x01000003: winkeys.VK_BACK,
	0x01000004: winkeys.VK_RETURN,
	0x01000005: winkeys.VK_RETURN, // keypad Enter
	0x01000006: winkeys.VK_INSERT,
	0x01000007: winkeys.VK_DELETE,
	0x01000008: winkeys.VK_PAUSE,
	0x01000010: winkeys.VK_HOME,
	0x01000011: winkeys.VK_END,
	0x01000012: winkeys.VK_LEFT,
	0x01000013: winkeys.VK_UP,
	0x01000014: winkeys.VK_RIGHT,
	0x01000015: winkeys.VK_DOWN,
	0x01000016: winkeys.VK_PRIOR,
	0x01000017: winkeys.VK_NEXT,
	0x01000020: winkeys.VK_SHIFT,
	0x01000021: winkeys.VK_CONTROL,
	0x01000022: winkeys.VK_LWIN, // Meta
	0x01000023: winkeys.VK_MENU, // Alt
	0x01000024: winkeys.VK_CAPITAL,
	0x01000025: winkeys.VK_NUMLOCK,
	0x01000026: winkeys.VK_SCROLL,
	0x01001103: winkeys.VK_MENU, // AltGr
	0x20:       winkeys.VK_SPACE,
}

// Qt::KeyboardModifier bits.
const (
	qtShift   = 0x02000000
	qtControl = 0x04000000
	qtAlt     = 0x08000000
)

const qtKeyF1 = 0x01000030

// keyFromQt reads a QKeyEvent. The fields are at fixed offsets within a Qt
// major (see abi_linux.go); the text is a QString inside the event.
func keyFromQt(ev unsafe.Pointer, down bool) core.KeyEvent {
	key := *(*uint32)(unsafe.Add(ev, lay.keyKey))
	mods := *(*uint32)(unsafe.Add(ev, lay.keyMods))
	text := (*qstring)(unsafe.Add(ev, lay.keyText)).String()

	var vk uint16
	switch {
	case qtKeyToVK[key] != 0:
		vk = qtKeyToVK[key]
	case key >= qtKeyF1 && key < qtKeyF1+24:
		vk = winkeys.VK_F1 + uint16(key-qtKeyF1)
	case key >= 'A' && key <= 'Z', key >= '0' && key <= '9':
		vk = uint16(key)
	}
	var ch rune
	if r := []rune(text); len(r) == 1 && r[0] >= 0x20 && r[0] != 0x7f {
		ch = r[0]
	} else if key >= 0x20 && key < 0x01000000 {
		// With Ctrl held the text is a control character or empty; the key
		// is still the character a shortcut is built from, in lowercase
		// unless Shift says otherwise, as GTK reports it.
		ch = rune(key)
		if mods&qtShift == 0 && ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
	}

	var state uint32
	if mods&qtShift != 0 {
		state |= uint32(winkeys.ShiftPressed)
	}
	if mods&qtControl != 0 {
		state |= uint32(winkeys.LeftCtrlPressed)
	}
	if mods&qtAlt != 0 {
		state |= uint32(winkeys.LeftAltPressed)
	}
	// RepeatCount stays 1: every auto-repeat arrives as an event of its own.
	return core.KeyEvent{VirtualKeyCode: vk, Char: ch, KeyDown: down, ControlKeyState: state, RepeatCount: 1}
}
