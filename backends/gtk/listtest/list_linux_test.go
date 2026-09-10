//go:build linux

// Package listtest exercises ListBox against a real GTK. Own directory: one
// GTK application per test binary.
package listtest

import (
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
	"github.com/unxed/goWidgets/core"
)

// The list is driven from outside the way GTK drives it for a click
// (gtk_list_box_select_row) and an Enter/double-click (gtk_widget_activate
// on the row). The program's own Select must reach the widget and not come
// back as the user's.
func TestListBoxAgainstGTK(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("нет DISPLAY: запускать под Xvfb")
	}
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "gtk" {
		t.Skipf("драйвер %s, а проверяется gtk", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("list", 300, 200)
	if err != nil {
		t.Fatal(err)
	}
	lb, _ := win.AddListBox([]string{"починить CI", "рефакторинг", "тесты"})
	btn, _ := win.AddButton("Открыть") // a focusable neighbour, to show Focus() wins over GTK's own pick
	lb.Select(0)
	lb.Focus()
	_ = btn

	lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		gtkBinChild  func(bin uintptr) uintptr
		gtkRowAt     func(box uintptr, i int32) uintptr
		gtkSelectRow func(box, row uintptr)
		gtkSelected  func(box uintptr) uintptr
		gtkRowIndex  func(row uintptr) int32
		gtkActivate  func(w uintptr) int32
		gtkSizeAlloc func(w uintptr, alloc *[4]int32)
		gtkIsFocus   func(w uintptr) int32
	)
	purego.RegisterLibFunc(&gtkIsFocus, lib, "gtk_widget_is_focus")
	purego.RegisterLibFunc(&gtkBinChild, lib, "gtk_bin_get_child")
	purego.RegisterLibFunc(&gtkRowAt, lib, "gtk_list_box_get_row_at_index")
	purego.RegisterLibFunc(&gtkSelectRow, lib, "gtk_list_box_select_row")
	purego.RegisterLibFunc(&gtkSelected, lib, "gtk_list_box_get_selected_row")
	purego.RegisterLibFunc(&gtkRowIndex, lib, "gtk_list_box_row_get_index")
	purego.RegisterLibFunc(&gtkActivate, lib, "gtk_widget_activate")
	purego.RegisterLibFunc(&gtkSizeAlloc, lib, "gtk_widget_get_allocation")

	var picked, opened []int
	lb.Selected.On(app.Scope(), func(i int) { picked = append(picked, i) })
	lb.Activated.On(app.Scope(), func(i int) { opened = append(opened, i) })

	var selectedAtStart int32 = -2
	var alloc [4]int32
	var rowFocused bool
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			sw := gtk.WidgetHandle(core.KindListBox)
			gtkSizeAlloc(sw, &alloc)
			// The scrolled window holds a viewport that holds the list.
			box := gtkBinChild(gtkBinChild(sw))
			if row := gtkSelected(box); row != 0 {
				selectedAtStart = gtkRowIndex(row)
				rowFocused = gtkIsFocus(row) != 0
			}
			gtkSelectRow(box, gtkRowAt(box, 2)) // the click
			gtkActivate(gtkRowAt(box, 2))       // the Enter
		})
		time.Sleep(300 * time.Millisecond)
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if selectedAtStart != 0 {
		t.Errorf("Select(0) did not reach the widget: selected row %d", selectedAtStart)
	}
	if !rowFocused {
		// Observed before the fix: focus ended on the button, and Down did
		// nothing to the list.
		t.Error("Focus() before Show did not put keyboard focus on the selected row")
	}
	if len(picked) != 1 || picked[0] != 2 || lb.SelectedIndex() != 2 {
		t.Errorf("Selected=%v index=%d, want [2] and 2 (Select(0) not reported)", picked, lb.SelectedIndex())
	}
	if len(opened) != 1 || opened[0] != 2 {
		t.Errorf("Activated=%v, want [2]", opened)
	}
	// The flow gives the list the window's width; its height is its own
	// natural height, which must be rows, not the scroller's minimum.
	if alloc[3] < 60 {
		t.Errorf("list allocated only %d px tall", alloc[3])
	}
}
