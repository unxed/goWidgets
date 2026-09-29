//go:build windows

// Package quittest checks that the application can quit while a modal
// dialog is up. Its own package: one Win32 application per test binary.
package quittest

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/win32"
)

var pFindWindow = syscall.NewLazyDLL("user32.dll").NewProc("FindWindowW")

// A modal loop takes the WM_QUIT for itself: the open-file dialog ends, and
// the main loop, which never saw the message, used to wait in GetMessage for
// good. This is what hung win32test in CI whenever its dialog was not
// answered in time. Quit has to end Run whatever loop is running.
func TestQuitWhileAModalDialogIsUp(t *testing.T) {
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "win32" {
		t.Skipf("драйвер %s, а проверяется win32", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("quittest", 300, 120)
	if err != nil {
		t.Fatal(err)
	}
	// A hang cannot be failed from another goroutine; end the binary.
	watchdog := time.AfterFunc(20*time.Second, func() {
		fmt.Println("--- FAIL: TestQuitWhileAModalDialogIsUp: Run did not return after Quit")
		os.Exit(1)
	})
	defer watchdog.Stop()

	dialogUp := false
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() { win.OpenFile("quittest-open") })
		title, _ := syscall.UTF16PtrFromString("quittest-open")
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
			if box, _, _ := pFindWindow.Call(0, uintptr(unsafe.Pointer(title))); box != 0 {
				dialogUp = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if !dialogUp {
		t.Log("the dialog was not seen; Quit was tested without it")
	}
}
