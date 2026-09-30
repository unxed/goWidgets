//go:build darwin

package cocoatest

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/cocoa"
	"github.com/unxed/goWidgets/core"
)

func TestCanvasPaintAndPointer(t *testing.T) {
	onMain(func() {
		app := newApp(t)
		if app == nil {
			return
		}
		win, err := app.NewWindow("Cocoa Canvas", 360, 240)
		if err != nil {
			t.Error(err)
			return
		}
		canvas, err := win.AddCanvas()
		if err != nil {
			t.Error(err)
			return
		}
		if err := win.Constrain(
			canvas.Left().Eq(win.Left()),
			canvas.Top().Eq(win.Top()),
			canvas.Right().Eq(win.Right()),
			canvas.Bottom().Eq(win.Bottom()),
		); err != nil {
			t.Error(err)
			return
		}
		want := color.RGBA{R: 51, G: 102, B: 153, A: 255}
		canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
			for i := 0; i < len(frame.Image.Pix); i += 4 {
				copy(frame.Image.Pix[i:i+4], []byte{want.R, want.G, want.B, want.A})
			}
		})
		type pointer struct {
			kind core.EventKind
			info goWidgets.MouseInfo
		}
		var events []pointer
		canvas.MouseDown.On(app.Scope(), func(m goWidgets.MouseInfo) {
			events = append(events, pointer{core.EventMouseDown, m})
		})
		canvas.MouseUp.On(app.Scope(), func(m goWidgets.MouseInfo) {
			events = append(events, pointer{core.EventMouseUp, m})
		})
		canvas.MouseMove.On(app.Scope(), func(m goWidgets.MouseInfo) {
			events = append(events, pointer{core.EventMouseMove, m})
		})

		snapshot := ""
		if dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR"); dir != "" {
			snapshot = filepath.Join(dir, "cocoa-canvas.png")
		}
		var canvasBounds [4]float64
		drive(app, 500*time.Millisecond,
			func() {
				view := cocoa.WidgetHandle(core.KindCanvas)
				_, _, width, height := cocoa.Rect(view)
				canvasBounds = [4]float64{width, height, 0, 0}
				local := cocoa.Point(width/2, height/2)
				location := cocoa.Msg(view, "convertPoint:toView:", local, objc.ID(0))
				windowNumber := int64(cocoa.Msg(cocoa.WindowHandle(), "windowNumber"))
				sendMouse := func(kind uint64) {
					ev := cocoa.Msg(cocoa.Class("NSEvent"), "mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:",
						kind, location, uint64(0), float64(0), windowNumber, objc.ID(0), int64(1), int64(1), float32(1))
					cocoa.Msg(cocoa.WindowHandle(), "sendEvent:", ev)
				}
				sendMouse(5) // moved
				sendMouse(1) // left down
				sendMouse(2) // left up
				if snapshot != "" {
					if err := cocoa.Snapshot(snapshot); err != nil {
						t.Errorf("snapshot: %v", err)
					}
				}
			},
		)
		if err := app.Run(win); err != nil {
			t.Error(err)
		}
		if canvasBounds[0] < 300 || canvasBounds[1] < 180 {
			t.Errorf("Auto Layout did not fill the window: %v", canvasBounds[:2])
		}
		wantKinds := []core.EventKind{core.EventMouseMove, core.EventMouseDown, core.EventMouseUp}
		if len(events) != len(wantKinds) {
			t.Errorf("Canvas pointer events = %+v, want move/down/up", events)
		} else {
			for i, kind := range wantKinds {
				if events[i].kind != kind {
					t.Errorf("pointer event %d = %v, want %v", i, events[i].kind, kind)
				}
			}
			if events[1].info.Button != goWidgets.MouseLeft || events[1].info.X <= 0 || events[1].info.Y <= 0 {
				t.Errorf("mouse down info = %+v", events[1].info)
			}
		}
		if snapshot != "" {
			f, err := os.Open(snapshot)
			if err != nil {
				t.Errorf("open snapshot: %v", err)
				return
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				t.Errorf("decode snapshot: %v", err)
				return
			}
			got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2)).(color.RGBA)
			if got != want {
				t.Errorf("canvas snapshot center = %v, want %v", got, want)
			}
		}
	})
}
