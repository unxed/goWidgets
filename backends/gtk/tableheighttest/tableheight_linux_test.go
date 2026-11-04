//go:build linux

// Package tableheighttest checks Table.SetPrefHeight against GTK 3. It has its
// own test binary because GTK application state is process-global.
package tableheighttest

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
)

// A table with few rows must take the height it was given, not collapse to the
// height of its rows (unxed/goWidgets#43): the widget below it then sits under
// the table instead of overlapping it.
func TestTableTakesChosenHeightInGTK(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("нет DISPLAY: запускать под Xvfb")
	}
	app, err := goWidgets.NewApp()
	defer runtime.UnlockOSThread()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "gtk" {
		t.Skipf("драйвер %s, а проверяется gtk", app.Diagnostics().Name)
	}
	const want = 240
	win, err := app.NewWindow("tableheighttest", 420, 500)
	if err != nil {
		t.Fatal(err)
	}
	table, err := win.AddTable([]goWidgets.TableColumn{{Title: "Name"}})
	if err != nil {
		t.Fatal(err)
	}
	table.SetRows([][]string{{"first.txt"}, {"second.txt"}})
	table.SetPrefHeight(want)
	if _, err := win.AddEdit("below"); err != nil {
		t.Fatal(err)
	}

	lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		gtkWidgetGetParent          func(widget uintptr) uintptr
		gtkWidgetGetAllocatedHeight func(widget uintptr) int32
	)
	purego.RegisterLibFunc(&gtkWidgetGetParent, lib, "gtk_widget_get_parent")
	purego.RegisterLibFunc(&gtkWidgetGetAllocatedHeight, lib, "gtk_widget_get_allocated_height")

	var height int32
	var found bool
	go func() {
		time.Sleep(500 * time.Millisecond)
		app.QueueUpdate(func() {
			defer app.Quit()
			view := gtk.TableViewHandle()
			if view == 0 {
				return
			}
			found = true
			// The tree view sits in a scrolled window; the layout sizes the
			// scrolled window, which is what the user sees as the table.
			height = gtkWidgetGetAllocatedHeight(gtkWidgetGetParent(view))
		})
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("GTK table view not found")
	}
	if height < want {
		t.Errorf("table height = %d, want at least %d", height, want)
	}
}
