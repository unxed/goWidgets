//go:build linux

package qttest

import (
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
)

// ComboBox against Qt, both variants. Items arrive with nothing selected; a
// program's Select reaches the widget and does not come back as a pick; the
// user's pick (Down on the closed box, through the X server) is Selected,
// Text and Changed; typing in the field is Changed without Selected.
func TestComboBox(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		t.Skip("нет xdotool")
	}
	win, _ := app.NewWindow("combo", 300, 120)
	items := []string{"gpt-5.6-luna", "gpt-5.6", "gpt-5.6-mini"}
	list, _ := win.AddComboBox(items, true)
	field, err := win.AddComboBox(items, false)
	if err != nil {
		t.Fatal(err)
	}
	var picks, typedPicks []int
	var changes, typed []string
	list.Selected.On(app.Scope(), func(i int) { picks = append(picks, i) })
	list.Changed.On(app.Scope(), func(s string) { changes = append(changes, s) })
	field.Selected.On(app.Scope(), func(i int) { typedPicks = append(typedPicks, i) })
	field.Changed.On(app.Scope(), func(s string) { typed = append(typed, s) })

	combo := func(i int) uintptr { return qt.NthWidgetHandle(core.KindComboBox, i) }
	index := func(i int) int { return int(int32(qt.Call(qt.Sym("_ZNK9QComboBox12currentIndexEv"), combo(i)))) }
	editable := func(i int) bool { return qt.Call(qt.Sym("_ZNK9QComboBox10isEditableEv"), combo(i))&0xff != 0 }
	count := func(i int) int { return int(int32(qt.Call(qt.Sym("_ZNK9QComboBox5countEv"), combo(i)))) }

	var start [2]int
	var afterSelect int
	var variants [2]bool
	var n int
	var wid uintptr
	keys := func(args ...string) {
		id := strconv.FormatUint(uint64(wid), 10)
		exec.Command(xdotool, "windowfocus", "--sync", id).Run()
		exec.Command(xdotool, args...).Run()
	}
	drive(app, 500*time.Millisecond,
		func() {
			start = [2]int{index(0), index(1)}
			variants = [2]bool{editable(0), editable(1)}
			n = count(0)
			wid = qt.Call(qt.Sym("_ZNK7QWidget5winIdEv"), qt.WindowHandle())
			list.Select(0)
		},
		func() {
			afterSelect = index(0)
			list.Focus()
			go keys("key", "Down")
		},
		func() { time.Sleep(800 * time.Millisecond) },
		func() {
			field.Focus()
			go keys("type", "--delay", "30", "o3")
		},
		func() { time.Sleep(800 * time.Millisecond) },
	)
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if start != [2]int{-1, -1} || n != 3 {
		t.Errorf("fresh boxes: current %v, count %d; want nothing selected of 3", start, n)
	}
	if variants != [2]bool{false, true} {
		t.Errorf("editable = %v, want dropdown-only then with a field", variants)
	}
	if afterSelect != 0 {
		t.Errorf("Select(0) did not reach the widget: %d", afterSelect)
	}
	if len(picks) != 1 || picks[0] != 1 || list.SelectedIndex() != 1 || list.Text.Get() != "gpt-5.6" {
		t.Errorf("pick: Selected=%v index=%d Text=%q", picks, list.SelectedIndex(), list.Text.Get())
	}
	if len(changes) != 1 || changes[0] != "gpt-5.6" {
		t.Errorf("pick: Changed = %q", changes)
	}
	if !equal(typed, []string{"o", "o3"}) || len(typedPicks) != 0 || field.Text.Get() != "o3" {
		t.Errorf("typing: Changed=%q Selected=%v Text=%q", typed, typedPicks, field.Text.Get())
	}
}
