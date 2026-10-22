//go:build linux

// Package tabletest exercises the native table against GTK 3. It has its own
// test binary because GTK application state is process-global.
package tabletest

import (
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
)

func TestTableAgainstGTK(t *testing.T) {
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
	win, err := app.NewWindow("tabletest", 420, 220)
	if err != nil {
		t.Fatal(err)
	}
	table, err := win.AddTable([]goWidgets.TableColumn{
		{Title: "Name"},
		{Title: "Size", Align: goWidgets.AlignRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	table.SetRows([][]string{{"first.txt", "10"}, {"second.txt", "20"}, {"third.txt", "30"}})
	table.Select(0)
	table.SetSortIndicator(1, true)

	lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		gtkTreeViewGetModel       func(view uintptr) uintptr
		gtkTreeViewGetSelection   func(view uintptr) uintptr
		gtkTreeViewGetColumn      func(view uintptr, index int32) uintptr
		gtkTreeSelectionGet       func(selection uintptr, model *uintptr, iter *treeIter) int32
		gtkTreeSelectionSelect    func(selection, path uintptr)
		gtkTreeModelGetPath       func(model uintptr, iter *treeIter) uintptr
		gtkTreeViewRowActivated   func(view, path, column uintptr)
		gtkTreeViewColumnClicked  func(column uintptr)
		gtkTreeViewColumnGetAlign func(column uintptr) float32
		gtkTreeViewColumnGetSort  func(column uintptr) int32
		gtkTreeViewColumnGetOrder func(column uintptr) int32
		gtkTreeModelRowCount      func(model uintptr, parent *treeIter) int32
		gtkTreePathNew            func(path string) uintptr
		gtkTreePathGetIndices     func(path uintptr) *int32
		gtkTreePathFree           func(path uintptr)
	)
	purego.RegisterLibFunc(&gtkTreeViewGetModel, lib, "gtk_tree_view_get_model")
	purego.RegisterLibFunc(&gtkTreeViewGetSelection, lib, "gtk_tree_view_get_selection")
	purego.RegisterLibFunc(&gtkTreeViewGetColumn, lib, "gtk_tree_view_get_column")
	purego.RegisterLibFunc(&gtkTreeSelectionGet, lib, "gtk_tree_selection_get_selected")
	purego.RegisterLibFunc(&gtkTreeSelectionSelect, lib, "gtk_tree_selection_select_path")
	purego.RegisterLibFunc(&gtkTreeModelGetPath, lib, "gtk_tree_model_get_path")
	purego.RegisterLibFunc(&gtkTreeViewRowActivated, lib, "gtk_tree_view_row_activated")
	purego.RegisterLibFunc(&gtkTreeViewColumnClicked, lib, "gtk_tree_view_column_clicked")
	purego.RegisterLibFunc(&gtkTreeViewColumnGetAlign, lib, "gtk_tree_view_column_get_alignment")
	purego.RegisterLibFunc(&gtkTreeViewColumnGetSort, lib, "gtk_tree_view_column_get_sort_indicator")
	purego.RegisterLibFunc(&gtkTreeViewColumnGetOrder, lib, "gtk_tree_view_column_get_sort_order")
	purego.RegisterLibFunc(&gtkTreeModelRowCount, lib, "gtk_tree_model_iter_n_children")
	purego.RegisterLibFunc(&gtkTreePathNew, lib, "gtk_tree_path_new_from_string")
	purego.RegisterLibFunc(&gtkTreePathGetIndices, lib, "gtk_tree_path_get_indices")
	purego.RegisterLibFunc(&gtkTreePathFree, lib, "gtk_tree_path_free")

	var selected, activated, header []int
	table.Selected.On(app.Scope(), func(i int) { selected = append(selected, i) })
	table.Activated.On(app.Scope(), func(i int) { activated = append(activated, i) })
	table.HeaderClicked.On(app.Scope(), func(i int) { header = append(header, i) })

	screenshot := ""
	if dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		screenshot = filepath.Join(dir, "gtk-table.png")
	}
	var (
		initialSelection int32 = -1
		rows             int32
		rightAlignment   float32
		sortIndicator    int32
		sortOrder        int32
		captureErr       error
	)
	go func() {
		time.Sleep(350 * time.Millisecond)
		app.QueueUpdate(func() {
			view := gtk.TableViewHandle()
			if view == 0 {
				captureErr = errNoTable
				return
			}
			model := gtkTreeViewGetModel(view)
			rows = gtkTreeModelRowCount(model, nil)
			selection := gtkTreeViewGetSelection(view)
			var selectedModel uintptr
			var iter treeIter
			if gtkTreeSelectionGet(selection, &selectedModel, &iter) != 0 {
				path := gtkTreeModelGetPath(selectedModel, &iter)
				if path != 0 {
					if indices := gtkTreePathGetIndices(path); indices != nil {
						initialSelection = *indices
					}
					gtkTreePathFree(path)
				}
			}
			nameColumn := gtkTreeViewGetColumn(view, 0)
			sizeColumn := gtkTreeViewGetColumn(view, 1)
			rightAlignment = gtkTreeViewColumnGetAlign(sizeColumn)
			sortIndicator = gtkTreeViewColumnGetSort(sizeColumn)
			sortOrder = gtkTreeViewColumnGetOrder(sizeColumn)
			path := gtkTreePathNew("1")
			gtkTreeSelectionSelect(selection, path)
			gtkTreeViewRowActivated(view, path, nameColumn)
			gtkTreeViewColumnClicked(sizeColumn)
			gtkTreePathFree(path)
		})
		time.Sleep(350 * time.Millisecond)
		var screenshotErr error
		if screenshot != "" {
			windowIDs, err := exec.Command("xdotool", "search", "--onlyvisible", "--name", "^tabletest$").Output()
			if err != nil {
				screenshotErr = err
			} else {
				id := strings.TrimSpace(strings.Split(string(windowIDs), "\n")[0])
				if err := exec.Command("xdotool", "windowraise", id, "windowfocus", "--sync", id).Run(); err != nil {
					screenshotErr = err
				} else {
					screenshotErr = exec.Command("scrot", "-u", screenshot).Run()
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
		app.QueueUpdate(func() {
			if screenshotErr != nil {
				captureErr = screenshotErr
			}
			app.Quit()
		})
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if initialSelection != 0 {
		t.Errorf("programmatic selection = %d, want 0", initialSelection)
	}
	if rows != 3 {
		t.Errorf("GTK model has %d rows, want 3", rows)
	}
	if rightAlignment < 0.99 {
		t.Errorf("right-aligned column alignment = %v, want 1", rightAlignment)
	}
	if sortIndicator == 0 || sortOrder != 1 {
		t.Errorf("descending sort indicator = (%d, %d), want (1, 1)", sortIndicator, sortOrder)
	}
	if len(selected) != 1 || selected[0] != 1 || table.SelectedIndex() != 1 {
		t.Errorf("Selected=%v index=%d, want [1] and 1", selected, table.SelectedIndex())
	}
	if len(activated) != 1 || activated[0] != 1 {
		t.Errorf("Activated=%v, want [1]", activated)
	}
	if len(header) != 1 || header[0] != 1 {
		t.Errorf("HeaderClicked=%v, want [1]", header)
	}
	if captureErr != nil {
		t.Fatalf("GTK table interaction or screenshot failed: %v", captureErr)
	}
	if screenshot != "" {
		f, err := os.Open(screenshot)
		if err != nil {
			t.Fatal(err)
		}
		_, _, decodeErr := image.Decode(f)
		_ = f.Close()
		if decodeErr != nil {
			t.Fatalf("decode GTK table screenshot: %v", decodeErr)
		}
	}
}

type treeIter struct {
	Stamp     int32
	UserData  uintptr
	UserData2 uintptr
	UserData3 uintptr
}

var errNoTable = &noTableError{}

type noTableError struct{}

func (*noTableError) Error() string { return "GTK table view not found" }
