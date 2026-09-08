//go:build windows

// Package win32 is the Windows driver.
//
// No FFI library is needed here: golang.org/x/sys/windows reaches user32/gdi32
// directly and syscall.NewCallback builds the window procedure, all with
// CGO_ENABLED=0.
//
// Win32 has no layout manager, so the "core owns layout" rule of §2.3 costs
// nothing: SetWindowPos is exactly the primitive ApplyLayout wants.
package win32

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"github.com/unxed/goWidgets/core"
	"golang.org/x/sys/windows"
)

func init() { core.RegisterDriver("win32", func() core.PlatformDriver { return &driver{} }) }

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pGetMessageW        = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessageW   = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pPostMessageW       = user32.NewProc("PostMessageW")
	pSendMessageW       = user32.NewProc("SendMessageW")
	pShowWindow         = user32.NewProc("ShowWindow")
	pSetWindowTextW     = user32.NewProc("SetWindowTextW")
	pGetKeyState        = user32.NewProc("GetKeyState")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pEnableWindow       = user32.NewProc("EnableWindow")
	pLoadCursorW        = user32.NewProc("LoadCursorW")
	pSysParamsInfoW     = user32.NewProc("SystemParametersInfoW")
	pGetDpiForWindow    = user32.NewProc("GetDpiForWindow")
	pSetProcessDpiCtx   = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	pGetDC              = user32.NewProc("GetDC")
	pReleaseDC          = user32.NewProc("ReleaseDC")

	pCreateFontIndirectW = gdi32.NewProc("CreateFontIndirectW")
	pSelectObject        = gdi32.NewProc("SelectObject")
	pGetTextExtentPoint  = gdi32.NewProc("GetTextExtentPoint32W")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateActCtxW    = kernel32.NewProc("CreateActCtxW")
	pActivateActCtx   = kernel32.NewProc("ActivateActCtx")

	pInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	ssLeftNoWordWrap   = 0x0000000C

	wmDestroy    = 0x0002
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	wmSize       = 0x0005
	wmClose      = 0x0010
	wmSetFont    = 0x0030
	wmCommand    = 0x0111
	wmDpiChanged = 0x02E0
	wmApp        = 0x8000
	wmWake       = wmApp + 1
	wmEndLoop    = wmApp + 2

	bcmGetIdealSize = 0x1601
	bsAutoCheckBox  = 0x00000003
	esMultiline     = 0x00000004
	esReadonly      = 0x00000800
	esAutoVScroll   = 0x00000040
	wsVScroll       = 0x00200000
	wsBorder        = 0x00800000
	bmGetCheck      = 0x00F0
	bmSetCheck      = 0x00F1

	swHide        = 0
	swShow        = 5
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpHideWindow = 0x0080
	swpShowWindow = 0x0040

	idcArrow        = 32512
	colorBtnFace    = 15
	hwndMessageOnly = ^uintptr(2) // (HWND)-3
	cwUseDefault    = ^uintptr(0x7FFFFFFF)
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type msgW struct {
	hwnd    windows.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type rectW struct{ left, top, right, bottom int32 }
type sizeW struct{ cx, cy int32 }

type logFontW struct {
	lfHeight         int32
	lfWidth          int32
	lfEscapement     int32
	lfOrientation    int32
	lfWeight         int32
	lfItalic         byte
	lfUnderline      byte
	lfStrikeOut      byte
	lfCharSet        byte
	lfOutPrecision   byte
	lfClipPrecision  byte
	lfQuality        byte
	lfPitchAndFamily byte
	lfFaceName       [32]uint16
}

type nonClientMetricsW struct {
	cbSize           uint32
	iBorderWidth     int32
	iScrollWidth     int32
	iScrollHeight    int32
	iCaptionWidth    int32
	iCaptionHeight   int32
	lfCaptionFont    logFontW
	iSmCaptionWidth  int32
	iSmCaptionHeight int32
	lfSmCaptionFont  logFontW
	iMenuWidth       int32
	iMenuHeight      int32
	lfMenuFont       logFontW
	lfStatusFont     logFontW
	lfMessageFont    logFontW
	iPaddingBorder   int32
}

type actCtxW struct {
	cbSize                 uint32
	dwFlags                uint32
	lpSource               *uint16
	wProcessorArchitecture uint16
	wLangID                uint16
	lpAssemblyDirectory    *uint16
	lpResourceName         *uint16
	lpApplicationName      *uint16
	hModule                windows.Handle
}

type initCommonControlsExT struct{ dwSize, dwICC uint32 }

type driver struct {
	hInstance windows.Handle
	className *uint16
	msgWnd    windows.Handle
	font      windows.Handle
	win       *window

	pumpMu sync.Mutex
	pump   func()
}

var (
	regMu    sync.Mutex
	theDrv   *driver
	ctlByID  = map[uint32]core.Handle{}
	wndProcP uintptr
)

func (d *driver) Name() string { return "win32" }

func (d *driver) Capabilities() core.Caps {
	return core.Caps{
		NativeControls: true,
		TreeView:       true,
		GridView:       true,
		FileDialog:     true,
		Menus:          true,
		Clipboard:      true,
		TrayIcon:       true, // Shell_NotifyIcon; wired up in a later phase
		MaxCallbacks:   2000,
	}
}

func (d *driver) Init() error {
	theDrv = d

	// Per-monitor DPI v2 must be set before the first window exists.
	if pSetProcessDpiCtx.Find() == nil {
		const perMonitorAwareV2 = ^uintptr(3) // (DPI_AWARENESS_CONTEXT)-4
		pSetProcessDpiCtx.Call(perMonitorAwareV2)
	} else if pSetProcessDPIAware.Find() == nil {
		pSetProcessDPIAware.Call()
	}
	enableVisualStyles()

	if pInitCommonControlsEx.Find() == nil {
		icc := initCommonControlsExT{
			dwSize: uint32(unsafe.Sizeof(initCommonControlsExT{})),
			dwICC:  0x0000FFFF,
		}
		pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
	}

	h, _, _ := pGetModuleHandleW.Call(0)
	d.hInstance = windows.Handle(h)

	cls, err := windows.UTF16PtrFromString("goWidgetsWindow")
	if err != nil {
		return err
	}
	d.className = cls

	if wndProcP == 0 {
		wndProcP = syscall.NewCallback(wndProc)
	}
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProcP,
		hInstance:     d.hInstance,
		hCursor:       windows.Handle(cursor),
		hbrBackground: windows.Handle(colorBtnFace + 1),
		lpszClassName: cls,
	}
	if r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassExW: %v", e)
	}
	d.font = uiFont()

	name, _ := windows.UTF16PtrFromString("goWidgetsMsg")
	mh, _, e := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)),
		0, 0, 0, 0, 0, hwndMessageOnly, 0, uintptr(d.hInstance), 0)
	if mh == 0 {
		return fmt.Errorf("message-only window: %v", e)
	}
	d.msgWnd = windows.Handle(mh)
	return nil
}

