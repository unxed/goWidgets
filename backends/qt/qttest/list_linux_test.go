//go:build linux

package qttest

import (
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
)

// ListBox against Qt: the program's Select reaches the list and is not a
// pick; the user moving the selection (Down through the X server) is
// Selected; Enter is Activated; SetItems clears the selection; the natural
// height is capped at eight rows, and in a column with a button under it the
// list is what stretches.
func TestListBox(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		t.Skip("нет xdotool")
	}
	win, _ := app.NewWindow("list", 300, 400)
	var items []string
	for i := 0; i < 12; i++ {
		items = append(items, fmt.Sprintf("цель %d", i))
	}
	lb, err := win.AddListBox(items)
	if err != nil {
		t.Fatal(err)
	}
	btn, _ := win.AddButton("Открыть")
	win.Constrain(goWidgets.Column(8, lb, btn)...)
	win.Constrain(btn.Bottom().Eq(win.Bottom().Minus(8)))

	var picks, opened []int
	lb.Selected.On(app.Scope(), func(i int) { picks = append(picks, i) })
	lb.Activated.On(app.Scope(), func(i int) { opened = append(opened, i) })

	list := func() uintptr { return qt.WidgetHandle(core.KindListBox) }
	row := func() int { return int(int32(qt.Call(qt.Sym("_ZNK11QListWidget10currentRowEv"), list()))) }
	var start, afterSelect, afterReset int
	var g rect
	var wid uintptr
	drive(app, 500*time.Millisecond,
		func() {
			start = row()
			g = geometry(list())
			wid = qt.Call(qt.Sym("_ZNK7QWidget5winIdEv"), qt.WindowHandle())
			lb.Select(2)
		},
		func() {
			afterSelect = row()
			lb.Focus()
			go func() {
				exec.Command(xdotool, "windowfocus", "--sync", strconv.FormatUint(uint64(wid), 10)).Run()
				exec.Command(xdotool, "key", "--delay", "50", "Down", "Return").Run()
			}()
		},
		func() { time.Sleep(800 * time.Millisecond) },
		func() { lb.SetItems(items[:3]) },
		func() { afterReset = row() },
	)
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if start != -1 || afterSelect != 2 {
		t.Errorf("current row: fresh %d, after Select(2) %d", start, afterSelect)
	}
	if len(picks) != 1 || picks[0] != 3 {
		t.Errorf("Selected = %v, want [3]", picks)
	}
	if len(opened) != 1 || opened[0] != 3 {
		t.Errorf("Activated = %v, want [3]", opened)
	}
	if afterReset != -1 || lb.SelectedIndex() != -1 {
		t.Errorf("after SetItems: row %d, SelectedIndex %d", afterReset, lb.SelectedIndex())
	}
	// The list stretches to fill the column above the button.
	if g.Y != 8 || g.H < 300 {
		t.Errorf("list geometry %+v: it should take the slack of a 400-high window", g)
	}
}
