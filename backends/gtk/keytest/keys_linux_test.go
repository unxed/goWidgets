//go:build linux

// Package keytest holds the GTK key-event test. It is a package of its own
// because GTK keeps process-global state and two applications in one test
// binary segfault (observed: the tray test and this one, each green alone,
// crash the process when run together).
package keytest

import (
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
	"github.com/unxed/winkeys"
)

// gdkEventKey mirrors GdkEventKey (see backends/gtk/keys_linux.go).
type gdkEventKey struct {
	kind      int32
	_         int32
	window    uintptr
	sendEvent int8
	_         [3]int8
	time      uint32
	state     uint32
	keyval    uint32
	length    int32
	_         int32
	str       uintptr
	hardware  uint16
	group     uint8
	bits      uint8
}

func dlopenAny(names ...string) (uintptr, error) {
	var last error
	for _, n := range names {
		h, err := purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		last = err
	}
	return 0, last
}

// The key callback receives the GdkEventKey as a typed pointer that the FFI
// layer converts from the C address (no uintptr→unsafe.Pointer cast in our
// code). That conversion is the one thing here worth proving against a real
// GTK: build a key event the way GTK would, push it through gtk_widget_event,
// and expect it to come out of KeyPressed as the right virtual key.
//
// Skips without a display; CI runs it under Xvfb.
func TestKeyEventArrivesAsTypedPointer(t *testing.T) {
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
	win, err := app.NewWindow("keys", 200, 100)
	if err != nil {
		t.Fatal(err)
	}

	lib, err := dlopenAny("libgtk-3.so.0", "libgtk-3.so")
	if err != nil {
		t.Fatal(err)
	}
	gdk, err := dlopenAny("libgdk-3.so.0", "libgdk-3.so")
	if err != nil {
		t.Fatal(err)
	}
	var (
		gdkEventNew   func(kind int32) *gdkEventKey
		gdkEventFree  func(ev *gdkEventKey)
		gtkGetWindow  func(w uintptr) uintptr
		gtkWidgetEvnt func(w uintptr, ev *gdkEventKey) int32
	)
	purego.RegisterLibFunc(&gdkEventNew, gdk, "gdk_event_new")
	purego.RegisterLibFunc(&gdkEventFree, gdk, "gdk_event_free")
	purego.RegisterLibFunc(&gtkGetWindow, lib, "gtk_widget_get_window")
	purego.RegisterLibFunc(&gtkWidgetEvnt, lib, "gtk_widget_event")
	const gdkKeyPress = 8 // GdkEventType

	got := make(chan goWidgets.Key, 4)
	seen := make(chan struct{}, 1)
	win.KeyPressed().On(app.Scope(), func(k goWidgets.Key) {
		got <- k
		select {
		case seen <- struct{}{}:
		default:
		}
	})

	go func() {
		// The window must be realized before it has a GdkWindow to stamp on
		// the event; Show from Run does that, so inject from the UI thread
		// once the loop is up.
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			ev := gdkEventNew(gdkKeyPress)
			ev.window = gtkGetWindow(gtk.WindowHandle())
			ev.keyval = 'a'
			ev.state = 0x4 // GDK_CONTROL_MASK
			gtkWidgetEvnt(gtk.WindowHandle(), ev)
			ev.window = 0 // gdk_event_free would unref a window it never ref'd
			gdkEventFree(ev)
		})
		select {
		case <-seen:
		case <-time.After(5 * time.Second):
		}
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	select {
	case k := <-got:
		if k.VirtualKeyCode != winkeys.VK_A || !k.KeyDown || !k.Ctrl() {
			t.Fatalf("got %+v, want Ctrl+A key-down", k)
		}
	default:
		t.Fatal("key event never reached KeyPressed")
	}
}
