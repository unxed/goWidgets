package goWidgets_test

import (
	"image/color"
	"testing"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/headless"
	"github.com/unxed/goWidgets/core"
	"github.com/unxed/goWidgets/vreactive"
)

func TestCanvasPaintScaleCoalescingAndMouse(t *testing.T) {
	app := newHeadlessApp(t)
	win, err := app.NewWindow("canvas", 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := win.AddCanvas()
	if err != nil {
		t.Fatal(err)
	}
	painted := 0
	var dims [][2]int
	canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
		painted++
		dims = append(dims, [2]int{frame.Image.Rect.Dx(), frame.Image.Rect.Dy()})
		frame.Image.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	})
	var got goWidgets.MouseInfo
	canvas.MouseDown.On(app.Scope(), func(info goWidgets.MouseInfo) { got = info })
	pumpUntilIdle(t, app)
	if painted != 1 || dims[0][0] == 0 || dims[0][1] == 0 {
		t.Fatalf("initial paint count/dimensions = %d/%v", painted, dims)
	}
	if frame := headless.PresentedCanvas(); frame == nil || frame.RGBAAt(0, 0).R != 255 {
		t.Fatal("headless backend did not present painted pixels")
	}
	canvas.Invalidate()
	canvas.Invalidate()
	canvas.Invalidate()
	pumpUntilIdle(t, app)
	if painted != 2 {
		t.Fatalf("coalesced invalidation painted %d times total; want 2", painted)
	}
	headless.SetScale(2)
	pumpUntilIdle(t, app)
	if canvas.Scale.Get() != 2 || painted != 3 {
		t.Fatalf("scale/paint = %v/%d; want 2/3", canvas.Scale.Get(), painted)
	}
	if dims[2][0] != 2*dims[0][0] || dims[2][1] != 2*dims[0][1] {
		t.Fatalf("scale-2 frame %v is not twice scale-1 dimensions %v", dims[2], dims[0])
	}
	if !headless.SendCanvasMouse(core.EventMouseDown, 15, 20, goWidgets.MouseLeft, goWidgets.ModShift, 0) {
		t.Fatal("mouse input did not hit the auto-laid-out canvas")
	}
	pumpUntilIdle(t, app)
	if got.X != 7 || got.Y != 12 || got.Button != goWidgets.MouseLeft || got.Mods != goWidgets.ModShift {
		t.Fatalf("mouse info = %+v; want relative DIP (7,12), left, shift", got)
	}
}

func TestCanvasEventSubscriptionsClose(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("canvas", 200, 160)
	canvas, err := win.AddCanvas()
	if err != nil {
		t.Fatal(err)
	}
	scope := vreactive.NewScope()
	called := false
	canvas.MouseMove.On(scope, func(goWidgets.MouseInfo) { called = true })
	if scope.Len() != 1 {
		t.Fatalf("subscription count = %d", scope.Len())
	}
	scope.Close()
	if scope.Len() != 0 {
		t.Fatalf("closed scope retained %d subscriptions", scope.Len())
	}
	pumpUntilIdle(t, app)
	if headless.SendCanvasMouse(core.EventMouseMove, 10, 10, goWidgets.MouseNone, 0, 0) {
		pumpUntilIdle(t, app)
	}
	if called {
		t.Fatal("event delivered after scope was closed")
	}
}
