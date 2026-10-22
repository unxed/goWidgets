package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/headless"
	"github.com/unxed/goWidgets/showcase/zipgui/engine"
)

func TestHumanSize(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0 Б"},
		{1023, "1023 Б"},
		{1024, "1.0 КБ"},
		{1536, "1.5 КБ"},
		{5 << 20, "5.0 МБ"},
		{3 << 30, "3.0 ГБ"},
		{2 << 40, "2.0 ТБ"},
		{2048 << 40, "2048.0 ТБ"},
	} {
		if got := humanSize(tc.n); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	for _, tc := range []struct {
		s    engine.Summary
		want string
	}{
		{engine.Summary{Files: 2, Dirs: 1, Size: 11, Packed: -1}, "a.zip — файлов: 2, папок: 1, 11 Б"},
		{engine.Summary{Files: 2, Dirs: 1, Size: 2048, Packed: 512}, "a.zip — файлов: 2, папок: 1, 2.0 КБ, сжато: 512 Б (25 %)"},
		{engine.Summary{Dirs: 1}, "a.zip — файлов: 0, папок: 1, 0 Б"},
	} {
		if got := describe("a.zip", tc.s); got != tc.want {
			t.Errorf("describe(%+v) = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestArchiveNavigationWindow(t *testing.T) {
	t.Setenv("goWidgets_BACKEND", "headless")
	archivePath := filepath.Join(t.TempDir(), "nested.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	entry, err := zw.Create("sub/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	app, err := goWidgets.NewApp()
	if err != nil {
		t.Fatal(err)
	}
	win, err := app.NewWindow("zipgui test", 640, 400)
	if err != nil {
		t.Fatal(err)
	}
	ui, err := newZipGUI(app, win)
	if err != nil {
		t.Fatal(err)
	}
	if err := ui.open(archivePath); err != nil {
		t.Fatal(err)
	}
	if rows := ui.table.Rows(); len(rows) != 1 || rows[0][0] != "sub/" {
		t.Fatalf("root rows = %v, want only sub/", rows)
	}
	if !headless.OpenTableRow(0) {
		t.Fatal("headless did not activate the sub directory")
	}
	// Backend events are forwarded to the UI queue asynchronously. Pumping
	// until the first idle frame can race the event-forwarding goroutine.
	deadline := time.Now().Add(time.Second)
	for ui.path.Text.Get() != "/sub" && time.Now().Before(deadline) {
		app.PumpOnce()
		runtime.Gosched()
	}
	if got := ui.path.Text.Get(); got != "/sub" {
		t.Fatalf("path after opening sub = %q, want /sub", got)
	}
	if rows := ui.table.Rows(); len(rows) != 2 || rows[0][0] != ".." || rows[1][0] != "b.txt" {
		t.Fatalf("sub rows = %v, want .. and b.txt", rows)
	}

	win.KeyPressed().Emit(goWidgets.Key{VirtualKeyCode: 0x08, KeyDown: true}) // VK_BACK
	if got := ui.path.Text.Get(); got != "/" {
		t.Fatalf("path after Backspace = %q, want /", got)
	}
	if rows := ui.table.Rows(); len(rows) != 1 || rows[0][0] != "sub/" {
		t.Fatalf("root rows after Backspace = %v, want only sub/", rows)
	}
}
