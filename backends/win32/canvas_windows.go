//go:build windows

package win32

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/unxed/goWidgets/core"
	"golang.org/x/sys/windows"
)

const (
	wmPaintCanvas       = 0x000F
	wmEraseBkgndCanvas  = 0x0014
	wmCanvasLButtonDown = 0x0201
	wmCanvasLButtonUp   = 0x0202
	wmCanvasMButtonDown = 0x0207
	wmCanvasMButtonUp   = 0x0208
	wmCanvasRButtonDown = 0x0204
	wmCanvasRButtonUp   = 0x0205
	wmMouseMoveCanvas   = 0x0200
	wmMouseWheelCanvas  = 0x020A

	biRGBCanvas  = 0
	dibRGBCanvas = 0
	srccopy      = 0x00CC0020
	wheelDelta   = 120

	mkLButton   = 0x0001
	mkRButton   = 0x0002
	mkShift     = 0x0004
	mkControl   = 0x0008
	mkMButton   = 0x0010
	canvasVKAlt = 0x12
)

var (
	pBeginPaintCanvas     = user32.NewProc("BeginPaint")
	pEndPaintCanvas       = user32.NewProc("EndPaint")
	pSetCaptureCanvas     = user32.NewProc("SetCapture")
	pReleaseCaptureCanvas = user32.NewProc("ReleaseCapture")
	pGetCaptureCanvas     = user32.NewProc("GetCapture")
	pScreenToClientCanvas = user32.NewProc("ScreenToClient")
	pStretchDIBitsCanvas  = gdi32.NewProc("StretchDIBits")
	pCanvasWndProc        = syscall.NewCallback(canvasWndProc)
)

type bitmapInfoHeaderCanvas struct {
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
}

type bitmapInfoCanvas struct {
	Header bitmapInfoHeaderCanvas
	Colors [1]uint32
}

type paintStructCanvas struct {
	HDC         uintptr
	Erase       int32
	Paint       rectW
	Restore     int32
	IncUpdate   int32
	RGBReserved [32]byte
}

type pointCanvas struct{ X, Y int32 }

func registerCanvasClass(instance windows.Handle) error {
	name, err := windows.UTF16PtrFromString("goWidgetsCanvas")
	if err != nil {
		return err
	}
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   pCanvasWndProc,
		hInstance:     instance,
		hCursor:       windows.Handle(cursor),
		lpszClassName: name,
	}
	if r, _, callErr := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		if errno, ok := callErr.(syscall.Errno); !ok || errno != 1410 { // ERROR_CLASS_ALREADY_EXISTS
			return fmt.Errorf("RegisterClassExW(goWidgetsCanvas): %v", callErr)
		}
	}
	return nil
}

// PresentCanvas copies the reusable RGBA frame into a GDI-compatible BGRA DIB.
// The child window paints this buffer from WM_PAINT; no drawing API leaks into
// the application or core layout code.
func (w *window) PresentCanvas(h core.Handle, frame *core.CanvasFrame) {
	n := w.nodes[h]
	if n == nil || n.kind != core.KindCanvas || frame == nil || frame.Image == nil {
		return
	}
	width, height := int32(frame.Image.Rect.Dx()), int32(frame.Image.Rect.Dy())
	if width <= 0 || height <= 0 {
		return
	}
	pixels := make([]byte, width*height*4)
	if err := core.ConvertRGBA(pixels, frame.Image, core.PixelBGRA); err != nil {
		return
	}
	n.canvasPixels = pixels
	n.canvasWidth, n.canvasHeight = width, height
	pInvalidateRect.Call(n.hwnd, 0, 0)
	runtime.KeepAlive(pixels)
}

func canvasWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	d := theDrv
	if d == nil || d.win == nil {
		return canvasDefWindowProc(hwnd, msg, wParam, lParam)
	}
	h, n := d.win.nodeByHwnd(hwnd)
	if n == nil || n.kind != core.KindCanvas {
		return canvasDefWindowProc(hwnd, msg, wParam, lParam)
	}
	switch msg {
	case wmPaintCanvas:
		var ps paintStructCanvas
		hdc, _, _ := pBeginPaintCanvas.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 && len(n.canvasPixels) != 0 && n.canvasWidth > 0 && n.canvasHeight > 0 {
			var client rectW
			pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			info := bitmapInfoCanvas{Header: bitmapInfoHeaderCanvas{
				Size:        uint32(unsafe.Sizeof(bitmapInfoHeaderCanvas{})),
				Width:       n.canvasWidth,
				Height:      -n.canvasHeight, // top-down DIB; Canvas pixels are top-down too
				Planes:      1,
				BitCount:    32,
				Compression: biRGBCanvas,
				SizeImage:   uint32(len(n.canvasPixels)),
			}}
			pStretchDIBitsCanvas.Call(hdc,
				0, 0, uintptr(client.right-client.left), uintptr(client.bottom-client.top),
				0, 0, uintptr(n.canvasWidth), uintptr(n.canvasHeight),
				uintptr(unsafe.Pointer(&n.canvasPixels[0])), uintptr(unsafe.Pointer(&info)), dibRGBCanvas, srccopy)
			runtime.KeepAlive(n.canvasPixels)
		}
		pEndPaintCanvas.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBkgndCanvas:
		return 1 // keep the previous frame until the replacement is blitted
	case wmMouseMoveCanvas:
		d.win.emit(core.BackendEvent{Kind: core.EventMouseMove, H: h, Mouse: canvasMouseInfo(n, wParam, lParam, false)})
		return 0
	case wmCanvasLButtonDown, wmCanvasRButtonDown, wmCanvasMButtonDown:
		pSetFocus.Call(hwnd)
		pSetCaptureCanvas.Call(hwnd)
		mouse := canvasMouseInfo(n, wParam, lParam, false)
		mouse.Button = canvasMessageButton(msg)
		d.win.emit(core.BackendEvent{Kind: core.EventMouseDown, H: h, Mouse: mouse})
		return 0
	case wmCanvasLButtonUp, wmCanvasRButtonUp, wmCanvasMButtonUp:
		mouse := canvasMouseInfo(n, wParam, lParam, false)
		mouse.Button = canvasMessageButton(msg)
		d.win.emit(core.BackendEvent{Kind: core.EventMouseUp, H: h, Mouse: mouse})
		if capture, _, _ := pGetCaptureCanvas.Call(); capture == hwnd {
			pReleaseCaptureCanvas.Call()
		}
		return 0
	case wmMouseWheelCanvas:
		mouse := canvasMouseInfo(n, wParam, lParam, true)
		mouse.Delta = float64(int16(uint16(wParam>>16))) / wheelDelta
		d.win.emit(core.BackendEvent{Kind: core.EventMouseWheel, H: h, Mouse: mouse})
		return 0
	}
	return canvasDefWindowProc(hwnd, msg, wParam, lParam)
}

func canvasMouseInfo(n *node, wParam, lParam uintptr, screenPoint bool) core.MouseInfo {
	x := int32(int16(uint16(lParam)))
	y := int32(int16(uint16(lParam >> 16)))
	if screenPoint {
		p := pointCanvas{X: x, Y: y}
		if ok, _, _ := pScreenToClientCanvas.Call(n.hwnd, uintptr(unsafe.Pointer(&p))); ok != 0 {
			x, y = p.X, p.Y
		}
	}
	scale := 1.0
	if theDrv != nil && theDrv.win != nil {
		if s := theDrv.win.scale(); s > 0 {
			scale = s
		}
	}
	m := core.MouseInfo{X: float64(x) / scale, Y: float64(y) / scale}
	if wParam&mkShift != 0 {
		m.Mods |= core.ModShift
	}
	if wParam&mkControl != 0 {
		m.Mods |= core.ModControl
	}
	if alt, _, _ := pGetKeyState.Call(canvasVKAlt); int16(alt) < 0 {
		m.Mods |= core.ModAlt
	}
	return m
}

func canvasMessageButton(msg uint32) core.MouseButton {
	switch msg {
	case wmCanvasLButtonDown, wmCanvasLButtonUp:
		return core.MouseLeft
	case wmCanvasMButtonDown, wmCanvasMButtonUp:
		return core.MouseMiddle
	case wmCanvasRButtonDown, wmCanvasRButtonUp:
		return core.MouseRight
	default:
		return core.MouseNone
	}
}

func canvasDefWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
