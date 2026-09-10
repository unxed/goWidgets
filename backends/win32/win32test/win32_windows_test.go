//go:build windows

// Package win32test exercises the Win32 driver against a real window: focus
// before Show, typing order, Enter, Tab. It runs on the Windows CI runner and
// under Wine (GOOS=windows go test -c, then wine).
package win32test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/win32"
	"github.com/unxed/goWidgets/core"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	pGetFocus    = user32.NewProc("GetFocus")
	pPostMessage = user32.NewProc("PostMessageW")
	pFindWindow  = user32.NewProc("FindWindowW")
	pGetDlgItem  = user32.NewProc("GetDlgItem")
	pSetDlgText  = user32.NewProc("SetDlgItemTextW")
	pSendMessage = user32.NewProc("SendMessageW")
	pGetCtrlID   = user32.NewProc("GetDlgCtrlID")
	pGetParent   = user32.NewProc("GetParent")
)

const (
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmChar       = 0x0102
	wmCommand    = 0x0111
	idYes        = 6
	idOK         = 1
	cbSetCurSel  = 0x014E
	cbnSelChange = 1
	lbSetCurSel  = 0x0186
	lbnSelChange = 1
	lbnDblClk    = 2
	vkReturn     = 0x0D
	vkTab        = 0x09
)

