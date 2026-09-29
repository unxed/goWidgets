package core

import (
	"os"
	"strings"
	"sync"
)

// MacKeyboard says how the modifier keys of a Mac keyboard read in a
// KeyEvent. The switches are Wine's Mac driver options of the same names
// (HKCU\Software\Wine\Mac Driver), and so are the defaults, all off:
//
//   - Command reads as Alt (the Windows Alt sits where Command does);
//   - Option types characters, as on a Mac, and is not a modifier at all;
//   - Control is Ctrl.
//
// The common rearrangement — Command as Ctrl, so that the Cmd+C a Mac user
// presses reaches the program as the Ctrl+C a Windows program binds — is
// LeftCommandIsCtrl and RightCommandIsCtrl. Turn one Option into Alt with
// it, or there is no Alt left on the keyboard.
type MacKeyboard struct {
	LeftOptionIsAlt    bool
	RightOptionIsAlt   bool
	LeftCommandIsCtrl  bool
	RightCommandIsCtrl bool
}

var (
	macKbdMu  sync.Mutex
	macKbd    MacKeyboard
	macKbdSet bool
)

// SetMacKeyboard replaces the Mac keyboard options. It overrides the
// goWidgets_MAC_KEYBOARD environment variable and takes effect with the
// next key.
func SetMacKeyboard(k MacKeyboard) {
	macKbdMu.Lock()
	macKbd, macKbdSet = k, true
	macKbdMu.Unlock()
}

// CurrentMacKeyboard returns the options in force: what SetMacKeyboard set,
// or else what goWidgets_MAC_KEYBOARD names — the switches to turn on,
// separated by commas ("LeftCommandIsCtrl,RightCommandIsCtrl") — so a user
// can have the arrangement they are used to without the program asking.
func CurrentMacKeyboard() MacKeyboard {
	macKbdMu.Lock()
	defer macKbdMu.Unlock()
	if macKbdSet {
		return macKbd
	}
	return ParseMacKeyboard(os.Getenv("goWidgets_MAC_KEYBOARD"))
}

// ParseMacKeyboard reads a comma-separated list of switch names, as in
// goWidgets_MAC_KEYBOARD. Names are case-insensitive; unknown ones are
// ignored.
func ParseMacKeyboard(s string) MacKeyboard {
	var k MacKeyboard
	for _, name := range strings.Split(s, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "leftoptionisalt":
			k.LeftOptionIsAlt = true
		case "rightoptionisalt":
			k.RightOptionIsAlt = true
		case "leftcommandisctrl":
			k.LeftCommandIsCtrl = true
		case "rightcommandisctrl":
			k.RightCommandIsCtrl = true
		}
	}
	return k
}
