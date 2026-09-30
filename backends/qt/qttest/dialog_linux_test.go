//go:build linux

package qttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
)

// Modal dialogs against Qt: Ask, Confirm, OpenFile and SaveFile in a row
// from one QueueUpdate, each answered from outside — through a QueueUpdate
// too, which is the proof that the queue drains under QDialog::exec. The
// buttons are Qt's standard ones in the user's language: the child runs
// under a Russian locale, and a Russian desktop gets "Да", not "Yes".
func TestModalDialogs(t *testing.T) {
	if os.Getenv(childEnv) == "" {
		t.Setenv("LC_ALL", "ru_RU.UTF-8")
	}
	app := perMajor(t)
	if app == nil {
		return
	}
	win, _ := app.NewWindow("dialog", 300, 100)

	tmp := filepath.Join(t.TempDir(), "цели.txt")
	if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveDir := t.TempDir()

	var yes, confirmed, openedOK, savedOK bool
	var openedPath, savedPath, yesLabel string
	answered := make(chan struct{})
	button := func(box uintptr, sb uintptr) uintptr {
		return qt.Call(qt.Sym("_ZNK11QMessageBox6buttonENS_14StandardButtonE"), box, sb)
	}
	click := func(b uintptr) { qt.Call(qt.Sym("_ZN15QAbstractButton5clickEv"), b) }
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			yes = win.Ask("Вопрос", "Убрать отмеченные?")
			confirmed = win.Confirm("Выход", "Точно?")
			openedPath, openedOK = win.OpenFile("Открыть", goWidgets.FileFilter{Name: "Текст", Patterns: []string{"*.txt"}})
			savedPath, savedOK = win.SaveFile("Сохранить", filepath.Join(saveDir, "новые-цели.txt"))
			close(answered)
		})
		answers := []func(d uintptr){
			func(d uintptr) {
				b := button(d, 0x4000) // Yes
				yesLabel = qt.CallString(qt.Sym("_ZNK15QAbstractButton4textEv"), b)
				click(b)
			},
			func(d uintptr) { click(button(d, 0x400000)) }, // Cancel
			func(d uintptr) {
				// Type the path into the file name field, which has the
				// focus; selectFile does nothing while it does.
				qt.CallWithString(qt.Sym("_ZN9QLineEdit7setTextERK7QString"), qt.FocusWidget(), tmp)
				qt.Call(qt.Sym("_ZN11QFileDialog6acceptEv"), d)
			},
			func(d uintptr) { qt.Call(qt.Sym("_ZN11QFileDialog6acceptEv"), d) },
		}
		for _, answer := range answers {
			var d uintptr
			deadline := time.Now().Add(5 * time.Second)
			for d == 0 && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
				got := make(chan uintptr)
				app.QueueUpdate(func() { got <- qt.ModalWidget() })
				d = <-got
			}
			if d == 0 {
				break
			}
			time.Sleep(300 * time.Millisecond) // let a file dialog read its folder
			app.QueueUpdate(func() { answer(d) })
			// The next box may well reuse this one's address (they live on
			// the stack of QMessageBox's static functions), so wait out the
			// close rather than watch for a different pointer.
			time.Sleep(500 * time.Millisecond)
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
		t.Error("Ask: Yes did not read as true")
	}
	if strings.ReplaceAll(yesLabel, "&", "") != "Да" {
		t.Errorf("Yes button reads %q, want the Russian «Да»", yesLabel)
	}
	if confirmed {
		t.Error("Confirm: Cancel read as OK")
	}
	if !openedOK || openedPath != tmp {
		t.Errorf("OpenFile = %q, %v; want %q, true", openedPath, openedOK, tmp)
	}
	if !savedOK || savedPath != filepath.Join(saveDir, "новые-цели.txt") {
		t.Errorf("SaveFile = %q, %v; want …/новые-цели.txt, true", savedPath, savedOK)
	}
}

func TestExistingDirectoryDialog(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	win, _ := app.NewWindow("folder-dialog", 300, 100)
	want := t.TempDir()
	var got string
	var ok bool
	done := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.QueueUpdate(func() {
			got, ok = win.SelectFolder("Выберите каталог", want)
			close(done)
		})
		var d uintptr
		deadline := time.Now().Add(5 * time.Second)
		for d == 0 && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			ready := make(chan uintptr, 1)
			app.QueueUpdate(func() { ready <- qt.ModalWidget() })
			d = <-ready
		}
		if d != 0 {
			app.QueueUpdate(func() {
				qt.CallWithString(qt.Sym("_ZN11QFileDialog12setDirectoryERK7QString"), d, want)
				qt.Call(qt.Sym("_ZN11QFileDialog6acceptEv"), d)
			})
		}
		select {
		case <-done:
		case <-time.After(8 * time.Second):
		}
		app.Quit()
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("directory chooser never returned")
	}
	if !ok || got != want {
		t.Errorf("SelectFolder = %q, %v; want %q, true", got, ok, want)
	}
}