// enableVisualStyles turns Win95-grey controls into themed ones. The usual
// route is an embedded manifest (a .syso), which needs a resource compiler;
// building the activation context at run time keeps the build to `go build`.
func enableVisualStyles() {
	const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <dependency><dependentAssembly>
    <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls"
      version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df"
      language="*"/>
  </dependentAssembly></dependency>
</assembly>`
	path := filepath.Join(os.TempDir(), fmt.Sprintf("goWidgets-%d.manifest", os.Getpid()))
	if os.WriteFile(path, []byte(manifest), 0o600) != nil {
		return
	}
	src, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	ctx := actCtxW{cbSize: uint32(unsafe.Sizeof(actCtxW{})), lpSource: src}
	h, _, _ := pCreateActCtxW.Call(uintptr(unsafe.Pointer(&ctx)))
	if h == 0 || h == ^uintptr(0) {
		os.Remove(path)
		return
	}
	var cookie uintptr
	pActivateActCtx.Call(h, uintptr(unsafe.Pointer(&cookie)))
	// Never deactivated: it stays in force for the process lifetime.
}

func uiFont() windows.Handle {
	const spiGetNonClientMetrics = 0x0029
	ncm := nonClientMetricsW{cbSize: uint32(unsafe.Sizeof(nonClientMetricsW{}))}
	if r, _, _ := pSysParamsInfoW.Call(spiGetNonClientMetrics,
		uintptr(ncm.cbSize), uintptr(unsafe.Pointer(&ncm)), 0); r == 0 {
		return 0
	}
	f, _, _ := pCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&ncm.lfMessageFont)))
	return windows.Handle(f)
}

func (d *driver) RunMainLoop(ctx context.Context, pump func()) error {
	d.pumpMu.Lock()
	d.pump = pump
	d.pumpMu.Unlock()
	pump()

	go func() {
		<-ctx.Done()
		// PostQuitMessage only works on the loop's own thread, so ask that
		// thread to call it.
		pPostMessageW.Call(uintptr(d.msgWnd), wmEndLoop, 0, 0)
	}()

	var m msgW
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0 = WM_QUIT, -1 = error
			return nil
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (d *driver) Wake() {
	if d.msgWnd != 0 {
		pPostMessageW.Call(uintptr(d.msgWnd), wmWake, 0, 0)
	}
}

func (d *driver) Shutdown() {}

func (d *driver) CreateWindow(spec core.WindowSpec) (core.BackendWindow, error) {
	title, err := windows.UTF16PtrFromString(spec.Title)
	if err != nil {
		return nil, err
	}
	h, _, e := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(d.className)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow, cwUseDefault, cwUseDefault,
		uintptr(int32(spec.Size.W)), uintptr(int32(spec.Size.H)),
		0, 0, uintptr(d.hInstance), 0)
	if h == 0 {
		return nil, fmt.Errorf("CreateWindowExW: %v", e)
	}
	w := &window{
		drv:    d,
		hwnd:   h,
		nodes:  map[core.Handle]*node{},
		events: make(chan core.BackendEvent, 128),
		nextH:  1,
		nextID: 1000,
	}
	w.root = w.nextH
	w.nodes[w.root] = &node{hwnd: h}
	d.win = w
	return w, nil
}

type node struct {
	hwnd uintptr
	id   uint32
	kind core.WidgetKind
}

type window struct {
	drv    *driver
	hwnd   uintptr
	root   core.Handle
	nextH  core.Handle
	nextID uint32
	nodes  map[core.Handle]*node
	events chan core.BackendEvent
}

func (w *window) SetTitle(s string) {
	if p, err := windows.UTF16PtrFromString(s); err == nil {
		pSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(p)))
	}
}

func (w *window) Show()                            { pShowWindow.Call(w.hwnd, swShow) }
func (w *window) Close()                           { pShowWindow.Call(w.hwnd, swHide) }
func (w *window) RootHandle() core.Handle          { return w.root }
func (w *window) Events() <-chan core.BackendEvent { return w.events }

func (w *window) Scale() core.ScaleInfo {
	return core.ScaleInfo{Scale: w.scale(), FontScale: 1}
}

func (w *window) scale() float64 {
	if pGetDpiForWindow.Find() == nil {
		if dpi, _, _ := pGetDpiForWindow.Call(w.hwnd); dpi > 0 {
			return float64(dpi) / 96.0
		}
	}
	return 1.0
}

func (w *window) CreateWidget(kind core.WidgetKind, parent core.Handle) (core.Handle, error) {
	w.nextH++
	w.nextID++
	h, id := w.nextH, w.nextID

	class, style := "STATIC", uintptr(wsChild|wsVisible|ssLeftNoWordWrap)
	switch kind {
	case core.KindButton:
		class, style = "BUTTON", wsChild|wsVisible|wsTabStop
	case core.KindCheckBox:
		// BS_AUTOCHECKBOX makes the control own its state; we read it back on
		// BN_CLICKED rather than tracking it ourselves.
		class, style = "BUTTON", wsChild|wsVisible|wsTabStop|bsAutoCheckBox
	case core.KindTextView:
		// A read-only multiline EDIT with a vertical scrollbar is the native
		// log view on Windows — no extra control needed.
		class = "EDIT"
		style = wsChild | wsVisible | wsBorder | wsVScroll |
			esMultiline | esReadonly | esAutoVScroll
	}
	clsp, _ := windows.UTF16PtrFromString(class)
	txtp, _ := windows.UTF16PtrFromString("")

	hwnd, _, e := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(clsp)), uintptr(unsafe.Pointer(txtp)),
		style, 0, 0, 10, 10, w.hwnd, uintptr(id), uintptr(w.drv.hInstance), 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW(%s): %v", class, e)
	}
	if w.drv.font != 0 {
		pSendMessageW.Call(hwnd, wmSetFont, uintptr(w.drv.font), 1)
	}
	w.nodes[h] = &node{hwnd: hwnd, id: id, kind: kind}

	regMu.Lock()
	ctlByID[id] = h
	regMu.Unlock()
	return h, nil
}

func (w *window) DestroyWidget(h core.Handle) {
	if n := w.nodes[h]; n != nil {
		pDestroyWindow.Call(n.hwnd)
		regMu.Lock()
		delete(ctlByID, n.id)
		regMu.Unlock()
		delete(w.nodes, h)
	}
}

func (w *window) SetParent(child, parent core.Handle, index int) {}

func (w *window) SetString(h core.Handle, p core.PropKey, v string) {
	n := w.nodes[h]
	if n == nil || p != core.PropText {
		return
	}
	// A multiline EDIT wants CRLF line breaks; a bare LF shows as one long
	// line. Normalise here so callers can use plain "\n".
	if n.kind == core.KindTextView {
		v = crlf(v)
	}
	if s, err := windows.UTF16PtrFromString(v); err == nil {
		pSetWindowTextW.Call(n.hwnd, uintptr(unsafe.Pointer(s)))
	}
}

// crlf converts lone LF to CRLF without doubling existing CRLF.
func crlf(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b = append(b, '\r', '\n')
			continue
		}
		b = append(b, s[i])
	}
	return string(b)
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropEnabled:
		b := uintptr(0)
		if v {
			b = 1
		}
		pEnableWindow.Call(n.hwnd, b)
	case core.PropVisible:
		cmd := uintptr(swHide)
		if v {
			cmd = swShow
		}
		pShowWindow.Call(n.hwnd, cmd)
	case core.PropChecked:
		if n.kind == core.KindCheckBox {
			b := uintptr(0)
			if v {
				b = 1
			}
			pSendMessageW.Call(n.hwnd, bmSetCheck, b, 0)
		}
	}
}

func (w *window) SetFloat(core.Handle, core.PropKey, float64) {}

// MeasureIntrinsic asks the control itself where it can: themed buttons answer
// BCM_GETIDEALSIZE, which accounts for the current theme's padding. Static text
// falls back to the font metrics of the UI font. Physical pixels are converted
// back to DIP here, because §4.1 says only the backend knows the scale.
func (w *window) MeasureIntrinsic(h core.Handle, avail core.Size) (min, natural core.Size) {
	n := w.nodes[h]
	if n == nil {
		return core.Size{}, core.Size{}
	}
	s := w.scale()

	if n.kind == core.KindButton || n.kind == core.KindCheckBox {
		var sz sizeW
		if r, _, _ := pSendMessageW.Call(n.hwnd, bcmGetIdealSize, 0,
			uintptr(unsafe.Pointer(&sz))); r != 0 && sz.cy > 0 {
			nat := core.Size{W: float64(sz.cx) / s, H: float64(sz.cy) / s}
			return core.Size{W: 0, H: nat.H}, nat
		}
	}

	tw, th := w.textExtent(n.hwnd)
	switch n.kind {
	case core.KindButton:
		nat := core.Size{W: (tw + 32*s) / s, H: (th + 12*s) / s}
		return core.Size{W: 0, H: nat.H}, nat
	case core.KindCheckBox:
		// SM_CXMENUCHECK-sized indicator plus a gap, in physical pixels.
		nat := core.Size{W: (tw + 24*s) / s, H: (th + 6*s) / s}
		return core.Size{W: 0, H: nat.H}, nat
	default:
		nat := core.Size{W: tw / s, H: (th + 6*s) / s}
		return core.Size{W: 0, H: nat.H}, nat
	}
}

// textExtent measures a control's caption with the UI font, in physical pixels.
func (w *window) textExtent(hwnd uintptr) (width, height float64) {
	const wmGetTextLength, wmGetText = 0x000E, 0x000D
	n, _, _ := pSendMessageW.Call(hwnd, wmGetTextLength, 0, 0)
	buf := make([]uint16, n+1)
	pSendMessageW.Call(hwnd, wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))

	dc, _, _ := pGetDC.Call(hwnd)
	if dc == 0 {
		return 0, 16
	}
	defer pReleaseDC.Call(hwnd, dc)
	if w.drv.font != 0 {
		pSelectObject.Call(dc, uintptr(w.drv.font))
	}
	var sz sizeW
	pGetTextExtentPoint.Call(dc, uintptr(unsafe.Pointer(&buf[0])), uintptr(n),
		uintptr(unsafe.Pointer(&sz)))
	return float64(sz.cx), float64(sz.cy)
}

func (w *window) ApplyLayout(changes []core.BoundsChange) {
	s := w.scale()
	for _, c := range changes {
		n := w.nodes[c.H]
		if n == nil {
			continue
		}
		flags := uintptr(swpNoZOrder | swpNoActivate)
		if c.Visible {
			flags |= swpShowWindow
		} else {
			flags |= swpHideWindow
		}
		pSetWindowPos.Call(n.hwnd, 0,
			uintptr(int32(c.R.X*s)), uintptr(int32(c.R.Y*s)),
			uintptr(int32(c.R.W*s)), uintptr(int32(c.R.H*s)), flags)
	}
}

func (w *window) emit(ev core.BackendEvent) {
	select {
	case w.events <- ev:
	default: // never block the UI thread on a slow consumer
	}
}

// modifierState reads the modifier keys into the winkeys ControlKeyState bits.
func modifierState() uint32 {
	var state uint32
	held := func(vk int) bool {
		r, _, _ := pGetKeyState.Call(uintptr(vk))
		return int16(r) < 0
	}
	const (
		vkShift, vkControl, vkMenu, vkCapital = 0x10, 0x11, 0x12, 0x14
	)
	if held(vkShift) {
		state |= 0x0010 // ShiftPressed
	}
	if held(vkControl) {
		state |= 0x0008 // LeftCtrlPressed
	}
	if held(vkMenu) {
		state |= 0x0002 // LeftAltPressed
	}
	if r, _, _ := pGetKeyState.Call(vkCapital); r&1 != 0 {
		state |= 0x0080 // CapsLockOn
	}
	return state
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	d := theDrv
	if d == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	}

	switch msg {
	case wmWake:
		d.pumpMu.Lock()
		p := d.pump
		d.pumpMu.Unlock()
		if p != nil {
			p()
		}
		return 0

	case wmEndLoop:
		pPostQuitMessage.Call(0)
		return 0

	case wmTrayIcon:
		handleTrayMessage(lParam)
		return 0

	case wmCommand:
		if d.win != nil {
			id := uint32(wParam & 0xFFFF)
			regMu.Lock()
			h := ctlByID[id]
			regMu.Unlock()
			if n := d.win.nodes[h]; n != nil {
				if n.kind == core.KindCheckBox {
					// BS_AUTOCHECKBOX has already flipped itself by now.
					st, _, _ := pSendMessageW.Call(n.hwnd, bmGetCheck, 0, 0)
					d.win.emit(core.BackendEvent{
						Kind: core.EventToggled, H: h, Bool: st == 1,
					})
				} else {
					d.win.emit(core.BackendEvent{Kind: core.EventClicked, H: h})
				}
			}
		}
		return 0

	case wmSize:
		if d.win != nil && hwnd == d.win.hwnd {
			var r rectW
			pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
			s := d.win.scale()
			d.win.emit(core.BackendEvent{
				Kind: core.EventResized,
				Size: core.Size{
					W: float64(r.right-r.left) / s,
					H: float64(r.bottom-r.top) / s,
				},
			})
		}
		return 0

	case wmDpiChanged:
		if d.win != nil {
			d.win.emit(core.BackendEvent{Kind: core.EventScaleChanged, Scale: d.win.Scale()})
		}
		return 0

	case wmKeyDown, wmKeyUp, wmSysKeyDown, wmSysKeyUp:
		// Nothing to translate: wParam is already the virtual key code, which
		// is the vocabulary KeyEvent speaks. lParam carries the repeat count in
		// its low word and the scan code in bits 16-23.
		if d.win != nil && hwnd == d.win.hwnd {
			down := msg == wmKeyDown || msg == wmSysKeyDown
			d.win.emit(core.BackendEvent{
				Kind: core.EventKey,
				Key: core.KeyEvent{
					VirtualKeyCode:  uint16(wParam),
					VirtualScanCode: uint16((lParam >> 16) & 0xff),
					KeyDown:         down,
					ControlKeyState: modifierState(),
					RepeatCount:     uint16(lParam & 0xffff),
				},
			})
		}
		// Fall through to the default handler so menus and controls still work.

	case wmClose:
		if d.win != nil && hwnd == d.win.hwnd {
			// Core decides; it calls Close() if the user does not veto.
			d.win.emit(core.BackendEvent{Kind: core.EventCloseRequested})
			return 0
		}

	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
