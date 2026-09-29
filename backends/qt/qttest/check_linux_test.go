//go:build linux

package qttest

import (
	"testing"
	"time"

	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
)

// CheckBox both ways: the program's Checked reaches the box without coming
// back as a toggle, and the user's click comes back as Toggled and Checked.
func TestCheckBox(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	win, _ := app.NewWindow("check", 300, 120)
	cb, err := win.AddCheckBox("Гнать", true)
	if err != nil {
		t.Fatal(err)
	}
	var toggles []bool
	cb.Toggled.On(app.Scope(), func(v bool) { toggles = append(toggles, v) })

	isChecked := func() bool {
		return qt.Call(qt.Sym("_ZNK15QAbstractButton9isCheckedEv"), qt.WidgetHandle(core.KindCheckBox))&0xff != 0
	}
	var initial, programmatic bool
	var text string
	drive(app, 300*time.Millisecond,
		func() {
			initial = isChecked()
			text = qt.CallString(qt.Sym("_ZNK15QAbstractButton4textEv"), qt.WidgetHandle(core.KindCheckBox))
			cb.Checked.Set(false)
		},
		func() {
			programmatic = isChecked()
			qt.Call(qt.Sym("_ZN15QAbstractButton5clickEv"), qt.WidgetHandle(core.KindCheckBox))
		},
	)
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if !initial || text != "Гнать" {
		t.Errorf("initial state: checked=%v text=%q", initial, text)
	}
	if programmatic {
		t.Error("Checked.Set(false) did not reach the box")
	}
	if !cb.Checked.Get() || len(toggles) != 1 || !toggles[0] {
		t.Errorf("after a click: Checked=%v toggles=%v, want true [true]", cb.Checked.Get(), toggles)
	}
}
