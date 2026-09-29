//go:build linux

package qttest

import (
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
	"github.com/unxed/winkeys"
)

// The text field against Qt, and the keyboard through it: focus asked for
// before the window is shown lands; real keystrokes (xdotool, through the X
// server) reach the field in order, come back as Changed per edit and as
// KeyPressed of the window; Enter is Activated; a property write reaches
// the field without coming back as an edit; Focus moves to a button.
func TestEditAndKeys(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		t.Skip("нет xdotool")
	}
	win, _ := app.NewWindow("edit", 300, 100)
	btn, _ := win.AddButton("Гнать")
	e, err := win.AddEdit("start")
	if err != nil {
		t.Fatal(err)
	}
	e.Focus()

	var changed, activated []string
	var keys []goWidgets.Key
	e.Changed.On(app.Scope(), func(v string) { changed = append(changed, v) })
	e.Activated.On(app.Scope(), func(v string) { activated = append(activated, v) })
	win.KeyPressed().On(app.Scope(), func(k goWidgets.Key) {
		if k.KeyDown {
			keys = append(keys, k)
		}
	})

	edit := func() uintptr { return qt.WidgetHandle(core.KindEdit) }
	text := func() string { return qt.CallString(qt.Sym("_ZNK9QLineEdit4textEv"), edit()) }
	var initial, programText, typed string
	var editFocused, buttonFocused bool
	var wid uintptr
	drive(app, 500*time.Millisecond,
		func() {
			initial = text()
			// No window manager under Xvfb, so the window is not active yet:
			// the request shows as the window's focus child.
			editFocused = qt.Call(qt.Sym("_ZNK7QWidget11focusWidgetEv"), qt.WindowHandle()) == edit()
			wid = qt.Call(qt.Sym("_ZNK7QWidget5winIdEv"), qt.WindowHandle())
			e.Text.Set("")
		},
		func() {
			programText = text()
			// xdotool runs off the UI thread: the loop has to keep going
			// for Qt to read the keystrokes it sends.
			go func() {
				id := strconv.FormatUint(uint64(wid), 10)
				exec.Command(xdotool, "windowfocus", "--sync", id).Run()
				exec.Command(xdotool, "type", "--delay", "30", "abc").Run()
				exec.Command(xdotool, "key", "ctrl+z", "Return").Run()
			}()
		},
		func() { time.Sleep(1500 * time.Millisecond) },
		func() {
			typed = text()
			btn.Focus()
			buttonFocused = qt.FocusWidget() == qt.WidgetHandle(core.KindButton)
		},
	)
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if initial != "start" || !editFocused {
		t.Errorf("before typing: text=%q focused=%v", initial, editFocused)
	}
	if programText != "" {
		t.Errorf("Text.Set did not reach the field: %q", programText)
	}
	// Ctrl+Z undoes the typing: an edit like any other.
	if want := []string{"a", "ab", "abc", ""}; !equal(changed, want) {
		t.Errorf("Changed = %q, want %q", changed, want)
	}
	if typed != "" || len(activated) != 1 || activated[0] != "" {
		t.Errorf("typed=%q activated=%q", typed, activated)
	}
	var sawA, sawCtrlZ, sawEnter bool
	for _, k := range keys {
		switch {
		case k.VirtualKeyCode == winkeys.VK_A && k.Char == 'a' && !k.Ctrl():
			sawA = true
		case k.VirtualKeyCode == winkeys.VK_Z && k.Ctrl() && k.Char == 'z':
			sawCtrlZ = true
		case k.VirtualKeyCode == winkeys.VK_RETURN:
			sawEnter = true
		}
	}
	if !sawA || !sawCtrlZ || !sawEnter {
		t.Errorf("KeyPressed missed keys (a=%v ctrl+z=%v enter=%v): %+v", sawA, sawCtrlZ, sawEnter, keys)
	}
	if !buttonFocused {
		t.Error("Focus() after Show did not move focus to the button")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
