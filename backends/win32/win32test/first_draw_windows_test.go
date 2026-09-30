//go:build windows

package win32test

import (
	"errors"
	"image"
	"image/png"
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
	firstDrawUser32 = syscall.NewLazyDLL("user32.dll")
	firstDrawGDI32  = syscall.NewLazyDLL("gdi32.dll")

	pFirstDrawGetParent       = firstDrawUser32.NewProc("GetParent")
	pFirstDrawGetWindowRect   = firstDrawUser32.NewProc("GetWindowRect")
	pFirstDrawGetClientRect   = firstDrawUser32.NewProc("GetClientRect")
	pFirstDrawScreenToClient  = firstDrawUser32.NewProc("ScreenToClient")
	pFirstDrawSetCursorPos    = firstDrawUser32.NewProc("SetCursorPos")
	pFirstDrawIsWindowVisible = firstDrawUser32.NewProc("IsWindowVisible")
	pFirstDrawGetDC           = firstDrawUser32.NewProc("GetDC")
	pFirstDrawReleaseDC       = firstDrawUser32.NewProc("ReleaseDC")
	pFirstDrawCreateCompatDC  = firstDrawGDI32.NewProc("CreateCompatibleDC")
	pFirstDrawCreateDIB       = firstDrawGDI32.NewProc("CreateDIBSection")
	pFirstDrawSelectObject    = firstDrawGDI32.NewProc("SelectObject")
	pFirstDrawDeleteObject    = firstDrawGDI32.NewProc("DeleteObject")
	pFirstDrawDeleteDC        = firstDrawGDI32.NewProc("DeleteDC")
	pFirstDrawBitBlt          = firstDrawGDI32.NewProc("BitBlt")
)

type firstDrawPoint struct{ x, y int32 }
type firstDrawRect struct{ left, top, right, bottom int32 }

type firstDrawBitmapInfoHeader struct {
	size            uint32
	width           int32
	height          int32
	planes          uint16
	bitCount        uint16
	compression     uint32
	imageSize       uint32
	xPixelsPerM     int32
	yPixelsPerM     int32
	colorsUsed      uint32
	colorsImportant uint32
}

type firstDrawBitmapInfo struct {
	header firstDrawBitmapInfoHeader
	colors [1]uint32
}

