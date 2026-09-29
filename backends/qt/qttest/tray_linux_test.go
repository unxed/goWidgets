//go:build linux

package qttest

import (
	"os/exec"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
)

// The tray against Qt and a real panel, as the GTK docking test does it:
// Xvfb is the display, stalonetray the status area. The icon docks, a left
// click (activated with reason Trigger) is Activated, a menu entry is its
// item's Clicked, and the tooltip follows the property.
func TestTray(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
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
	// The panel has to own the _NET_SYSTEM_TRAY selection first.
	time.Sleep(1500 * time.Millisecond)

	if !app.Diagnostics().Caps.TrayIcon {
		t.Fatal("Caps.TrayIcon is false")
	}
	show := goWidgets.NewMenuItem("Показать")
	quit := goWidgets.NewMenuItem("Выход")
	tray, err := app.NewTrayIcon("проверка", show, goWidgets.Separator(), quit)
	if err != nil {
		t.Fatalf("иконка не создана: %v", err)
	}
	activated, quits, shows := 0, 0, 0
	tray.Activated.On(app.Scope(), func(struct{}) { activated++ })
	quit.Clicked.On(app.Scope(), func(struct{}) { quits++ })
	show.Clicked.On(app.Scope(), func(struct{}) { shows++ })

	var embedded bool
	var tooltip string
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && !embedded {
			done := make(chan bool, 1)
			app.QueueUpdate(func() { done <- tray.Embedded() })
			embedded = <-done
			time.Sleep(200 * time.Millisecond)
		}
		step := func(f func()) {
			done := make(chan struct{})
			app.QueueUpdate(func() { f(); close(done) })
			<-done
			time.Sleep(100 * time.Millisecond)
		}
		step(func() {
			const trigger = 3
			qt.Call(qt.Sym("_ZN15QSystemTrayIcon9activatedENS_16ActivationReasonE"), qt.TrayHandle(), trigger)
			// QAction::trigger() is inline: activate(Trigger).
			qt.Call(qt.Sym("_ZN7QAction8activateENS_11ActionEventE"), qt.TrayAction(2), 0)
			tray.Tooltip.Set("новая подсказка")
		})
		step(func() {
			tooltip = qt.CallString(qt.Sym("_ZNK15QSystemTrayIcon7toolTipEv"), qt.TrayHandle())
		})
		app.Quit()
	}()
	if err := app.Run(nil); err != nil {
		t.Fatal(err)
	}
	if !embedded {
		t.Fatal("панель не приняла иконку за 10 секунд")
	}
	if activated != 1 {
		t.Errorf("Activated fired %d times, want 1", activated)
	}
	if quits != 1 || shows != 0 {
		t.Errorf("menu: Выход %d, Показать %d; want 1, 0", quits, shows)
	}
	if tooltip != "новая подсказка" {
		t.Errorf("tooltip = %q", tooltip)
	}
}
