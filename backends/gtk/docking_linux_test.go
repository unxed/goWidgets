//go:build linux

package gtk_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
)

// Whether a status-area icon actually appears was the one thing about this
// library that seemed to need a human with a desktop. It does not: Xvfb
// provides the display and any standalone tray manager provides the panel, so
// docking becomes an assertion like any other.
//
// This lives in its own package on purpose. GTK keeps process-global state and
// initialises against one thread, so a binary that has already built an
// application on another driver cannot then build one on GTK — the two together
// crash, while either alone is fine. A separate package means a separate test
// binary, which is the supported way to give a toolkit a process to itself.
//
// The test skips without a display or a tray manager, so an ordinary `go test`
// on a developer machine is unaffected. CI supplies both.
func TestTrayDocksIntoARealPanel(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("нет DISPLAY: запускать под Xvfb")
	}
	panelBin := ""
	for _, name := range []string{"stalonetray", "trayer"} {
		if p, err := exec.LookPath(name); err == nil {
			panelBin = p
			break
		}
	}
	if panelBin == "" {
		t.Skip("нет трей-менеджера (stalonetray или trayer)")
	}

	panel := exec.Command(panelBin)
	if err := panel.Start(); err != nil {
		t.Skipf("трей-менеджер не запустился: %v", err)
	}
	defer func() {
		_ = panel.Process.Kill()
		_ = panel.Wait()
	}()
	// The panel has to own the _NET_SYSTEM_TRAY selection before an icon can
	// dock; without this wait the test races the panel's startup.
	time.Sleep(1500 * time.Millisecond)

	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "gtk" {
		t.Skipf("драйвер %s, а трей проверяется на gtk", app.Diagnostics().Name)
	}

	tray, err := app.NewTrayIcon("проверка", goWidgets.NewMenuItem("Выход"))
	if err != nil {
		t.Fatalf("иконка не создана: %v", err)
	}

	// Docking is asynchronous: the icon asks the panel to embed it and the
	// answer arrives through the event loop, so the loop has to run.
	embedded := make(chan bool, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			done := make(chan bool, 1)
			app.QueueUpdate(func() { done <- tray.Embedded() })
			if <-done {
				embedded <- true
				app.Quit()
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		embedded <- false
		app.Quit()
	}()

	if err := app.Run(nil); err != nil {
		t.Fatalf("цикл событий: %v", err)
	}
	if !<-embedded {
		t.Fatal("панель не приняла иконку за 10 секунд")
	}
}