func TestEditFocusAndKeys(t *testing.T) {
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "win32" {
		t.Skipf("драйвер %s, а проверяется win32", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("win32test", 300, 120)
	if err != nil {
		t.Fatal(err)
	}
	btn, _ := win.AddButton("Гнать")
	e, _ := win.AddEdit("")
	combo, _ := win.AddComboBox([]string{"luna", "sol", "terra"}, true)
	combo.Select(0)
	list, _ := win.AddListBox([]string{"один", "два", "три"})
	list.Select(0)
	if err := win.Constrain(
		e.Left().Eq(win.Left().Plus(8)), e.Top().Eq(win.Top().Plus(8)),
		btn.Left().Eq(e.Right().Plus(8)), btn.Top().Eq(e.Top()),
		combo.Left().Eq(e.Left()), combo.Top().Eq(e.Bottom().Plus(8)),
		list.Left().Eq(e.Left()), list.Top().Eq(combo.Bottom().Plus(8)), list.Height().Is(60),
	); err != nil {
		t.Fatal(err)
	}
	e.Focus() // before Show: must still land

	var changed, activated []string
	e.Changed.On(app.Scope(), func(v string) { changed = append(changed, v) })
	e.Activated.On(app.Scope(), func(v string) { activated = append(activated, v) })

	focus := func() uintptr { f, _, _ := pGetFocus.Call(); return f }
	var focusAtStart, focusAfterTab, entryHwnd, btnHwnd uintptr
	var asked, boxFound, openBoxFound, editFound, openedOK bool
	var picked, listPicked, listOpened []int
	combo.Selected.On(app.Scope(), func(i int) { picked = append(picked, i) })
	list.Selected.On(app.Scope(), func(i int) { listPicked = append(listPicked, i) })
	list.Activated.On(app.Scope(), func(i int) { listOpened = append(listOpened, i) })
	var openedPath string
	tmpFile := filepath.Join(os.TempDir(), "win32test-open.txt")
	if err := os.WriteFile(tmpFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)
	step := func(delay time.Duration, f func()) {
		time.Sleep(delay)
		done := make(chan struct{})
		app.QueueUpdate(func() { f(); close(done) })
		<-done
	}
	go func() {
		step(500*time.Millisecond, func() {
			entryHwnd, btnHwnd = win32.WidgetHandle(core.KindEdit), win32.WidgetHandle(core.KindButton)
			focusAtStart = focus()
			// Typing: WM_CHAR is what TranslateMessage produces per key.
			for _, ch := range "abc" {
				pPostMessage.Call(entryHwnd, wmChar, uintptr(ch), 1)
			}
		})
		step(300*time.Millisecond, func() {
			pPostMessage.Call(entryHwnd, wmKeyDown, vkReturn, 1)
			pPostMessage.Call(entryHwnd, wmKeyUp, vkReturn, 1)
		})
		step(300*time.Millisecond, func() {
			pPostMessage.Call(entryHwnd, wmKeyDown, vkTab, 1)
			pPostMessage.Call(entryHwnd, wmKeyUp, vkTab, 1)
		})
		step(300*time.Millisecond, func() { focusAfterTab = focus() })

		// A pick in the combo box, as the control reports one: the selection
		// set, then CBN_SELCHANGE to the parent.
		step(100*time.Millisecond, func() {
			ch := win32.WidgetHandle(core.KindComboBox)
			pSendMessage.Call(ch, cbSetCurSel, 2, 0)
			id, _, _ := pGetCtrlID.Call(ch)
			parent, _, _ := pGetParent.Call(ch)
			pPostMessage.Call(parent, wmCommand, uintptr(cbnSelChange)<<16|id, ch)
		})
		// The same for the list box: a selection, then a double-click.
		step(100*time.Millisecond, func() {
			lh := win32.WidgetHandle(core.KindListBox)
			pSendMessage.Call(lh, lbSetCurSel, 2, 0)
			id, _, _ := pGetCtrlID.Call(lh)
			parent, _, _ := pGetParent.Call(lh)
			pPostMessage.Call(parent, wmCommand, uintptr(lbnSelChange)<<16|id, lh)
			pPostMessage.Call(parent, wmCommand, uintptr(lbnDblClk)<<16|id, lh)
		})
		step(300*time.Millisecond, func() {})

		// A modal box blocks the UI goroutine in MessageBox's own loop; the
		// box is found by its title and answered with the Yes command, and
		// the answer must come back to the caller.
		askDone := make(chan struct{})
		app.QueueUpdate(func() {
			asked = win.Ask("win32test-ask", "Убрать отмеченные?")
			close(askDone)
		})
		title, _ := syscall.UTF16PtrFromString("win32test-ask")
		deadline := time.Now().Add(5 * time.Second)
		var box uintptr
		for box == 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			box, _, _ = pFindWindow.Call(0, uintptr(unsafe.Pointer(title)))
		}
		boxFound = box != 0
		if boxFound {
			pPostMessage.Call(box, wmCommand, idYes, 0)
		}
		select {
		case <-askDone:
		case <-time.After(5 * time.Second):
		}

		// The open-file common dialog: found by title, the file-name field
		// (edt1 = 1152 in Explorer-style dialogs, cmb13 = 1148 in older
		// layouts) filled with a real file, OK posted.
		openDone := make(chan struct{})
		app.QueueUpdate(func() {
			openedPath, openedOK = win.OpenFile("win32test-open", goWidgets.FileFilter{Name: "Текст", Patterns: []string{"*.txt"}})
			close(openDone)
		})
		otitle, _ := syscall.UTF16PtrFromString("win32test-open")
		deadline = time.Now().Add(8 * time.Second)
		box = 0
		for box == 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			box, _, _ = pFindWindow.Call(0, uintptr(unsafe.Pointer(otitle)))
		}
		openBoxFound = box != 0
		if openBoxFound {
			time.Sleep(300 * time.Millisecond) // the dialog populates its controls after showing
			pth, _ := syscall.UTF16PtrFromString(tmpFile)
			for _, id := range []uintptr{1152, 1148} {
				if item, _, _ := pGetDlgItem.Call(box, id); item != 0 {
					pSetDlgText.Call(box, id, uintptr(unsafe.Pointer(pth)))
					editFound = true
					break
				}
			}
			pPostMessage.Call(box, wmCommand, idOK, 0)
		}
		select {
		case <-openDone:
		case <-time.After(8 * time.Second):
		}
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}

	if focusAtStart != entryHwnd {
		t.Errorf("focus at start = %#x, want the entry %#x", focusAtStart, entryHwnd)
	}
	if len(changed) != 3 || changed[2] != "abc" {
		t.Errorf("Changed = %v, want [a ab abc] (order, and no echo)", changed)
	}
	if len(activated) != 1 || activated[0] != "abc" {
		t.Errorf("Activated = %v, want [abc]", activated)
	}
	if focusAfterTab != btnHwnd {
		t.Errorf("focus after Tab = %#x, want the button %#x", focusAfterTab, btnHwnd)
	}
	if len(picked) != 1 || picked[0] != 2 || combo.Text.Get() != "terra" {
		t.Errorf("combo: Selected=%v Text=%q, want [2] terra (and Select(0) not reported)", picked, combo.Text.Get())
	}
	if len(listPicked) != 1 || listPicked[0] != 2 || len(listOpened) != 1 || listOpened[0] != 2 || list.SelectedIndex() != 2 {
		t.Errorf("list: Selected=%v Activated=%v index=%d, want [2] [2] 2", listPicked, listOpened, list.SelectedIndex())
	}
	if !boxFound {
		t.Error("the message box never appeared")
	}
	if !asked {
		t.Error("Ask: the Yes command did not read as Yes")
	}
	if !openBoxFound {
		t.Error("the open-file dialog never appeared")
	} else if !editFound {
		t.Error("no file-name field (1152/1148) in the open-file dialog")
	}
	if !openedOK || !strings.EqualFold(openedPath, tmpFile) {
		t.Errorf("OpenFile = %q, %v; want %q, true", openedPath, openedOK, tmpFile)
	}
}
