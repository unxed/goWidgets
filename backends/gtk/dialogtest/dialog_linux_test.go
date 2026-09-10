//go:build linux

// Package dialogtest runs a modal message box against a real GTK. Own
// directory: one GTK application per test binary.
package dialogtest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
)

// gtk_dialog_run blocks the UI goroutine in a nested GTK loop. The claim
// worth testing is that the application still runs underneath: a QueueUpdate
// posted while the box is up must execute (here it is what answers the box),
// and the answer must come back to the caller as the right bool.
func TestModalDialogAgainstGTK(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("нет DISPLAY: запускать под Xvfb")
	}
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Skipf("графическая подсистема недоступна: %v", err)
	}
	if app.Diagnostics().Name != "gtk" {
		t.Skipf("драйвер %s, а проверяется gtk", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("dialog", 300, 100)
	if err != nil {
		t.Fatal(err)
	}

	tmp := filepath.Join(t.TempDir(), "цели.txt")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var yes, confirmed, openedOK, savedOK bool
	var openedPath, savedPath string
	answered := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			yes = win.Ask("Вопрос", "Убрать отмеченные?")
			confirmed = win.Confirm("Выход", "Точно?")
			openedPath, openedOK = win.OpenFile("Открыть", goWidgets.FileFilter{Name: "Текст", Patterns: []string{"*.txt"}})
			savedPath, savedOK = win.SaveFile("Сохранить", "новые-цели.txt")
			close(answered)
		})
		// Each box is answered from outside, through the running dialog's
		// handle: accept (1) or reject (2), via QueueUpdate — which is also
		// the proof that the queue drains under gtk_dialog_run.
		for i, resp := range []int32{1, 2, 1, 1} {
			deadline := time.Now().Add(5 * time.Second)
			for gtk.DialogHandle() == 0 && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
			d := gtk.DialogHandle()
			if d == 0 {
				break
			}
			if i == 2 {
				// The open dialog: point it at the file first, as clicking
				// it would, and give the chooser a moment to load the folder.
				app.QueueUpdate(func() { gtk.SelectFile(tmp) })
				time.Sleep(300 * time.Millisecond)
			}
			if i == 3 {
				time.Sleep(300 * time.Millisecond)
			}
			app.QueueUpdate(func() { gtk.RespondDialog(d, resp) })
			for gtk.DialogHandle() == d && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
		}
		select {
		case <-answered:
		case <-time.After(10 * time.Second):
		}
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	select {
	case <-answered:
	default:
		t.Fatal("the dialogs never returned")
	}
	if !yes {
		t.Error("Ask: accept did not read as Yes")
	}
	if confirmed {
		t.Error("Confirm: reject read as OK")
	}
	if !openedOK || openedPath != tmp {
		t.Errorf("OpenFile = %q, %v; want %q, true", openedPath, openedOK, tmp)
	}
	if !savedOK || filepath.Base(savedPath) != "новые-цели.txt" {
		t.Errorf("SaveFile = %q, %v; want …/новые-цели.txt, true", savedPath, savedOK)
	}
}
