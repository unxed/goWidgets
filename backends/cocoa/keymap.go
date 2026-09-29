package cocoa

import (
	"github.com/unxed/goWidgets/core"
	"github.com/unxed/winkeys"
)

// Keys are translated the way Wine's Mac driver does it (dlls/winemac.drv,
// keyboard.c and cocoa_window.m): a Mac key code is a physical position,
// and defaultMap gives each one the Windows virtual key and set-1 scan code
// of the PC key in the same place, 0x100 marking an extended key. Keys that
// type a letter or a digit take their virtual key from the layout instead —
// on AZERTY the key in the Q position is VK_A — as Wine derives it from the
// keyboard layout for its non-fixed keys.
//
// Modifiers follow Wine too, including its four options (core.MacKeyboard):
// Command is Alt, Option types characters, unless configured otherwise.

// Virtual keys winkeys does not name.
const (
	vkVolumeMute   = 0xAD
	vkVolumeDown   = 0xAE
	vkVolumeUp     = 0xAF
	vkOEMNECEqual  = 0x92
	vkOEMReset     = 0xE9
	vkWineUnmapped = 0xFF
)

type macKey struct {
	vk    uint16
	scan  uint16 // set 1; 0x100: extended
	fixed bool   // not taken from the layout
}

// defaultMap is Wine's default_map, indexed by Mac key code (kVK_*).
var defaultMap = [128]macKey{
	{'A', 0x1E, false}, {'S', 0x1F, false}, {'D', 0x20, false}, {'F', 0x21, false}, // 0x00
	{'H', 0x23, false}, {'G', 0x22, false}, {'Z', 0x2C, false}, {'X', 0x2D, false},
	{'C', 0x2E, false}, {'V', 0x2F, false}, {winkeys.VK_OEM_102, 0x56, true}, {'B', 0x30, false}, // 0x08
	{'Q', 0x10, false}, {'W', 0x11, false}, {'E', 0x12, false}, {'R', 0x13, false},
	{'Y', 0x15, false}, {'T', 0x14, false}, {'1', 0x02, false}, {'2', 0x03, false}, // 0x10
	{'3', 0x04, false}, {'4', 0x05, false}, {'6', 0x07, false}, {'5', 0x06, false},
	{winkeys.VK_OEM_PLUS, 0x0D, false}, {'9', 0x0A, false}, {'7', 0x08, false}, {winkeys.VK_OEM_MINUS, 0x0C, false}, // 0x18
	{'8', 0x09, false}, {'0', 0x0B, false}, {winkeys.VK_OEM_6, 0x1B, false}, {'O', 0x18, false},
	{'U', 0x16, false}, {winkeys.VK_OEM_4, 0x1A, false}, {'I', 0x17, false}, {'P', 0x19, false}, // 0x20
	{winkeys.VK_RETURN, 0x1C, true}, {'L', 0x26, false}, {'J', 0x24, false}, {winkeys.VK_OEM_7, 0x28, false},
	{'K', 0x25, false}, {winkeys.VK_OEM_1, 0x27, false}, {winkeys.VK_OEM_5, 0x2B, false}, {winkeys.VK_OEM_COMMA, 0x33, false}, // 0x28
	{winkeys.VK_OEM_2, 0x35, false}, {'N', 0x31, false}, {'M', 0x32, false}, {winkeys.VK_OEM_PERIOD, 0x34, false},
	{winkeys.VK_TAB, 0x0F, true}, {winkeys.VK_SPACE, 0x39, true}, {winkeys.VK_OEM_3, 0x29, false}, {winkeys.VK_BACK, 0x0E, true}, // 0x30
	{0, 0, false}, {winkeys.VK_ESCAPE, 0x01, true}, {winkeys.VK_RMENU, 0x38 | 0x100, true}, {winkeys.VK_LMENU, 0x38, true},
	{winkeys.VK_LSHIFT, 0x2A, true}, {winkeys.VK_CAPITAL, 0x3A, true}, {0, 0, false}, {winkeys.VK_LCONTROL, 0x1D, true}, // 0x38
	{winkeys.VK_RSHIFT, 0x36, true}, {0, 0, false}, {winkeys.VK_RCONTROL, 0x1D | 0x100, true}, {0, 0, false},
	{winkeys.VK_F17, 0x68, true}, {winkeys.VK_DECIMAL, 0x53, true}, {0, 0, false}, {winkeys.VK_MULTIPLY, 0x37, true}, // 0x40
	{0, 0, false}, {winkeys.VK_ADD, 0x4E, true}, {0, 0, false}, {winkeys.VK_OEM_CLEAR, 0x59, true},
	{vkVolumeUp, 0x100, true}, {vkVolumeDown, 0x100, true}, {vkVolumeMute, 0x100, true}, {winkeys.VK_DIVIDE, 0x35 | 0x100, true}, // 0x48
	{winkeys.VK_RETURN, 0x1C | 0x100, true}, {0, 0, false}, {winkeys.VK_SUBTRACT, 0x4A, true}, {winkeys.VK_F18, 0x69, true},
	{winkeys.VK_F19, 0x6A, true}, {vkOEMNECEqual, 0x0D | 0x100, true}, {winkeys.VK_NUMPAD0, 0x52, true}, {winkeys.VK_NUMPAD1, 0x4F, true}, // 0x50
	{winkeys.VK_NUMPAD2, 0x50, true}, {winkeys.VK_NUMPAD3, 0x51, true}, {winkeys.VK_NUMPAD4, 0x4B, true}, {winkeys.VK_NUMPAD5, 0x4C, true},
	{winkeys.VK_NUMPAD6, 0x4D, true}, {winkeys.VK_NUMPAD7, 0x47, true}, {winkeys.VK_F20, 0x6B, true}, {winkeys.VK_NUMPAD8, 0x48, true}, // 0x58
	{winkeys.VK_NUMPAD9, 0x49, true}, {vkWineUnmapped, 0x7D, true}, {0xC1, 0x73, true}, {winkeys.VK_SEPARATOR, 0x7E, true},
	{winkeys.VK_F5, 0x3F, true}, {winkeys.VK_F6, 0x40, true}, {winkeys.VK_F7, 0x41, true}, {winkeys.VK_F3, 0x3D, true}, // 0x60
	{winkeys.VK_F8, 0x42, true}, {winkeys.VK_F9, 0x43, true}, {vkWineUnmapped, 0x72, true}, {winkeys.VK_F11, 0x57, true},
	{vkOEMReset, 0x71, true}, {winkeys.VK_F13, 0x64, true}, {winkeys.VK_F16, 0x67, true}, {winkeys.VK_F14, 0x65, true}, // 0x68
	{0, 0, false}, {winkeys.VK_F10, 0x44, true}, {0, 0, false}, {winkeys.VK_F12, 0x58, true},
	{0, 0, false}, {winkeys.VK_F15, 0x66, true}, {winkeys.VK_INSERT, 0x52 | 0x100, true}, {winkeys.VK_HOME, 0x47 | 0x100, true}, // 0x70
	{winkeys.VK_PRIOR, 0x49 | 0x100, true}, {winkeys.VK_DELETE, 0x53 | 0x100, true}, {winkeys.VK_F4, 0x3E, true}, {winkeys.VK_END, 0x4F | 0x100, true},
	{winkeys.VK_F2, 0x3C, true}, {winkeys.VK_NEXT, 0x51 | 0x100, true}, {winkeys.VK_F1, 0x3B, true}, {winkeys.VK_LEFT, 0x4B | 0x100, true}, // 0x78
	{winkeys.VK_RIGHT, 0x4D | 0x100, true}, {winkeys.VK_DOWN, 0x50 | 0x100, true}, {winkeys.VK_UP, 0x48 | 0x100, true}, {0, 0, false},
}

