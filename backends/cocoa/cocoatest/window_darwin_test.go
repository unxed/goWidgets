//go:build darwin

package cocoatest

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/cocoa"
	"github.com/unxed/goWidgets/core"
	"github.com/unxed/winkeys"
)

// drive runs steps on the UI thread one turn apart, then quits.
func drive(app *goWidgets.App, settle time.Duration, steps ...func()) {
	go func() {
		time.Sleep(settle)
		for _, s := range steps {
			done := make(chan struct{})
			app.QueueUpdate(func() { s(); close(done) })
			<-done
			time.Sleep(80 * time.Millisecond)
		}
		app.QueueUpdate(func() {})
		time.Sleep(100 * time.Millisecond)
		app.Quit()
	}()
}

type rect struct{ X, Y, W, H float64 }

func alignment(v objc.ID) rect {
	x, y, w, h := cocoa.Rect(v)
	return rect{x, y, w, h}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.51 }

// The window as a whole: controls get their natural size from AppKit, the
// solver's rectangles reach them (flipped: y grows downwards), a resize of
// the window re-runs the layout, a click is Clicked, and the title-bar
// close goes to Closing, which keeps the window.
func TestWindowLayoutAndClick(t *testing.T) {
	onMain(func() {
		app := newApp(t)
		if app == nil {
			return
		}
		win, err := app.NewWindow("cocoa", 320, 160)
		if err != nil {
			t.Error(err)
			return
		}
		status, _ := win.AddLabel("Готов")
		ok, _ := win.AddButton("OK")
		const pad = 8
		if err := win.Constrain(
			status.Left().Eq(win.Left().Plus(pad)),
			status.Top().Eq(win.Top().Plus(pad)),
			status.Right().Eq(win.Right().Minus(pad)),
			ok.Right().Eq(win.Right().Minus(pad)),
			ok.Bottom().Eq(win.Bottom().Minus(pad)),
		); err != nil {
			t.Error(err)
			return
		}
		clicks, closes := 0, 0
		ok.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
			clicks++
			status.Text.Set("Нажато")
		})
		win.Closing.On(app.Scope(), func(r *goWidgets.CloseRequest) {
			closes++
			r.Cancel = true
		})

		var before, after, label rect
		var labelText string
		var visibleAfterClose bool
		drive(app, 500*time.Millisecond,
			func() {
				before = alignment(cocoa.WidgetHandle(core.KindButton))
				cocoa.Msg(cocoa.WidgetHandle(core.KindButton), "performClick:", objc.ID(0))
			},
			func() {
				labelText = cocoa.GoString(cocoa.Msg(cocoa.WidgetHandle(core.KindLabel), "stringValue"))
				cocoa.Msg(cocoa.WindowHandle(), "performClose:", objc.ID(0))
				cocoa.Msg(cocoa.WindowHandle(), "setContentSize:", cocoa.Size(420, 220))
			},
			func() {},
			func() {
				visibleAfterClose = cocoa.MsgBool(cocoa.WindowHandle(), "isVisible")
				after = alignment(cocoa.WidgetHandle(core.KindButton))
				label = alignment(cocoa.WidgetHandle(core.KindLabel))
			},
		)
		if err := app.Run(win); err != nil {
			t.Error(err)
		}
		t.Logf("button before %+v after %+v, label %+v", before, after, label)
		if before.W <= 0 || before.H <= 0 {
			t.Errorf("button has no natural size: %+v", before)
		}
		if !near(before.X+before.W, 320-pad) || !near(before.Y+before.H, 160-pad) {
			t.Errorf("button %+v is not in the bottom right corner of 320×160", before)
		}
		if !near(after.X+after.W, 420-pad) || !near(after.Y+after.H, 220-pad) {
			t.Errorf("after resize, button %+v is not in the corner of 420×220", after)
		}
		if !near(label.X, pad) || !near(label.W, 420-2*pad) || !near(label.Y, pad) {
			t.Errorf("label %+v, want x=%d y=%d w=%d", label, pad, pad, 420-2*pad)
		}
		if clicks != 1 || labelText != "Нажато" {
			t.Errorf("clicks = %d, label = %q", clicks, labelText)
		}
		if closes != 1 || !visibleAfterClose {
			t.Errorf("Closing fired %d times, window visible after close: %v", closes, visibleAfterClose)
		}
	})
}

