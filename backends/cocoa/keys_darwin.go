//go:build darwin

package cocoa

import (
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
	"github.com/unxed/winkeys"
)

// NSEventType values read here.
const eventFlagsChanged = 12

// onWindowEvent sees every event of our window before AppKit dispatches it.
// It reports keys and returns true only for an event it consumed.
func onWindowEvent(window, ev objc.ID) bool {
	kind := uint64(msg(ev, "type"))
	if kind != eventKeyDown && kind != eventKeyUp && kind != eventFlagsChanged {
		return false
	}
	kb := core.CurrentMacKeyboard()
	code := msgT[uint16](ev, "keyCode")
	flags := uint64(msg(ev, "modifierFlags"))

	if kind == eventFlagsChanged {
		// A modifier key alone, as GTK and Win32 report them.
		if code == kvkCapsLock {
			m := defaultMap[code]
			for _, down := range []bool{true, false} {
				emit(core.BackendEvent{Kind: core.EventKey, Key: core.KeyEvent{
					VirtualKeyCode: m.vk, VirtualScanCode: m.scan, KeyDown: down,
					ControlKeyState: controlState(flags, kb), RepeatCount: 1,
				}})
			}
			return false
		}
		m, bit, ok := modifierKey(code, kb)
		if !ok {
			return false
		}
		state := controlState(flags, kb)
		if m.scan&0x100 != 0 {
			state |= uint32(winkeys.EnhancedKey)
		}
		emit(core.BackendEvent{Kind: core.EventKey, Key: core.KeyEvent{
			VirtualKeyCode: genericVK(m.vk), VirtualScanCode: m.scan & 0xFF,
			KeyDown: deviceFlags(flags)&bit != 0, ControlKeyState: state, RepeatCount: 1,
		}})
		return false
	}

	down := kind == eventKeyDown
	k := keyEvent(code, flags,
		goString(msg(ev, "charactersIgnoringModifiers")), goString(msg(ev, "characters")), down, kb)
	emit(core.BackendEvent{Kind: core.EventKey, Key: k})

	// Enter on a list opens the selected item, as a double-click does;
	// NSTableView has no such action of its own.
	if down && k.VirtualKeyCode == winkeys.VK_RETURN {
		if h, n := lookup(msg(window, "firstResponder")); n != nil && n.kind == core.KindListBox {
			if i := int(int64(msg(n.inner, "selectedRow"))); i >= 0 {
				emit(core.BackendEvent{Kind: core.EventItemActivated, H: h, Int: i, Text: current.win.itemText(h, i)})
				return true
			}
		}
	}
	return false
}
