package cocoa

import (
	"testing"

	"github.com/unxed/goWidgets/core"
	"github.com/unxed/winkeys"
)

// The translation is pure data plus rules, so it is checked here, on any
// platform, against what Wine's Mac driver produces for the same keys.
func TestKeysTranslateLikeWine(t *testing.T) {
	var wine core.MacKeyboard // Wine's defaults
	cmdCtrl := core.MacKeyboard{LeftCommandIsCtrl: true, RightCommandIsCtrl: true}
	optAlt := core.MacKeyboard{LeftOptionIsAlt: true}
	const (
		alt   = uint32(winkeys.LeftAltPressed)
		ralt  = uint32(winkeys.RightAltPressed)
		ctrl  = uint32(winkeys.LeftCtrlPressed)
		shift = uint32(winkeys.ShiftPressed)
		ext   = uint32(winkeys.EnhancedKey)
	)
	cases := []struct {
		name         string
		code         uint16
		flags        uint64
		plain, typed string
		kb           core.MacKeyboard
		vk, scan     uint16
		ch           rune
		state        uint32
	}{
		{"a", 0x00, 0, "a", "a", wine, 'A', 0x1E, 'a', 0},
		{"Shift+a", 0x00, flagShift | devLShift, "a", "A", wine, 'A', 0x1E, 'A', shift},
		// AZERTY: the key in the Q position types a, and is VK_A — Wine
		// takes a letter key's virtual key from the layout.
		{"AZERTY a", 0x0C, 0, "a", "a", wine, 'A', 0x10, 'a', 0},
		{"Cmd+z is Alt+z", 0x06, flagCommand | devLCmd, "z", "z", wine, 'Z', 0x2C, 'z', alt},
		{"Cmd+z as Ctrl+z", 0x06, flagCommand | devLCmd, "z", "z", cmdCtrl, 'Z', 0x2C, 'z', ctrl},
		{"right Cmd is right Alt", 0x06, flagCommand | devRCmd, "z", "z", wine, 'Z', 0x2C, 'z', ralt},
		{"synthesized Cmd, no side bits", 0x06, flagCommand, "z", "z", wine, 'Z', 0x2C, 'z', alt},
		{"Option types", 0x0E, flagOption | devLAlt, "e", "´", wine, 'E', 0x12, '´', 0},
		{"Option as Alt", 0x0E, flagOption | devLAlt, "e", "´", optAlt, 'E', 0x12, 'e', alt},
		{"Ctrl+c", 0x08, flagControl | devLCtrl, "c", "\x03", wine, 'C', 0x2E, 'c', ctrl},
		{"right arrow", 0x7C, 0, "", "", wine, winkeys.VK_RIGHT, 0x4D, 0, ext},
		{"keypad Enter", 0x4C, 0, "\x03", "\x03", wine, winkeys.VK_RETURN, 0x1C, 0, ext},
		{"Return", 0x24, 0, "\r", "\r", wine, winkeys.VK_RETURN, 0x1C, 0, 0},
		{"Help is Insert", 0x72, 0, "", "", wine, winkeys.VK_INSERT, 0x52, 0, ext},
		{"F5", 0x60, 0, "", "", wine, winkeys.VK_F5, 0x3F, 0, 0},
		{"minus", 0x1B, 0, "-", "-", wine, winkeys.VK_OEM_MINUS, 0x0C, '-', 0},
	}
	for _, c := range cases {
		k := keyEvent(c.code, c.flags, c.plain, c.typed, true, c.kb)
		if k.VirtualKeyCode != c.vk || k.VirtualScanCode != c.scan || k.Char != c.ch || k.ControlKeyState != c.state || !k.KeyDown {
			t.Errorf("%s: got vk=%#x scan=%#x ch=%q state=%#x, want vk=%#x scan=%#x ch=%q state=%#x",
				c.name, k.VirtualKeyCode, k.VirtualScanCode, k.Char, k.ControlKeyState, c.vk, c.scan, c.ch, c.state)
		}
	}
}

// A modifier pressed alone reports itself, by the same rules.
func TestModifierKeysLikeWine(t *testing.T) {
	var wine core.MacKeyboard
	cmdCtrl := core.MacKeyboard{LeftCommandIsCtrl: true, RightCommandIsCtrl: true}
	cases := []struct {
		name string
		code uint16
		kb   core.MacKeyboard
		ok   bool
		vk   uint16
		scan uint16
	}{
		{"Command is Alt", kvkCommand, wine, true, winkeys.VK_LMENU, 0x38},
		{"right Command is right Alt", kvkRightCommand, wine, true, winkeys.VK_RMENU, 0x138},
		{"Command as Ctrl", kvkCommand, cmdCtrl, true, winkeys.VK_LCONTROL, 0x1D},
		{"right Command as right Ctrl", kvkRightCommand, cmdCtrl, true, winkeys.VK_RCONTROL, 0x11D},
		{"Option is not a modifier", kvkOption, wine, false, 0, 0},
		{"Option as Alt", kvkOption, core.MacKeyboard{LeftOptionIsAlt: true}, true, winkeys.VK_LMENU, 0x38},
		{"Control", kvkControl, wine, true, winkeys.VK_LCONTROL, 0x1D},
		{"right Shift", kvkRightShift, wine, true, winkeys.VK_RSHIFT, 0x36},
	}
	for _, c := range cases {
		m, _, ok := modifierKey(c.code, c.kb)
		if ok != c.ok || (ok && (m.vk != c.vk || m.scan != c.scan)) {
			t.Errorf("%s: got ok=%v vk=%#x scan=%#x", c.name, ok, m.vk, m.scan)
		}
	}
	if genericVK(winkeys.VK_RMENU) != winkeys.VK_MENU || genericVK(winkeys.VK_LCONTROL) != winkeys.VK_CONTROL {
		t.Error("sided modifiers must fold into the generic virtual keys")
	}
}
