//go:build linux

// Package combotest exercises ComboBox against a real GTK. Own directory:
// one GTK application per test binary.
package combotest

import (
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
	"github.com/unxed/goWidgets/core"
)

// Two combo boxes: a dropdown-only one, picked from outside with
// gtk_combo_box_set_active as a click would; and one with an entry, typed
// into through its child GtkEntry. Program-side Select must reach the widget
// and must not come back as the user's pick.
func TestComboBoxAgainstGTK(t *testing.T) {
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
	win, err := app.NewWindow("combo", 300, 120)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := win.AddComboBox([]string{"luna", "sol", "terra"}, true)
	typed, _ := win.AddComboBox([]string{"alpha", "beta"}, false)
	list.Select(0)

	lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		gtkSetActive func(cb uintptr, i int32)
		gtkGetActive func(cb uintptr) int32
		gtkBinChild  func(bin uintptr) uintptr
		gtkEntrySet  func(e uintptr, text string)
	)
	purego.RegisterLibFunc(&gtkSetActive, lib, "gtk_combo_box_set_active")
	purego.RegisterLibFunc(&gtkGetActive, lib, "gtk_combo_box_get_active")
	purego.RegisterLibFunc(&gtkBinChild, lib, "gtk_bin_get_child")
	purego.RegisterLibFunc(&gtkEntrySet, lib, "gtk_entry_set_text")

	var listPicked, typedPicked []int
	var listChanged, typedChanged []string
	list.Selected.On(app.Scope(), func(i int) { listPicked = append(listPicked, i) })
	list.Changed.On(app.Scope(), func(v string) { listChanged = append(listChanged, v) })
	typed.Selected.On(app.Scope(), func(i int) { typedPicked = append(typedPicked, i) })
	typed.Changed.On(app.Scope(), func(v string) { typedChanged = append(typedChanged, v) })

	var activeAtStart int32 = -2
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			h := gtk.WidgetHandle(core.KindComboBox) // the first: the list
			activeAtStart = gtkGetActive(h)
			gtkSetActive(h, 2) // the user's pick
		})
		time.Sleep(200 * time.Millisecond)
		app.QueueUpdate(func() {
			// The second combo box's GtkEntry: typing.
			h := gtk.NthWidgetHandle(core.KindComboBox, 1)
			gtkEntrySet(gtkBinChild(h), "gam")
		})
		time.Sleep(300 * time.Millisecond)
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if activeAtStart != 0 {
		t.Errorf("Select(0) did not reach the widget: active=%d", activeAtStart)
	}
	if len(listPicked) != 1 || listPicked[0] != 2 || list.SelectedIndex() != 2 || list.Text.Get() != "terra" {
		t.Errorf("list: Selected=%v index=%d Text=%q", listPicked, list.SelectedIndex(), list.Text.Get())
	}
	if len(listChanged) != 1 || listChanged[0] != "terra" {
		t.Errorf("list: Changed=%v, want [terra] (Select(0) is the program's, not a change)", listChanged)
	}
	if len(typedPicked) != 0 || typed.Text.Get() != "gam" || len(typedChanged) == 0 || typedChanged[len(typedChanged)-1] != "gam" {
		t.Errorf("typed: Selected=%v Text=%q Changed=%v", typedPicked, typed.Text.Get(), typedChanged)
	}
}