// Mac key codes of the modifier keys.
const (
	kvkRightCommand = 0x36
	kvkCommand      = 0x37
	kvkShift        = 0x38
	kvkCapsLock     = 0x39
	kvkOption       = 0x3A
	kvkControl      = 0x3B
	kvkRightShift   = 0x3C
	kvkRightOption  = 0x3D
	kvkRightControl = 0x3E
)

// NSEventModifierFlags: the device-independent bits, and the per-side
// device bits (IOKit's NX_DEVICE*KEYMASK) that Wine reads to tell left from
// right.
const (
	flagCapsLock = 1 << 16
	flagShift    = 1 << 17
	flagControl  = 1 << 18
	flagOption   = 1 << 19
	flagCommand  = 1 << 20

	devLCtrl  = 0x0001
	devLShift = 0x0002
	devRShift = 0x0004
	devLCmd   = 0x0008
	devRCmd   = 0x0010
	devLAlt   = 0x0020 // Option
	devRAlt   = 0x0040
	devRCtrl  = 0x2000
	devAll    = devLCtrl | devLShift | devRShift | devLCmd | devRCmd | devLAlt | devRAlt | devRCtrl
)

// deviceFlags returns the per-side bits, filled in from the generic ones
// where an event carries none (a synthesized one): then it is the left key.
func deviceFlags(flags uint64) uint64 {
	dev := flags & devAll
	fill := func(generic, any, left uint64) {
		if flags&generic != 0 && dev&any == 0 {
			dev |= left
		}
	}
	fill(flagShift, devLShift|devRShift, devLShift)
	fill(flagControl, devLCtrl|devRCtrl, devLCtrl)
	fill(flagOption, devLAlt|devRAlt, devLAlt)
	fill(flagCommand, devLCmd|devRCmd, devLCmd)
	return dev
}