func TestFirstDrawAndDelayedCheckboxRelayout(t *testing.T) {
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "win32" {
		t.Skipf("драйвер %s, а проверяется win32", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("win32-first-draw", 500, 300)
	if err != nil {
		t.Fatal(err)
	}
	button, err := win.AddButton("Нижняя кнопка")
	if err != nil {
		t.Fatal(err)
	}
	if err := win.Constrain(
		button.Left().Eq(win.Left().Plus(8)),
		button.Right().Eq(win.Right().Minus(8)),
		button.Bottom().Eq(win.Bottom().Minus(8)),
	); err != nil {
		t.Fatal(err)
	}
	checkbox, err := win.AddCheckBox("Цель закреплена", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := win.Constrain(
		checkbox.Left().Eq(win.Left().Plus(8)),
		checkbox.Right().Eq(win.Right().Minus(8)),
		checkbox.Top().Eq(win.Top().Plus(8)),
	); err != nil {
		t.Fatal(err)
	}
	toggled := make(chan bool, 1)
	checkbox.Toggled.On(app.Scope(), func(on bool) { toggled <- on })

	type result struct {
		buttonBounds  firstDrawRect
		buttonClient  firstDrawRect
		buttonVisible bool
		buttonImage   string
		checkBounds   firstDrawRect
		checkClient   firstDrawRect
		checkImage    string
		err           error
	}
	resultCh := make(chan result, 1)
	go func() {
		// Keep the pointer away from the button: the first observation must
		// precede any hover repaint that could hide a bad first frame.
		pFirstDrawSetCursorPos.Call(0, 0)
		time.Sleep(150 * time.Millisecond)
		initialCh := make(chan result, 1)
		clicked := make(chan struct{})
		app.QueueUpdate(func() {
			hwnd := win32.WidgetHandle(core.KindButton)
			parent, _, _ := pFirstDrawGetParent.Call(hwnd)
			r := result{}
			if hwnd == 0 || parent == 0 {
				r.err = errors.New("button or parent HWND is missing")
			} else if r.buttonBounds, r.buttonClient, r.err = firstDrawControlBounds(hwnd, parent); r.err == nil {
				visible, _, _ := pFirstDrawIsWindowVisible.Call(hwnd)
				r.buttonVisible = visible != 0
				dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR")
				if dir == "" {
					dir = os.TempDir()
				}
				r.buttonImage = filepath.Join(dir, "win32-first-draw.png")
				r.err = saveFirstDrawSnapshot(parent, r.buttonImage)
			}
			initialCh <- r
			if r.err == nil {
				// Simulate an actual user click on the native control.
				pSendMessage.Call(win32.WidgetHandle(core.KindCheckBox), 0x00F5, 0, 0) // BM_CLICK
			}
			close(clicked)
		})
		<-clicked
		initial := <-initialCh
		if initial.err != nil {
			resultCh <- initial
			app.Quit()
			return
		}
		select {
		case on := <-toggled:
			if !on {
				initial.err = errors.New("BM_CLICK did not check the checkbox")
				resultCh <- initial
				app.Quit()
				return
			}
		case <-time.After(3 * time.Second):
			initial.err = errors.New("timed out waiting for the native checkbox toggle")
			resultCh <- initial
			app.Quit()
			return
		}

		// The 700 ms wait matches Crescent's log/status refresh interval. A
		// changed label dirties intrinsic measurements and runs the layout
		// pipeline again after the click.
		time.Sleep(750 * time.Millisecond)
		updated := make(chan struct{})
		app.QueueUpdate(func() {
			checkbox.Text.Set("✓ Цель закреплена  [blocked]  12345 токенов — ждёт сброса лимита")
			close(updated)
		})
		<-updated
		time.Sleep(100 * time.Millisecond)

		app.QueueUpdate(func() {
			hwnd := win32.WidgetHandle(core.KindCheckBox)
			parent, _, _ := pFirstDrawGetParent.Call(hwnd)
			r := initial
			if hwnd == 0 || parent == 0 {
				r.err = errors.New("checkbox or parent HWND is missing")
			} else {
				r.checkBounds, r.checkClient, r.err = firstDrawControlBounds(hwnd, parent)
				dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR")
				if dir == "" {
					dir = os.TempDir()
				}
				r.checkImage = filepath.Join(dir, "win32-checkbox-delayed.png")
				if r.err == nil {
					r.err = saveFirstDrawSnapshot(parent, r.checkImage)
				}
			}
			resultCh <- r
			app.Quit()
		})
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	r := <-resultCh
	if r.err != nil {
		t.Fatal(r.err)
	}
	if !r.buttonVisible {
		t.Fatal("button was not visible on the first draw")
	}
	if r.buttonBounds.left < r.buttonClient.left || r.buttonBounds.top < r.buttonClient.top || r.buttonBounds.right > r.buttonClient.right || r.buttonBounds.bottom > r.buttonClient.bottom {
		t.Fatalf("first-draw button bounds %+v escape client area %+v; screenshot: %s", r.buttonBounds, r.buttonClient, r.buttonImage)
	}
	if r.buttonBounds.right-r.buttonBounds.left < 100 || r.buttonBounds.bottom-r.buttonBounds.top <= 10 {
		t.Fatalf("first-draw button kept creation-size bounds %+v; screenshot: %s", r.buttonBounds, r.buttonImage)
	}
	ink, err := countDarkPixelsInBottomBand(r.buttonImage)
	if err != nil {
		t.Fatal(err)
	}
	if ink < 40 {
		t.Fatalf("first-draw screenshot has no painted button pixels (%d dark pixels); screenshot: %s", ink, r.buttonImage)
	}
	if !checkbox.Checked.Get() {
		t.Fatal("checkbox lost its checked state after delayed text update")
	}
	if r.checkBounds.left < r.checkClient.left || r.checkBounds.top < r.checkClient.top || r.checkBounds.right > r.checkClient.right || r.checkBounds.bottom > r.checkClient.bottom {
		t.Fatalf("delayed checkbox bounds %+v escape client area %+v; screenshot: %s", r.checkBounds, r.checkClient, r.checkImage)
	}
	if r.checkBounds.right-r.checkBounds.left < 100 || r.checkBounds.bottom-r.checkBounds.top <= 10 {
		t.Fatalf("delayed checkbox kept creation-size bounds %+v; screenshot: %s", r.checkBounds, r.checkImage)
	}
	t.Logf("first frame button: %+v (screenshot: %s)", r.buttonBounds, r.buttonImage)
	t.Logf("checked checkbox survived delayed text relayout at %+v (screenshot: %s)", r.checkBounds, r.checkImage)
}

func firstDrawControlBounds(hwnd, parent uintptr) (bounds, client firstDrawRect, err error) {
	var screen firstDrawRect
	if ok, _, _ := pFirstDrawGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&screen))); ok == 0 {
		return bounds, client, errors.New("GetWindowRect(control) failed")
	}
	if ok, _, _ := pFirstDrawGetClientRect.Call(parent, uintptr(unsafe.Pointer(&client))); ok == 0 {
		return bounds, client, errors.New("GetClientRect(parent) failed")
	}
	points := [2]firstDrawPoint{{screen.left, screen.top}, {screen.right, screen.bottom}}
	for i := range points {
		if ok, _, _ := pFirstDrawScreenToClient.Call(parent, uintptr(unsafe.Pointer(&points[i]))); ok == 0 {
			return bounds, client, errors.New("ScreenToClient(control) failed")
		}
	}
	return firstDrawRect{points[0].x, points[0].y, points[1].x, points[1].y}, client, nil
}

func countDarkPixelsInBottomBand(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return 0, err
	}
	b := img.Bounds()
	count := 0
	for y := max(b.Min.Y, b.Max.Y-45); y < b.Max.Y-3; y++ {
		for x := b.Min.X + 4; x < b.Max.X-4; x++ {
			r, g, blue, _ := img.At(x, y).RGBA()
			if r < 0xC000 && g < 0xC000 && blue < 0xC000 {
				count++
			}
		}
	}
	return count, nil
}

