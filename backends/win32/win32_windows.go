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
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/unxed/goWidgets/core"
	"golang.org/x/sys/windows"
)

func init() { core.RegisterDriver("win32", func() core.PlatformDriver { return &driver{} }) }

// WidgetHandle returns the HWND of the first widget of the given kind in the
// current window, or 0. A test hook (see backends/win32/win32test); an
// application has no use for it.
func WidgetHandle(kind core.WidgetKind) uintptr {
	d := theDrv
	if d == nil || d.win == nil {
		return 0
	}
	for h := core.Handle(1); h <= d.win.nextH; h++ {
		if n := d.win.nodes[h]; n != nil && n.kind == kind {
			return n.hwnd
		}
	}
	return 0
}

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pGetMessageW        = user32.NewProc("GetMessageW")
	pIsDialogMessageW   = user32.NewProc("IsDialogMessageW")
	pSetFocus           = user32.NewProc("SetFocus")
	pGetFocus           = user32.NewProc("GetFocus")
	pGetNextDlgTabItem  = user32.NewProc("GetNextDlgTabItem")
	pIsWindowVisible    = user32.NewProc("IsWindowVisible")
	pMessageBoxW        = user32.NewProc("MessageBoxW")
	pGetOpenFileNameW   = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileNameW   = comdlg32.NewProc("GetSaveFileNameW")
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
	vkReturn     = 0x0D
	wmSetFocus   = 0x0007
	wmActivate   = 0x0006
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
	esAutoHScroll   = 0x00000080
	cbsDropdown     = 0x0002
	cbsDropdownList = 0x0003
	cbsAutoHScroll  = 0x0040
	cbAddString     = 0x0143
	cbGetCurSel     = 0x0147
	cbGetLBText     = 0x0148
	cbGetLBTextLen  = 0x0149
	cbResetContent  = 0x014B
	cbSetCurSel     = 0x014E
	cbGetItemHeight = 0x0154
	cbnSelChange    = 1
	cbnEditChange   = 5
	comboListRows   = 8 // rows shown when the list drops down
	lbsNotify       = 0x0001
	lbsNoIntegral   = 0x0100
	lbAddString     = 0x0180
	lbResetContent  = 0x0184
	lbSetCurSel     = 0x0186
	lbGetCurSel     = 0x0188
	lbGetText       = 0x0189
	lbGetTextLen    = 0x018A
	lbGetCount      = 0x018B
	lbGetItemHeight = 0x01A1
	lbnSelChange    = 1
	lbnDblClk       = 2
	listRows        = 8 // natural height of a list box, in rows
	wsVScroll       = 0x00200000
	wsHScroll       = 0x00100000
	wsBorder        = 0x00800000
	bmGetCheck      = 0x00F0
	bmSetCheck      = 0x00F1

	swHide        = 0
	swShow        = 5
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
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
	monoFont  windows.Handle
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
	d.monoFont = monoFont()

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