// controlState is the Windows control-key state for the modifier flags, by
// Wine's rules: Command is Alt (or Ctrl, when configured), Option is
// nothing (or Alt, when configured), Control is Ctrl.
func controlState(flags uint64, kb core.MacKeyboard) uint32 {
	dev := deviceFlags(flags)
	var s uint32
	on := func(bit uint64) bool { return dev&bit != 0 }
	if on(devLShift) || on(devRShift) {
		s |= uint32(winkeys.ShiftPressed)
	}
	if on(devLCtrl) || (on(devLCmd) && kb.LeftCommandIsCtrl) {
		s |= uint32(winkeys.LeftCtrlPressed)
	}
	if on(devRCtrl) || (on(devRCmd) && kb.RightCommandIsCtrl) {
		s |= uint32(winkeys.RightCtrlPressed)
	}
	if (on(devLCmd) && !kb.LeftCommandIsCtrl) || (on(devLAlt) && kb.LeftOptionIsAlt) {
		s |= uint32(winkeys.LeftAltPressed)
	}
	if (on(devRCmd) && !kb.RightCommandIsCtrl) || (on(devRAlt) && kb.RightOptionIsAlt) {
		s |= uint32(winkeys.RightAltPressed)
	}
	if flags&flagCapsLock != 0 {
		s |= uint32(winkeys.CapsLockOn)
	}
	return s
}

// modifierKey is the virtual key and scan code a modifier key itself
// reports, and the device bit that says whether it is down; ok is false for
// a key that is not a Windows modifier (Option, unless it is Alt).
func modifierKey(code uint16, kb core.MacKeyboard) (m macKey, bit uint64, ok bool) {
	// defaultMap has Command as Alt (VK_LMENU, VK_RMENU); Control is Ctrl.
	switch code {
	case kvkShift:
		return defaultMap[code], devLShift, true
	case kvkRightShift:
		return defaultMap[code], devRShift, true
	case kvkControl:
		return defaultMap[code], devLCtrl, true
	case kvkRightControl:
		return defaultMap[code], devRCtrl, true
	case kvkCommand:
		if kb.LeftCommandIsCtrl {
			return defaultMap[kvkControl], devLCmd, true
		}
		return defaultMap[kvkCommand], devLCmd, true
	case kvkRightCommand:
		if kb.RightCommandIsCtrl {
			return defaultMap[kvkRightControl], devRCmd, true
		}
		return defaultMap[kvkRightCommand], devRCmd, true
	case kvkOption:
		return defaultMap[kvkCommand], devLAlt, kb.LeftOptionIsAlt
	case kvkRightOption:
		return defaultMap[kvkRightCommand], devRAlt, kb.RightOptionIsAlt
	}
	return macKey{}, 0, false
}

// genericVK folds the sided modifier keys into the generic ones, as the
// other drivers and Win32's own WM_KEYDOWN report them; the side is in the
// extended bit of the control-key state.
func genericVK(vk uint16) uint16 {
	switch vk {
	case winkeys.VK_LSHIFT, winkeys.VK_RSHIFT:
		return winkeys.VK_SHIFT
	case winkeys.VK_LCONTROL, winkeys.VK_RCONTROL:
		return winkeys.VK_CONTROL
	case winkeys.VK_LMENU, winkeys.VK_RMENU:
		return winkeys.VK_MENU
	}
	return vk
}

// keyEvent builds the KeyEvent for a key down or up.
func keyEvent(code uint16, flags uint64, plain, typed string, down bool, kb core.MacKeyboard) core.KeyEvent {
	m := macKey{}
	if int(code) < len(defaultMap) {
		m = defaultMap[code]
	}
	vk := m.vk
	p := []rune(plain)
	if !m.fixed && len(p) == 1 {
		switch c := p[0]; {
		case c >= 'a' && c <= 'z':
			vk = uint16(c - 'a' + 'A')
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			vk = uint16(c)
		}
	}
	state := controlState(flags, kb)
	if m.scan&0x100 != 0 {
		state |= uint32(winkeys.EnhancedKey)
	}

	// The character: what the key typed, with Shift and Option (as a Mac
	// types) — or, with Ctrl or Alt held, the plain key, as GTK reports a
	// shortcut's letter.
	var ch rune
	src := []rune(typed)
	if state&uint32(winkeys.LeftCtrlPressed|winkeys.RightCtrlPressed|winkeys.LeftAltPressed|winkeys.RightAltPressed) != 0 {
		src = p
	}
	if len(src) == 1 && src[0] >= 0x20 && src[0] != 0x7F && (src[0] < 0xF700 || src[0] > 0xF8FF) {
		ch = src[0] // 0xF700…0xF8FF are AppKit's function-key codes
	}
	return core.KeyEvent{
		VirtualKeyCode:  genericVK(vk),
		VirtualScanCode: m.scan & 0xFF,
		Char:            ch,
		KeyDown:         down,
		ControlKeyState: state,
		RepeatCount:     1,
	}
}
