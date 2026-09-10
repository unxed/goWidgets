//go:build linux

// Package entrytest exercises the text field against a real GTK. Its own
// directory for the reason given in keytest: one GTK application per test
// binary.
package entrytest

import (
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
	"github.com/unxed/goWidgets/core"
)

// The parts worth proving against GTK itself: gtk_entry_get_text comes back
// as a Go string through the FFI, "changed" fires for an edit made on the
// platform side and carries the whole text, "activate" fires for Enter, and
// a property write reaches the widget without bouncing back as an edit.
func TestEntryAgainstGTK(t *testing.T) {
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
	win, err := app.NewWindow("entry", 300, 100)
	if err != nil {
		t.Fatal(err)
	}
	e, err := win.AddEntry("start")
	if err != nil {
		t.Fatal(err)
	}

	lib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		gtkEntrySetText func(e uintptr, text string)
		gtkEntryGetText func(e uintptr) string
		gtkWidgetAct    func(w uintptr) int32
	)
	purego.RegisterLibFunc(&gtkEntrySetText, lib, "gtk_entry_set_text")
	purego.RegisterLibFunc(&gtkEntryGetText, lib, "gtk_entry_get_text")
	purego.RegisterLibFunc(&gtkWidgetAct, lib, "gtk_widget_activate")

	var changed, activated []string
	done := make(chan struct{}, 1)
	e.Changed.On(app.Scope(), func(v string) { changed = append(changed, v) })
	e.Activated.On(app.Scope(), func(v string) {
		activated = append(activated, v)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	var widgetText string
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			h := gtk.WidgetHandle(core.KindEntry)
			widgetText = gtkEntryGetText(h)
			// Property → widget.
			e.Text.Set("из программы")
			// Widget → property, as typing would.
			gtkEntrySetText(h, "из виджета")
			// Enter: gtk_widget_activate emits GtkEntry's activate signal.
			gtkWidgetAct(h)
		})
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		// One more turn so the last events reach their handlers.
		app.QueueUpdate(func() {})
		time.Sleep(100 * time.Millisecond)
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if widgetText != "start" {
		t.Errorf("initial text did not reach the widget: %q", widgetText)
	}
	if got := e.Text.Get(); got != "из виджета" {
		t.Errorf("Text = %q, want the widget-side edit", got)
	}
	// gtk_entry_set_text emits "changed" for both writes; only the field's
	// final contents matter to a listener, and the property must not have
	// pushed "из программы" back over "из виджета".
	if len(changed) == 0 || changed[len(changed)-1] != "из виджета" {
		t.Errorf("Changed = %v, want it to end on the widget-side edit", changed)
	}
	if len(activated) != 1 || activated[0] != "из виджета" {
		t.Errorf("Activated = %v, want [из виджета]", activated)
	}
}
