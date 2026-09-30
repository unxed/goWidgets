//go:build darwin

package cocoatest

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/cocoa"
)

// Modal dialogs against AppKit: Ask and Confirm in a row, then SaveFile and
// OpenFile, all from one QueueUpdate; each is answered from outside through
// a QueueUpdate too, which is the proof that the queue drains under
// runModal. The panels are answered the way their buttons end the session
// (they run out of process; ok: is not ours to call). NSOpenPanel has no
// public way to preselect a file, so the open panel is cancelled and must
// read as ok=false.
func TestModalDialogs(t *testing.T) {
	onMain(func() {
		app := newApp(t)
		if app == nil {
			return
		}
		win, _ := app.NewWindow("dialog", 300, 100)
		saveDir := t.TempDir()

		var yes, confirmed, savedOK, openedOK, folderOK bool
		var savedPath, folderPath string
		folderDir := t.TempDir()
		answered := make(chan struct{})
		go func() {
			time.Sleep(400 * time.Millisecond)
			app.QueueUpdate(func() {
				yes = win.Ask("Вопрос", "Убрать отмеченные?")
				confirmed = win.Confirm("Выход", "Точно?")
				savedPath, savedOK = win.SaveFile("Сохранить", filepath.Join(saveDir, "новые-цели.txt"))
				_, openedOK = win.OpenFile("Открыть", goWidgets.FileFilter{Name: "Текст", Patterns: []string{"*.txt"}})
				folderPath, folderOK = win.SelectFolder("Выберите каталог", folderDir)
				close(answered)
			})
			button := func(alert objc.ID, i int) objc.ID {
				return cocoa.Msg(cocoa.Msg(alert, "buttons"), "objectAtIndex:", uint64(i))
			}
			answers := []func(d objc.ID){
				func(d objc.ID) { cocoa.Msg(button(d, 0), "performClick:", objc.ID(0)) }, // Yes
				func(d objc.ID) { cocoa.Msg(button(d, 1), "performClick:", objc.ID(0)) }, // Cancel
				func(d objc.ID) { cocoa.EndModal(1) },                                    // NSModalResponseOK: Save
				func(d objc.ID) { cocoa.EndModal(0) },                                    // NSModalResponseCancel
				func(d objc.ID) { cocoa.EndModal(1) },                                    // NSModalResponseOK: folder
			}
			for i, answer := range answers {
				var d objc.ID
				deadline := time.Now().Add(8 * time.Second)
				for d == 0 && time.Now().Before(deadline) {
					time.Sleep(100 * time.Millisecond)
					d = cocoa.DialogHandle()
				}
				if d == 0 {
					t.Errorf("dialog %d never came up", i)
					break
				}
				time.Sleep(400 * time.Millisecond) // let a panel settle
				app.QueueUpdate(func() { answer(d) })
				for cocoa.DialogHandle() == d && time.Now().Before(deadline) {
					time.Sleep(50 * time.Millisecond)
				}
			}
			select {
			case <-answered:
			case <-time.After(10 * time.Second):
			}
			app.Quit()
		}()
		if err := app.Run(win); err != nil {
			t.Error(err)
		}
		select {
		case <-answered:
		default:
			t.Error("the dialogs never returned")
			return
		}
		if !yes {
			t.Error("Ask: Yes did not read as true")
		}
		if confirmed {
			t.Error("Confirm: Cancel read as OK")
		}
		if !savedOK || filepath.Base(savedPath) != "новые-цели.txt" {
			t.Errorf("SaveFile = %q, %v; want …/новые-цели.txt, true", savedPath, savedOK)
		}
		if openedOK {
			t.Error("OpenFile: a cancelled panel read as ok")
		}
		if !folderOK || folderPath != folderDir {
			t.Errorf("SelectFolder = %q, %v; want %q, true", folderPath, folderOK, folderDir)
		}
	})
}

// The tray: an NSStatusItem in the menu bar. A left click is Activated, a
// menu entry is its item's Clicked, the tooltip follows the property.
func TestTray(t *testing.T) {
	onMain(func() {
		app := newApp(t)
		if app == nil {
			return
		}
		show := goWidgets.NewMenuItem("Показать")
		quit := goWidgets.NewMenuItem("Выход")
		tray, err := app.NewTrayIcon("проверка", show, goWidgets.Separator(), quit)
		if err != nil {
			t.Errorf("tray: %v", err)
			return
		}
		activated, quits, shows := 0, 0, 0
		tray.Activated.On(app.Scope(), func(struct{}) { activated++ })
		quit.Clicked.On(app.Scope(), func(struct{}) { quits++ })
		show.Clicked.On(app.Scope(), func(struct{}) { shows++ })

		var embedded bool
		var tooltip string
		go func() {
			deadline := time.Now().Add(5 * time.Second)
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
				cocoa.Msg(cocoa.TrayButton(), "performClick:", objc.ID(0))
				item := cocoa.TrayMenuItem(2)
				cocoa.Msg(cocoa.Msg(item, "menu"), "performActionForItemAtIndex:", int64(2))
				tray.Tooltip.Set("новая подсказка")
			})
			step(func() { tooltip = cocoa.GoString(cocoa.Msg(cocoa.TrayButton(), "toolTip")) })
			app.Quit()
		}()
		if err := app.Run(nil); err != nil {
			t.Error(err)
		}
		if !embedded {
			t.Error("the status item never reached the menu bar")
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
	})
}