// monoFont returns the system fixed-pitch UI font, sized like the message font
// so a log pane does not jump out from the rest of the window.
func monoFont() windows.Handle {
	const spiGetNonClientMetrics = 0x0029
	ncm := nonClientMetricsW{cbSize: uint32(unsafe.Sizeof(nonClientMetricsW{}))}
	if r, _, _ := pSysParamsInfoW.Call(spiGetNonClientMetrics,
		uintptr(ncm.cbSize), uintptr(unsafe.Pointer(&ncm)), 0); r == 0 {
		return 0
	}
	lf := ncm.lfMessageFont
	const fixedPitchFontFamily = 0x01 | 0x30 // FIXED_PITCH | FF_MODERN
	lf.lfPitchAndFamily = fixedPitchFontFamily
	// Consolas ships with every supported Windows; an empty face name would
	// let GDI pick whatever it liked.
	name, err := windows.UTF16FromString("Consolas")
	if err == nil {
		for i := range lf.lfFaceName {
			lf.lfFaceName[i] = 0
		}
		copy(lf.lfFaceName[:], name)
	}
	f, _, _ := pCreateFontIndirectW.Call(uintptr(unsafe.Pointer(&lf)))
	return windows.Handle(f)
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
		// Keys are reported for the whole window, whichever control has
		// focus: with focus living in controls (as it does once there is a
		// text field), a WM_KEYDOWN reaches the control's window, never the
		// toplevel, so a window-level shortcut has to be caught here. Same
		// vantage point as GTK's key-press-event on the toplevel.
		switch m.message {
		case wmKeyDown, wmKeyUp, wmSysKeyDown, wmSysKeyUp:
			if d.win != nil && d.win.isOurs(uintptr(m.hwnd)) {
				down := m.message == wmKeyDown || m.message == wmSysKeyDown
				d.win.emit(core.BackendEvent{
					Kind: core.EventKey,
					Key: core.KeyEvent{
						VirtualKeyCode:  uint16(m.wParam),
						VirtualScanCode: uint16((m.lParam >> 16) & 0xff),
						KeyDown:         down,
						ControlKeyState: modifierState(),
						RepeatCount:     uint16(m.lParam & 0xffff),
					},
				})
			}
		}
		// Enter in a text field is the field's activation. A single-line
		// EDIT does nothing with the key itself (in a dialog the dialog
		// manager would turn it into the default button), so it is taken
		// here, before translation, and not passed on — passing it on would
		// only produce the "unhandled character" beep.
		if m.message == wmKeyDown && m.wParam == vkReturn && d.win != nil {
			if h, n := d.win.nodeByHwnd(uintptr(m.hwnd)); n != nil {
				switch n.kind {
				case core.KindEdit:
					d.win.emit(core.BackendEvent{
						Kind: core.EventActivated, H: h, Text: d.win.text(n.hwnd),
					})
					continue
				case core.KindListBox:
					// Enter on a list opens the selected item, as a
					// double-click does; a LISTBOX has no notification for it.
					if i, _, _ := pSendMessageW.Call(n.hwnd, lbGetCurSel, 0, 0); int32(i) >= 0 {
						d.win.emit(core.BackendEvent{
							Kind: core.EventItemActivated, H: h, Int: int(int32(i)), Text: d.win.listItemText(n.hwnd, int(int32(i))),
						})
					}
					continue
				}
			}
		}
		// Tab, Shift+Tab and mnemonics between controls are the dialog
		// manager's job; IsDialogMessage does it for any window with
		// controls. It also turns Enter into a WM_COMMAND with IDOK (1) and
		// Esc into IDCANCEL (2), which no control id matches, so those fall
		// through wndProc harmlessly.
		if d.win != nil {
			if r, _, _ := pIsDialogMessageW.Call(d.win.hwnd, uintptr(unsafe.Pointer(&m))); r != 0 {
				continue
			}
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
	hwnd    uintptr
	id      uint32
	kind    core.WidgetKind
	rect    core.Rect // last applied, in DIP; for tab order
	visible bool
}

// isOurs reports whether hwnd is the window or one of its controls.
func (w *window) isOurs(hwnd uintptr) bool {
	if hwnd == w.hwnd {
		return true
	}
	_, n := w.nodeByHwnd(hwnd)
	return n != nil
}

type window struct {
	drv    *driver
	hwnd   uintptr
	root   core.Handle
	nextH  core.Handle
	nextID uint32
	nodes  map[core.Handle]*node
	events chan core.BackendEvent

	// focus is the control that should own keyboard focus: what Focus()
	// asked for, or the control that had it when the window was last
	// deactivated. A plain window does not restore focus to a child by
	// itself; a dialog manager does, and this is that part of one.
	focus uintptr

	// tabOrder is the Z-order last established by sortTabOrder.
	tabOrder []uintptr
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
	case core.KindComboBox:
		// CBS_DROPDOWN has a text field; SetBool swaps in CBS_DROPDOWNLIST for
		// dropdown-only. The height a COMBOBOX is given is the height of its
		// opened list; the closed control is one line regardless (ApplyLayout
		// adds the list height back).
		class, style = "COMBOBOX", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropdown|cbsAutoHScroll
	case core.KindListBox:
		// LBS_NOTIFY for selection notifications; LBS_NOINTEGRALHEIGHT so the
		// control takes the height the layout gives it rather than rounding
		// to whole rows.
		class, style = "LISTBOX", wsChild|wsVisible|wsTabStop|wsBorder|wsVScroll|lbsNotify|lbsNoIntegral
	case core.KindEdit:
		// A single-line EDIT. ES_AUTOHSCROLL lets text longer than the field
		// scroll instead of stopping; WS_BORDER draws the box.
		class, style = "EDIT", wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll
	case core.KindTextView:
		// A read-only multiline EDIT with a vertical scrollbar is the native
		// log view on Windows — no extra control needed.
		class = "EDIT"
		// Both scrollbars, and no wrapping: a multiline EDIT wraps unless it
		// is given a horizontal scrollbar, and a wrapped log loses the column
		// structure that makes it readable at all.
		style = wsChild | wsVisible | wsBorder | wsVScroll | wsHScroll |
			esMultiline | esReadonly | esAutoVScroll | esAutoHScroll
	}
	hwnd, err := w.createControl(class, style, id)
	if err != nil {
		return 0, err
	}
	// A text view shows a log: columns of time, source and message. In a
	// proportional font the columns do not line up and it reads as a mess.
	f := w.drv.font
	if kind == core.KindTextView && w.drv.monoFont != 0 {
		f = w.drv.monoFont
	}
	if f != 0 {
		pSendMessageW.Call(hwnd, wmSetFont, uintptr(f), 1)
	}
	w.nodes[h] = &node{hwnd: hwnd, id: id, kind: kind}

	regMu.Lock()
	ctlByID[id] = h
	regMu.Unlock()
	return h, nil
}