// The controls: CheckBox both ways; Edit — the program's text, typing
// through the field editor as the keyboard would, Enter; keys of the
// window by Wine's rules, Command as Alt and then, configured, as Ctrl;
// ComboBox both variants; ListBox; TextView. Then a snapshot of the
// window for the CI artifact.
func TestControls(t *testing.T) {
	onMain(func() {
		app := newApp(t)
		if app == nil {
			return
		}
		win, _ := app.NewWindow("controls", 460, 420)
		cb, _ := win.AddCheckBox("Гнать", true)
		e, _ := win.AddEdit("start")
		items := []string{"gpt-5.6-luna", "gpt-5.6", "gpt-5.6-mini"}
		popup, _ := win.AddComboBox(items, true)
		combo, _ := win.AddComboBox(items, false)
		lb, _ := win.AddListBox([]string{"цель 0", "цель 1", "цель 2", "цель 3"})
		tv, _ := win.AddTextView(60)
		tv.SetText("12:00 codex   старт\n12:01 codex   лимит")

		var toggles, picks, typedPicks, listPicks, opened []int
		var changed, activated, comboTyped []string
		var keys []goWidgets.Key
		cb.Toggled.On(app.Scope(), func(v bool) { toggles = append(toggles, map[bool]int{false: 0, true: 1}[v]) })
		e.Changed.On(app.Scope(), func(s string) { changed = append(changed, s) })
		e.Activated.On(app.Scope(), func(s string) { activated = append(activated, s) })
		popup.Selected.On(app.Scope(), func(i int) { picks = append(picks, i) })
		combo.Selected.On(app.Scope(), func(i int) { typedPicks = append(typedPicks, i) })
		combo.Changed.On(app.Scope(), func(s string) { comboTyped = append(comboTyped, s) })
		lb.Selected.On(app.Scope(), func(i int) { listPicks = append(listPicks, i) })
		lb.Activated.On(app.Scope(), func(i int) { opened = append(opened, i) })
		win.KeyPressed().On(app.Scope(), func(k goWidgets.Key) {
			if k.KeyDown {
				keys = append(keys, k)
			}
		})

		key := func(chars string, code uint16, flags uint64) {
			ev := cocoa.Msg(cocoa.Class("NSEvent"), "keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:",
				uint64(10), cocoa.Point(0, 0), flags, float64(0), cocoa.Msg(cocoa.WindowHandle(), "windowNumber"),
				objc.ID(0), cocoa.NSString(chars), cocoa.NSString(chars), false, code)
			cocoa.Msg(cocoa.WindowHandle(), "sendEvent:", ev)
		}
		var checkState, fieldText, programText, viewText string
		var editable bool
		var popupIndex, tableRow int
		drive(app, 500*time.Millisecond,
			func() {
				checkState = map[int64]string{0: "off", 1: "on"}[int64(cocoa.Msg(cocoa.WidgetHandle(core.KindCheckBox), "state"))]
				fieldText = cocoa.GoString(cocoa.Msg(cocoa.WidgetHandle(core.KindEdit), "stringValue"))
				cb.Checked.Set(false)
				e.Text.Set("")
			},
			func() {
				cocoa.Msg(cocoa.WidgetHandle(core.KindCheckBox), "performClick:", objc.ID(0))
				programText = cocoa.GoString(cocoa.Msg(cocoa.WidgetHandle(core.KindEdit), "stringValue"))
				e.Focus()
			},
			func() {
				// Typing, as the keyboard delivers it: through the field editor.
				ed := cocoa.Msg(cocoa.WidgetHandle(core.KindEdit), "currentEditor")
				if ed == 0 {
					t.Error("the field has no editor after Focus")
					return
				}
				for _, s := range []string{"a", "b", "c"} {
					cocoa.Msg(ed, "insertText:", cocoa.NSString(s))
				}
				cocoa.Msg(ed, "insertNewline:", objc.ID(0))
			},
			func() {
				// Keys for the window, not for the field that still has
				// focus after Enter: the window itself as first responder.
				cocoa.Msg(cocoa.WindowHandle(), "makeFirstResponder:", objc.ID(0))
				key("a", 0x00, 0)
				key("z", 0x06, 1<<20) // Command+Z: Alt+Z, as in Wine
				goWidgets.SetMacKeyboard(goWidgets.MacKeyboard{LeftCommandIsCtrl: true})
				key("z", 0x06, 1<<20) // and, configured, Ctrl+Z
				goWidgets.SetMacKeyboard(goWidgets.MacKeyboard{})
			},
			func() {
				p := cocoa.NthWidgetHandle(core.KindComboBox, 0)
				cocoa.Msg(p, "selectItemAtIndex:", int64(1))
				cocoa.Msg(p, "sendAction:to:", cocoa.Msg(p, "action"), cocoa.Msg(p, "target"))
				combo.Focus()
			},
			func() {
				ed := cocoa.Msg(cocoa.NthWidgetHandle(core.KindComboBox, 1), "currentEditor")
				if ed == 0 {
					t.Error("the combo box has no editor after Focus")
					return
				}
				cocoa.Msg(ed, "insertText:", cocoa.NSString("o3"))
			},
			func() {
				lb.Select(2)
			},
			func() {
				table := cocoa.InnerHandle(core.KindListBox, 0)
				tableRow = int(int64(cocoa.Msg(table, "selectedRow")))
				set := cocoa.Msg(cocoa.Class("NSIndexSet"), "indexSetWithIndex:", uint64(3))
				cocoa.Msg(table, "selectRowIndexes:byExtendingSelection:", set, false)
				cocoa.Msg(table, "sendAction:to:", cocoa.Msg(table, "doubleAction"), cocoa.Msg(table, "target"))
			},
			func() {
				popupIndex = int(int64(cocoa.Msg(cocoa.NthWidgetHandle(core.KindComboBox, 0), "indexOfSelectedItem")))
				text := cocoa.InnerHandle(core.KindTextView, 0)
				viewText = cocoa.GoString(cocoa.Msg(text, "string"))
				editable = cocoa.MsgBool(text, "isEditable")
				if dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR"); dir != "" {
					if err := cocoa.Snapshot(filepath.Join(dir, "controls.png")); err != nil {
						t.Errorf("snapshot: %v", err)
					}
				}
			},
		)
		if err := app.Run(win); err != nil {
			t.Error(err)
		}

		if checkState != "on" || fieldText != "start" {
			t.Errorf("initial: check %s, field %q", checkState, fieldText)
		}
		if len(toggles) != 1 || toggles[0] != 1 || !cb.Checked.Get() {
			t.Errorf("CheckBox: toggles %v, Checked %v; want one toggle to on", toggles, cb.Checked.Get())
		}
		if programText != "" {
			t.Errorf("Text.Set did not reach the field: %q", programText)
		}
		if want := []string{"a", "ab", "abc"}; !equal(changed, want) || len(activated) != 1 || activated[0] != "abc" {
			t.Errorf("Edit: Changed %q (want %q), Activated %q", changed, want, activated)
		}
		var sawA, sawAltZ, sawCtrlZ bool
		for _, k := range keys {
			switch {
			case k.VirtualKeyCode == winkeys.VK_A && k.Char == 'a' && k.ControlKeyState == 0:
				sawA = true
			case k.VirtualKeyCode == winkeys.VK_Z && k.Alt() && !k.Ctrl():
				sawAltZ = true
			case k.VirtualKeyCode == winkeys.VK_Z && k.Ctrl() && !k.Alt():
				sawCtrlZ = true
			}
		}
		if !sawA || !sawAltZ || !sawCtrlZ {
			t.Errorf("keys (a=%v Cmd+Z as Alt=%v Cmd+Z as Ctrl=%v): %+v", sawA, sawAltZ, sawCtrlZ, keys)
		}
		if len(picks) != 1 || picks[0] != 1 || popup.Text.Get() != "gpt-5.6" || popupIndex != 1 {
			t.Errorf("pop-up: Selected %v, Text %q, index %d", picks, popup.Text.Get(), popupIndex)
		}
		if !equal(comboTyped, []string{"o3"}) || len(typedPicks) != 0 {
			t.Errorf("combo typing: Changed %q, Selected %v", comboTyped, typedPicks)
		}
		if tableRow != 2 || !equalInts(listPicks, []int{3}) || !equalInts(opened, []int{3}) {
			t.Errorf("list: row after Select(2) %d, Selected %v, Activated %v", tableRow, listPicks, opened)
		}
		if viewText != "12:00 codex   старт\n12:01 codex   лимит" || editable {
			t.Errorf("TextView: %q, editable %v", viewText, editable)
		}
	})
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

func equalInts(a, b []int) bool {
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
