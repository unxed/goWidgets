//go:build windows

package win32

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/unxed/goWidgets/core"
	"golang.org/x/sys/windows"
)

// The Windows status area is Shell_NotifyIcon, and it has been that since 1996.
// Unlike the Linux side there is no ambiguity here: one API, no deprecation, no
// desktop-environment lottery.
//
// The icon needs a window to deliver its notifications to, and the driver
// already owns a message-only window for QueueUpdate — so the tray reuses it
// rather than creating another.

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	pShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	pCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	pDestroyMenu      = user32.NewProc("DestroyMenu")
	pAppendMenuW      = user32.NewProc("AppendMenuW")
	pTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	pGetCursorPos     = user32.NewProc("GetCursorPos")
	pSetForegroundWnd = user32.NewProc("SetForegroundWindow")
	pLoadIconW        = user32.NewProc("LoadIconW")
)

const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	wmTrayIcon = wmApp + 3

	wmLButtonUp = 0x0202
	wmRButtonUp = 0x0205

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	tpmNonNotify   = 0x0080
	idiApplication = 32512
	trayIconID     = 1
	menuIDBase     = 40000
)

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             windows.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     windows.Handle
}

type pointW struct{ x, y int32 }

type tray struct {
	drv    *driver
	data   notifyIconDataW
	events chan core.BackendEvent

	mu    sync.Mutex
	menu  uintptr
	items map[uint32]core.Handle // menu command id → item id
}

var trayCurrent *tray

func (d *driver) CreateTray(spec core.TraySpec) (core.BackendTray, error) {
	if d.msgWnd == 0 {
		return nil, fmt.Errorf("нет окна для доставки уведомлений трея")
	}
	icon, _, _ := pLoadIconW.Call(0, idiApplication)

	t := &tray{
		drv:    d,
		events: make(chan core.BackendEvent, 32),
		items:  map[uint32]core.Handle{},
	}
	t.data = notifyIconDataW{
		cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:             d.msgWnd,
		uID:              trayIconID,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayIcon,
		hIcon:            windows.Handle(icon),
	}
	setTip(&t.data, spec.Tooltip)

	if r, _, e := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&t.data))); r == 0 {
		return nil, fmt.Errorf("Shell_NotifyIcon(NIM_ADD): %v", e)
	}
	trayCurrent = t
	t.SetMenu(spec.Menu)
	return t, nil
}

// setTip copies a tooltip into the fixed-size field, truncating rather than
// overflowing: szTip holds 127 characters plus a terminator.
func setTip(d *notifyIconDataW, s string) {
	for i := range d.szTip {
		d.szTip[i] = 0
	}
	u := windows.StringToUTF16(s)
	if len(u) > len(d.szTip) {
		u = u[:len(d.szTip)-1]
		u = append(u, 0)
	}
	copy(d.szTip[:], u)
}

func (t *tray) SetTooltip(s string) {
	setTip(&t.data, s)
	t.data.uFlags = nifMessage | nifIcon | nifTip
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&t.data)))
}

func (t *tray) SetMenu(items []core.MenuItem) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	t.mu.Lock()
	t.items = map[uint32]core.Handle{}
	for i, it := range items {
		if it.Separator {
			pAppendMenuW.Call(menu, mfSeparator, 0, 0)
			continue
		}
		cmd := uint32(menuIDBase + i)
		t.items[cmd] = it.ID
		label, err := windows.UTF16PtrFromString(it.Label)
		if err != nil {
			continue
		}
		pAppendMenuW.Call(menu, mfString, uintptr(cmd), uintptr(unsafe.Pointer(label)))
	}
	old := t.menu
	t.menu = menu
	t.mu.Unlock()

	if old != 0 {
		pDestroyMenu.Call(old)
	}
}

func (t *tray) Destroy() {
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.data)))
	t.mu.Lock()
	if t.menu != 0 {
		pDestroyMenu.Call(t.menu)
		t.menu = 0
	}
	t.mu.Unlock()
	if trayCurrent == t {
		trayCurrent = nil
	}
}

func (t *tray) Events() <-chan core.BackendEvent { return t.events }

// Embedded: on Windows the notification area is part of the shell, so a
// successful NIM_ADD means the icon is in it. There is no separate docking
// step to wait for, unlike X11.
func (t *tray) Embedded() bool { return t.data.hWnd != 0 }

func (t *tray) emit(ev core.BackendEvent) {
	select {
	case t.events <- ev:
	default:
	}
}

// popup shows the menu at the cursor.
//
// SetForegroundWindow before and a null message after are the documented
// workaround for a menu that otherwise refuses to close when the user clicks
// elsewhere — a Win32 wart older than most of its users.
func (t *tray) popup() {
	t.mu.Lock()
	menu := t.menu
	t.mu.Unlock()
	if menu == 0 {
		return
	}
	var pt pointW
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	pSetForegroundWnd.Call(uintptr(t.data.hWnd))
	cmd, _, _ := pTrackPopupMenu.Call(menu,
		tpmRightButton|tpmReturnCmd|tpmNonNotify,
		uintptr(pt.x), uintptr(pt.y), 0, uintptr(t.data.hWnd), 0)
	pPostMessageW.Call(uintptr(t.data.hWnd), 0, 0, 0)

	if cmd == 0 {
		return // dismissed
	}
	t.mu.Lock()
	id, ok := t.items[uint32(cmd)]
	t.mu.Unlock()
	if ok {
		t.emit(core.BackendEvent{Kind: core.EventMenuItem, H: id})
	}
}

// handleTrayMessage is called from the window procedure.
func handleTrayMessage(lParam uintptr) {
	t := trayCurrent
	if t == nil {
		return
	}
	switch uint32(lParam) {
	case wmLButtonUp:
		t.emit(core.BackendEvent{Kind: core.EventTrayActivated})
	case wmRButtonUp:
		t.popup()
	}
}