// createControl makes a child control of the window with the given class,
// style and command id.
func (w *window) createControl(class string, style uintptr, id uint32) (uintptr, error) {
	clsp, _ := windows.UTF16PtrFromString(class)
	txtp, _ := windows.UTF16PtrFromString("")
	hwnd, _, e := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(clsp)), uintptr(unsafe.Pointer(txtp)),
		style, 0, 0, 10, 10, w.hwnd, uintptr(id), uintptr(w.drv.hInstance), 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW(%s): %v", class, e)
	}
	return hwnd, nil
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

// text reads a control's current text.
func (w *window) text(hwnd uintptr) string {
	const wmGetTextLength, wmGetText = 0x000E, 0x000D
	n, _, _ := pSendMessageW.Call(hwnd, wmGetTextLength, 0, 0)
	buf := make([]uint16, n+1)
	pSendMessageW.Call(hwnd, wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return windows.UTF16ToString(buf)
}

// nodeByHwnd finds the node owning a control window.
func (w *window) nodeByHwnd(hwnd uintptr) (core.Handle, *node) {
	for h, n := range w.nodes {
		if n.hwnd == hwnd {
			return h, n
		}
	}
	return 0, nil
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

// Dialog is MessageBoxW: modal over the window, with the system's own
// localized buttons. MessageBox runs its own message loop, so the
// application's wake messages keep being dispatched to the message window
// while the box is up.
func (w *window) Dialog(kind core.DialogKind, title, text string) core.DialogResult {
	const (
		mbOK        = 0x0
		mbOKCancel  = 0x1
		mbYesNo     = 0x4
		mbIconInfo  = 0x40
		mbIconQuest = 0x20
		idOK, idYes = 1, 6
	)
	var style uintptr
	switch kind {
	case core.DialogConfirm:
		style = mbOKCancel | mbIconQuest
	case core.DialogYesNo:
		style = mbYesNo | mbIconQuest
	default:
		style = mbOK | mbIconInfo
	}
	tp, _ := windows.UTF16PtrFromString(text)
	cp, _ := windows.UTF16PtrFromString(title)
	r, _, _ := pMessageBoxW.Call(w.hwnd, uintptr(unsafe.Pointer(tp)), uintptr(unsafe.Pointer(cp)), style)
	switch kind {
	case core.DialogConfirm:
		if r == idOK {
			return core.DialogOK
		}
		return core.DialogCancel
	case core.DialogYesNo:
		if r == idYes {
			return core.DialogYes
		}
		return core.DialogNo
	}
	return core.DialogOK
}

// openFileNameW mirrors OPENFILENAMEW; the field order and Go's natural
// alignment reproduce the C layout (152 bytes on 64-bit).
type openFileNameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// FileDialog is GetOpenFileNameW / GetSaveFileNameW — the common dialog
// with the system's own look and buttons. Like MessageBox it runs its own
// loop, so the application keeps ticking underneath.
func (w *window) FileDialog(save bool, title, suggested string, filters []core.FileFilter) (string, bool) {
	const (
		ofnOverwritePrompt = 0x2
		ofnNoChangeDir     = 0x8
		ofnPathMustExist   = 0x800
		ofnFileMustExist   = 0x1000
		ofnExplorer        = 0x80000
	)
	// The filter is "name\0patterns\0…\0\0", patterns joined by ';'.
	var filt []uint16
	for _, f := range filters {
		filt = append(filt, utf16z(f.Name)...)
		filt = append(filt, utf16z(strings.Join(f.Patterns, ";"))...)
	}
	filt = append(filt, 0)

	file := make([]uint16, 32768)
	copy(file, utf16z(suggested))
	tp, _ := windows.UTF16PtrFromString(title)

	ofn := openFileNameW{
		hwndOwner:    w.hwnd,
		lpstrFile:    &file[0],
		nMaxFile:     uint32(len(file)),
		lpstrTitle:   tp,
		nFilterIndex: 1,
		flags:        ofnExplorer | ofnNoChangeDir | ofnPathMustExist,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	if len(filters) > 0 {
		ofn.lpstrFilter = &filt[0]
	}
	proc := pGetOpenFileNameW
	if save {
		proc = pGetSaveFileNameW
		ofn.flags |= ofnOverwritePrompt
	} else {
		ofn.flags |= ofnFileMustExist
	}
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(file), true
}

// utf16z encodes s as UTF-16 with a terminating NUL.
func utf16z(s string) []uint16 {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return u
}

// Focus moves keyboard focus to a control. Before the window is visible
// SetFocus would have nothing to attach to, so the request is kept and
// applied when the window first takes focus (WM_SETFOCUS).
func (w *window) Focus(h core.Handle) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	w.focus = n.hwnd
	if vis, _, _ := pIsWindowVisible.Call(w.hwnd); vis != 0 {
		pSetFocus.Call(n.hwnd)
	}
}

// focusChild hands focus to the remembered control, or to the first
// tab stop when nothing was asked for.
func (w *window) focusChild() {
	target := w.focus
	if target == 0 {
		target, _, _ = pGetNextDlgTabItem.Call(w.hwnd, 0, 0)
	}
	if target != 0 && target != w.hwnd {
		pSetFocus.Call(target)
	}
}

func (w *window) SetBool(h core.Handle, p core.PropKey, v bool) {
	n := w.nodes[h]
	if n == nil {
		return
	}
	switch p {
	case core.PropDropdownOnly:
		// The list-only style is a creation-time choice: rebuild the control
		// (this precedes items and layout).
		if n.kind == core.KindComboBox && v {
			hwnd, err := w.createControl("COMBOBOX", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropdownList, n.id)
			if err != nil {
				return
			}
			pDestroyWindow.Call(n.hwnd)
			n.hwnd = hwnd
			if w.drv.font != 0 {
				pSendMessageW.Call(hwnd, wmSetFont, uintptr(w.drv.font), 1)
			}
		}
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

func (w *window) SetInt(h core.Handle, p core.PropKey, v int) {
	n := w.nodes[h]
	if n == nil || p != core.PropSelected {
		return
	}
	switch n.kind {
	case core.KindComboBox:
		pSendMessageW.Call(n.hwnd, cbSetCurSel, uintptr(v), 0)
	case core.KindListBox:
		pSendMessageW.Call(n.hwnd, lbSetCurSel, uintptr(v), 0)
	}
}

func (w *window) SetList(h core.Handle, p core.PropKey, items []string) {
	n := w.nodes[h]
	if n == nil || p != core.PropItems {
		return
	}
	reset, add := uintptr(cbResetContent), uintptr(cbAddString)
	switch n.kind {
	case core.KindListBox:
		reset, add = lbResetContent, lbAddString
	case core.KindComboBox:
	default:
		return
	}
	pSendMessageW.Call(n.hwnd, reset, 0, 0)
	for _, it := range items {
		if s, err := windows.UTF16PtrFromString(it); err == nil {
			pSendMessageW.Call(n.hwnd, add, 0, uintptr(unsafe.Pointer(s)))
		}
	}
}

// comboItemText reads item i of a combo box's list.
func (w *window) comboItemText(hwnd uintptr, i int) string {
	return itemText(hwnd, cbGetLBTextLen, cbGetLBText, i)
}

// listItemText reads item i of a list box.
func (w *window) listItemText(hwnd uintptr, i int) string {
	return itemText(hwnd, lbGetTextLen, lbGetText, i)
}

func itemText(hwnd uintptr, lenMsg, textMsg uintptr, i int) string {
	n, _, _ := pSendMessageW.Call(hwnd, lenMsg, uintptr(i), 0)
	if int32(n) < 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pSendMessageW.Call(hwnd, textMsg, uintptr(i), uintptr(unsafe.Pointer(&buf[0])))
	return windows.UTF16ToString(buf)
}

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
	case core.KindEdit:
		// Width is a choice, not a function of the contents; the height is
		// one line plus the border and padding of a themed EDIT.
		h := (th + 8*s) / s
		return core.Size{W: 40, H: h}, core.Size{W: 160, H: h}
	case core.KindListBox:
		ih, _, _ := pSendMessageW.Call(n.hwnd, lbGetItemHeight, 0, 0)
		widest := 0.0
		cnt, _, _ := pSendMessageW.Call(n.hwnd, lbGetCount, 0, 0)
		for i := 0; i < int(cnt); i++ {
			if tw, _ := w.textExtentOf(n.hwnd, w.listItemText(n.hwnd, i)); tw > widest {
				widest = tw
			}
		}
		row := float64(ih) / s
		return core.Size{W: 40, H: 3 * row}, core.Size{W: (widest + 40*s) / s, H: listRows * row}
	case core.KindComboBox:
		// The control reports its own closed height; width fits the widest
		// item plus the arrow button.
		ih, _, _ := pSendMessageW.Call(n.hwnd, cbGetItemHeight, ^uintptr(0), 0)
		h := (float64(ih) + 6*s) / s
		widest := 0.0
		cnt, _, _ := pSendMessageW.Call(n.hwnd, 0x0146 /* CB_GETCOUNT */, 0, 0)
		for i := 0; i < int(cnt); i++ {
			if tw, _ := w.textExtentOf(n.hwnd, w.comboItemText(n.hwnd, i)); tw > widest {
				widest = tw
			}
		}
		return core.Size{W: 40, H: h}, core.Size{W: (widest + 40*s) / s, H: h}
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
	return w.textExtentOf(hwnd, w.text(hwnd))
}

// textExtentOf measures s with the UI font on a control's DC, in physical pixels.
func (w *window) textExtentOf(hwnd uintptr, s string) (width, height float64) {
	buf, _ := windows.UTF16FromString(s)
	if len(buf) == 0 {
		buf = []uint16{0}
	}

	dc, _, _ := pGetDC.Call(hwnd)
	if dc == 0 {
		return 0, 16
	}
	defer pReleaseDC.Call(hwnd, dc)
	if w.drv.font != 0 {
		pSelectObject.Call(dc, uintptr(w.drv.font))
	}
	var sz sizeW
	pGetTextExtentPoint.Call(dc, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)-1),
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
		h := c.R.H
		if n.kind == core.KindComboBox {
			// The given height is the opened list's; the closed control
			// stays one line, so the layout's height is the closed one and
			// room for comboListRows items is added underneath.
			ih, _, _ := pSendMessageW.Call(n.hwnd, cbGetItemHeight, 0, 0)
			h += float64(ih) / s * comboListRows
		}
		pSetWindowPos.Call(n.hwnd, 0,
			uintptr(int32(c.R.X*s)), uintptr(int32(c.R.Y*s)),
			uintptr(int32(c.R.W*s)), uintptr(int32(h*s)), flags)
		n.rect, n.visible = c.R, c.Visible
	}
	if len(changes) > 0 {
		w.sortTabOrder()
	}
}

// sortTabOrder makes Tab follow the layout. On Win32 the tab order is the
// Z-order, which is creation order unless something changes it; GTK's
// GtkFixed sorts its focus chain by position instead. Creation order is an
// accident of the program's structure (a list that grows at run time ends up
// after the buttons created before it), so the Z-order is re-sorted after
// every layout: by top edge, then left — the same rule GTK applies.
func (w *window) sortTabOrder() {
	var ns []*node
	for _, n := range w.nodes {
		if n.visible {
			ns = append(ns, n)
		}
	}
	sort.Slice(ns, func(i, j int) bool {
		if ns[i].rect.Y != ns[j].rect.Y {
			return ns[i].rect.Y < ns[j].rect.Y
		}
		return ns[i].rect.X < ns[j].rect.X
	})
	same := len(ns) == len(w.tabOrder)
	for i := 0; same && i < len(ns); i++ {
		same = ns[i].hwnd == w.tabOrder[i]
	}
	if same {
		return
	}
	w.tabOrder = w.tabOrder[:0]
	var prev uintptr // 0 = HWND_TOP
	for _, n := range ns {
		pSetWindowPos.Call(n.hwnd, prev, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		prev = n.hwnd
		w.tabOrder = append(w.tabOrder, n.hwnd)
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
				if n.kind == core.KindListBox {
					code := uint32(wParam >> 16)
					if code == lbnSelChange || code == lbnDblClk {
						i, _, _ := pSendMessageW.Call(n.hwnd, lbGetCurSel, 0, 0)
						kind := core.EventSelected
						if code == lbnDblClk {
							kind = core.EventItemActivated
						}
						d.win.emit(core.BackendEvent{
							Kind: kind, H: h, Int: int(int32(i)), Text: d.win.listItemText(n.hwnd, int(int32(i))),
						})
					}
				} else if n.kind == core.KindComboBox {
					switch uint32(wParam >> 16) {
					case cbnSelChange:
						i, _, _ := pSendMessageW.Call(n.hwnd, cbGetCurSel, 0, 0)
						if int32(i) >= 0 {
							d.win.emit(core.BackendEvent{
								Kind: core.EventSelected, H: h, Int: int(int32(i)), Text: d.win.comboItemText(n.hwnd, int(int32(i))),
							})
						}
					case cbnEditChange:
						d.win.emit(core.BackendEvent{
							Kind: core.EventTextChanged, H: h, Text: d.win.text(n.hwnd),
						})
					}
				} else if n.kind == core.KindEdit {
					// EN_CHANGE arrives after the control has updated; the
					// other EDIT notifications (focus, update, scroll) are
					// not edits.
					const enChange = 0x0300
					if uint32(wParam>>16) == enChange {
						d.win.emit(core.BackendEvent{
							Kind: core.EventTextChanged, H: h, Text: d.win.text(n.hwnd),
						})
					}
				} else if n.kind == core.KindCheckBox {
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

	case wmSetFocus:
		// The toplevel took focus (first show, or back from another
		// window): pass it on to a control, or Tab has nothing to move.
		if d.win != nil && hwnd == d.win.hwnd {
			d.win.focusChild()
			return 0
		}

	case wmActivate:
		// Deactivating: remember which control had focus so it gets it back.
		if d.win != nil && hwnd == d.win.hwnd && uint16(wParam) == 0 {
			if f, _, _ := pGetFocus.Call(); f != 0 && f != d.win.hwnd && d.win.isOurs(f) {
				d.win.focus = f
			}
		}

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
