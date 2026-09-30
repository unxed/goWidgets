//go:build windows

package canvastest

import (
	"errors"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/win32"
	"github.com/unxed/goWidgets/core"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	gdi32             = syscall.NewLazyDLL("gdi32.dll")
	pCanvasPost       = user32.NewProc("PostMessageW")
	pCanvasGetRect    = user32.NewProc("GetClientRect")
	pCanvasToScreen   = user32.NewProc("ClientToScreen")
	pCanvasUpdate     = user32.NewProc("UpdateWindow")
	pCanvasGetDC      = user32.NewProc("GetDC")
	pCanvasRelease    = user32.NewProc("ReleaseDC")
	pCanvasGetCapture = user32.NewProc("GetCapture")
	pCanvasCreateDC   = gdi32.NewProc("CreateCompatibleDC")
	pCanvasCreateDIB  = gdi32.NewProc("CreateDIBSection")
	pCanvasBitBlt     = gdi32.NewProc("BitBlt")
	pCanvasDeleteDC   = gdi32.NewProc("DeleteDC")
	pCanvasSelectObj  = gdi32.NewProc("SelectObject")
	pCanvasDeleteObj  = gdi32.NewProc("DeleteObject")
)

const (
	wmCanvasLButtonDown  = 0x0201
	wmCanvasLButtonUp    = 0x0202
	wmCanvasMouseMove    = 0x0200
	wmCanvasMouseWheel   = 0x020A
	canvasMKLButton      = 0x0001
	canvasMKMouseMove    = 0x0001
	canvasMKShift        = 0x0004
	canvasWheelDelta     = 120
	canvasSRCCOPY        = 0x00CC0020
	canvasBI_RGB         = 0
	canvasDIB_RGB_COLORS = 0
)

type canvasRect struct{ Left, Top, Right, Bottom int32 }
type canvasPoint struct{ X, Y int32 }
type canvasBitmapInfo struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	Colors        [1]uint32
}
type canvasInput struct {
	kind  core.EventKind
	mouse goWidgets.MouseInfo
}

func TestCanvasPaintAndPointerInput(t *testing.T) {
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("graphical subsystem unavailable: %v", err)
	}
	if app.Diagnostics().Name != "win32" {
		t.Skipf("selected %s, not Win32", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("Win32 Canvas test", 360, 260)
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := win.AddCanvas()
	if err != nil {
		t.Fatal(err)
	}
	if err := win.Constrain(
		canvas.Left().Eq(win.Left().Plus(8)), canvas.Top().Eq(win.Top().Plus(8)),
		canvas.Right().Eq(win.Right().Minus(8)), canvas.Bottom().Eq(win.Bottom().Minus(8)),
	); err != nil {
		t.Fatal(err)
	}
	want := [3]byte{51, 102, 153}
	canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
		for i := 0; i < len(frame.Image.Pix); i += 4 {
			frame.Image.Pix[i+0] = want[0]
			frame.Image.Pix[i+1] = want[1]
			frame.Image.Pix[i+2] = want[2]
			frame.Image.Pix[i+3] = 255
		}
	})
	inputs := make(chan canvasInput, 4)
	canvas.MouseDown.On(app.Scope(), func(m goWidgets.MouseInfo) { inputs <- canvasInput{core.EventMouseDown, m} })
	canvas.MouseUp.On(app.Scope(), func(m goWidgets.MouseInfo) { inputs <- canvasInput{core.EventMouseUp, m} })
	canvas.MouseMove.On(app.Scope(), func(m goWidgets.MouseInfo) { inputs <- canvasInput{core.EventMouseMove, m} })
	canvas.MouseWheel.On(app.Scope(), func(m goWidgets.MouseInfo) { inputs <- canvasInput{core.EventMouseWheel, m} })

	type result struct {
		pixel       uint32
		width       int32
		height      int32
		scale       float64
		centerX     int32
		centerY     int32
		screenX     int32
		screenY     int32
		window      uintptr
		captured    bool
		released    bool
		snapshotErr error
	}
	resultCh := make(chan result, 1)
	go func() {
		time.Sleep(400 * time.Millisecond)
		runOnUI := func(f func()) {
			done := make(chan struct{})
			app.QueueUpdate(func() { f(); close(done) })
			<-done
		}
		var r result
		runOnUI(func() {
			hwnd := win32.WidgetHandle(core.KindCanvas)
			r.window, r.scale = hwnd, canvas.Scale.Get()
			if hwnd == 0 {
				return
			}
			var bounds canvasRect
			if ok, _, _ := pCanvasGetRect.Call(hwnd, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
				return
			}
			r.width, r.height = bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
			r.centerX, r.centerY = r.width/2, r.height/2
			pCanvasUpdate.Call(hwnd) // force the first native paint, before pointer input
			dc, _, _ := pCanvasGetDC.Call(hwnd)
			if dc != 0 {
				snapshotPath := ""
				if dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR"); dir != "" {
					snapshotPath = filepath.Join(dir, "win32-canvas.png")
				}
				r.pixel, r.snapshotErr = captureSnapshot(dc, r.width, r.height, r.centerX, r.centerY, snapshotPath)
				pCanvasRelease.Call(hwnd, dc)
			}
			point := canvasPoint{X: r.centerX, Y: r.centerY}
			if ok, _, _ := pCanvasToScreen.Call(hwnd, uintptr(unsafe.Pointer(&point))); ok != 0 {
				r.screenX, r.screenY = point.X, point.Y
			}
			click := packPoint(r.centerX, r.centerY)
			pCanvasPost.Call(hwnd, wmCanvasLButtonDown, canvasMKLButton|canvasMKShift, click)
		})
		time.Sleep(60 * time.Millisecond)
		runOnUI(func() {
			capture, _, _ := pCanvasGetCapture.Call()
			r.captured = capture == r.window
			point := packPoint(r.centerX, r.centerY)
			pCanvasPost.Call(r.window, wmCanvasMouseMove, canvasMKMouseMove, point)
			pCanvasPost.Call(r.window, wmCanvasLButtonUp, 0, point)
			pCanvasPost.Call(r.window, wmCanvasMouseWheel, uintptr(canvasWheelDelta)<<16, packPoint(r.screenX, r.screenY))
		})
		time.Sleep(60 * time.Millisecond)
		runOnUI(func() {
			capture, _, _ := pCanvasGetCapture.Call()
			r.released = capture != r.window
		})
		resultCh <- r
		app.QueueUpdate(func() { app.Quit() })
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	r := <-resultCh
	if r.window == 0 {
		t.Fatal("Canvas HWND was not created")
	}
	if r.width <= 0 || r.height <= 0 {
		t.Fatalf("Auto Layout gave Canvas an empty client area: %dx%d", r.width, r.height)
	}
	if r.snapshotErr != nil {
		t.Fatal(r.snapshotErr)
	}
	wantPixel := uint32(want[0]) | uint32(want[1])<<8 | uint32(want[2])<<16
	if r.pixel != wantPixel {
		t.Fatalf("first Canvas pixel before pointer input = %#08x, want %#08x", r.pixel, wantPixel)
	}
	if !r.captured || !r.released {
		t.Errorf("mouse capture during drag = %v, released after up = %v; want true/true", r.captured, r.released)
	}
	if r.scale <= 0 {
		r.scale = 1
	}
	var got []canvasInput
	for len(got) < 4 {
		select {
		case input := <-inputs:
			got = append(got, input)
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("received %d/4 Canvas input events: %v", len(got), got)
		}
	}
	if got[0].kind != core.EventMouseDown || got[0].mouse.Button != goWidgets.MouseLeft || got[0].mouse.Mods&goWidgets.ModShift == 0 {
		t.Errorf("mouse down = %+v, want left button with Shift", got[0])
	}
	if got[1].kind != core.EventMouseMove {
		t.Errorf("mouse move = %+v, want mouse move", got[1])
	}
	if got[2].kind != core.EventMouseUp || got[2].mouse.Button != goWidgets.MouseLeft {
		t.Errorf("mouse up = %+v, want left button", got[2])
	}
	if got[3].kind != core.EventMouseWheel || got[3].mouse.Delta != 1 {
		t.Errorf("wheel = %+v, want delta +1", got[3])
	}
	wantX, wantY := float64(r.centerX)/r.scale, float64(r.centerY)/r.scale
	for _, input := range got {
		if math.Abs(input.mouse.X-wantX) > 1/r.scale || math.Abs(input.mouse.Y-wantY) > 1/r.scale {
			t.Errorf("%v coordinates = (%.2f, %.2f), want about (%.2f, %.2f) DIP", input.kind, input.mouse.X, input.mouse.Y, wantX, wantY)
		}
	}
}

