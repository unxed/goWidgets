//go:build linux

package qttest

import (
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
)

// The window as a whole: labels and buttons get their natural size from Qt,
// the solver's rectangles reach the widgets, a resize of the window re-runs
// the layout, a click comes back as Clicked, and a close request goes to
// Closing instead of closing.
func TestWindowLayoutAndClick(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	win, err := app.NewWindow("qt", 320, 160)
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
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

	var before, after, btn rect
	var labelText string
	drive(app, 400*time.Millisecond,
		func() {
			before = geometry(qt.WidgetHandle(core.KindButton))
			qt.Call(qt.Sym("_ZN15QAbstractButton5clickEv"), qt.WidgetHandle(core.KindButton))
		},
		func() {
			labelText = qt.CallString(qt.Sym("_ZNK6QLabel4textEv"), qt.WidgetHandle(core.KindLabel))
			// The title-bar cross, as far as the widget can tell.
			if qt.Call(qt.Sym("_ZN7QWidget5closeEv"), qt.WindowHandle())&0xff != 0 {
				t.Error("QWidget::close() went through")
			}
			// Grow the window; the button is pinned to the far corner.
			sz := [2]int32{420, 220}
			qt.Call(qt.Sym("_ZN7QWidget6resizeERK5QSize"), qt.WindowHandle(), uintptrOf(&sz))
		},
		func() {},
		func() {
			after = geometry(qt.WidgetHandle(core.KindButton))
			btn = geometry(qt.WidgetHandle(core.KindLabel))
		},
	)
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	t.Logf("button before %+v after %+v, label %+v", before, after, btn)
	if before.W <= 0 || before.H <= 0 {
		t.Fatalf("button has no natural size: %+v", before)
	}
	if got, want := before.X+before.W, 320-pad; got != want {
		t.Errorf("button right edge = %d, want %d", got, want)
	}
	if got, want := before.Y+before.H, 160-pad; got != want {
		t.Errorf("button bottom edge = %d, want %d", got, want)
	}
	if got, want := after.X+after.W, 420-pad; got != want {
		t.Errorf("after resize, button right edge = %d, want %d", got, want)
	}
	if got, want := after.Y+after.H, 220-pad; got != want {
		t.Errorf("after resize, button bottom edge = %d, want %d", got, want)
	}
	if btn.X != pad || btn.W != 420-2*pad {
		t.Errorf("label spans %+v, want x=%d w=%d", btn, pad, 420-2*pad)
	}
	if clicks != 1 || labelText != "Нажато" {
		t.Errorf("clicks = %d, label = %q", clicks, labelText)
	}
	if closes != 1 {
		t.Errorf("Closing fired %d times, want 1", closes)
	}
}