func saveFirstDrawSnapshot(hwnd uintptr, path string) error {
	var windowRect firstDrawRect
	if ok, _, _ := pFirstDrawGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&windowRect))); ok == 0 {
		return errors.New("GetWindowRect(window) failed")
	}
	width, height := int(windowRect.right-windowRect.left), int(windowRect.bottom-windowRect.top)
	if width <= 0 || height <= 0 {
		return errors.New("window has empty snapshot dimensions")
	}
	// Copy the actual desktop pixels rather than asking the control to paint
	// into a memory DC; PrintWindow can trigger a fresh render and hide a
	// first-visible-frame defect.
	hdc, _, _ := pFirstDrawGetDC.Call(0)
	if hdc == 0 {
		return errors.New("GetDC(desktop) failed")
	}
	defer pFirstDrawReleaseDC.Call(0, hdc)
	memDC, _, _ := pFirstDrawCreateCompatDC.Call(hdc)
	if memDC == 0 {
		return errors.New("CreateCompatibleDC failed")
	}
	defer pFirstDrawDeleteDC.Call(memDC)

	info := firstDrawBitmapInfo{header: firstDrawBitmapInfoHeader{
		size: uint32(unsafe.Sizeof(firstDrawBitmapInfoHeader{})), width: int32(width), height: -int32(height),
		planes: 1, bitCount: 32,
	}}
	var pixels uintptr
	bmp, _, _ := pFirstDrawCreateDIB.Call(hdc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bmp == 0 || pixels == 0 {
		return errors.New("CreateDIBSection failed")
	}
	defer pFirstDrawDeleteObject.Call(bmp)
	old, _, _ := pFirstDrawSelectObject.Call(memDC, bmp)
	if old == 0 {
		return errors.New("SelectObject(bitmap) failed")
	}
	defer pFirstDrawSelectObject.Call(memDC, old)
	if ok, _, _ := pFirstDrawBitBlt.Call(memDC, 0, 0, uintptr(width), uintptr(height), hdc,
		uintptr(windowRect.left), uintptr(windowRect.top), 0x00CC0020); ok == 0 {
		return errors.New("BitBlt(desktop) failed before first-draw snapshot")
	}

	bgra := unsafe.Slice((*byte)(unsafe.Pointer(pixels)), width*height*4)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height; i++ {
		img.Pix[i*4+0] = bgra[i*4+2]
		img.Pix[i*4+1] = bgra[i*4+1]
		img.Pix[i*4+2] = bgra[i*4+0]
		img.Pix[i*4+3] = 0xff
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