func packPoint(x, y int32) uintptr {
	return uintptr(uint32(uint16(x)) | uint32(uint16(y))<<16)
}

func captureSnapshot(dc uintptr, width, height, centerX, centerY int32, path string) (uint32, error) {
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return 0, err
		}
	}
	memDC, _, _ := pCanvasCreateDC.Call(dc)
	if memDC == 0 {
		return 0, errors.New("CreateCompatibleDC failed while capturing Win32 Canvas")
	}
	defer pCanvasDeleteDC.Call(memDC)
	info := canvasBitmapInfo{Size: uint32(unsafe.Sizeof(canvasBitmapInfo{})), Width: width, Height: -height, Planes: 1, BitCount: 32, Compression: canvasBI_RGB}
	var pixelsPtr unsafe.Pointer
	bitmap, _, _ := pCanvasCreateDIB.Call(dc, uintptr(unsafe.Pointer(&info)), canvasDIB_RGB_COLORS, uintptr(unsafe.Pointer(&pixelsPtr)), 0, 0)
	if bitmap == 0 || pixelsPtr == nil {
		return 0, errors.New("CreateDIBSection failed while capturing Win32 Canvas")
	}
	defer pCanvasDeleteObj.Call(bitmap)
	oldBitmap, _, _ := pCanvasSelectObj.Call(memDC, bitmap)
	if oldBitmap == 0 {
		return 0, errors.New("SelectObject failed while capturing Win32 Canvas")
	}
	if ok, _, _ := pCanvasBitBlt.Call(memDC, 0, 0, uintptr(width), uintptr(height), dc, 0, 0, canvasSRCCOPY); ok == 0 {
		pCanvasSelectObj.Call(memDC, oldBitmap)
		return 0, errors.New("BitBlt failed while capturing Win32 Canvas")
	}
	pCanvasSelectObj.Call(memDC, oldBitmap)
	pixels := unsafe.Slice((*byte)(pixelsPtr), int(width*height*4))
	center := (int(centerY)*int(width) + int(centerX)) * 4
	centerPixel := uint32(pixels[center+2]) | uint32(pixels[center+1])<<8 | uint32(pixels[center])<<16
	if path == "" {
		return centerPixel, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	for i := 0; i < len(pixels); i += 4 {
		img.Pix[i+0], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i+0], 255
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return 0, err
	}
	return centerPixel, nil
}
