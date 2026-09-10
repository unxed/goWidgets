//go:build windows

// Package win32test exercises the Win32 driver against a real window: focus
// before Show, typing order, Enter, Tab. It runs on the Windows CI runner and
// under Wine (GOOS=windows go test -c, then wine).
package win32test

import (
	"syscall"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/win32"
	"github.com/unxed/goWidgets/core"
)

var (
	user32       = syscall.NewLazyDLL("user32.dll")
	pGetFocus    = user32.NewProc("GetFocus")
	pPostMessage = user32.NewProc("PostMessageW")
)

const (
	wmKeyDown = 0x0100
	wmKeyUp   = 0x0101
	wmChar    = 0x0102
	vkReturn  = 0x0D
	vkTab     = 0x09
)

func TestEntryFocusAndKeys(t *testing.T) {
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
	e, _ := win.AddEntry("")
	if err := win.Constrain(
		e.Left().Eq(win.Left().Plus(8)), e.Top().Eq(win.Top().Plus(8)),
		btn.Left().Eq(e.Right().Plus(8)), btn.Top().Eq(e.Top()),
	); err != nil {
		t.Fatal(err)
	}
	e.Focus() // before Show: must still land

	var changed, activated []string
	e.Changed.On(app.Scope(), func(v string) { changed = append(changed, v) })
	e.Activated.On(app.Scope(), func(v string) { activated = append(activated, v) })

	focus := func() uintptr { f, _, _ := pGetFocus.Call(); return f }
	var focusAtStart, focusAfterTab, entryHwnd, btnHwnd uintptr
	step := func(delay time.Duration, f func()) {
		time.Sleep(delay)
		done := make(chan struct{})
		app.QueueUpdate(func() { f(); close(done) })
		<-done
	}
	go func() {
		step(500*time.Millisecond, func() {
			entryHwnd, btnHwnd = win32.WidgetHandle(core.KindEntry), win32.WidgetHandle(core.KindButton)
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
}
