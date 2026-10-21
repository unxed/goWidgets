// Command zipgui is the goWidgets archiver showcase: a window over the
// github.com/unxed/zipper engine with the UX of a classic single-window
// archiver (ADR-0019, docs/zipgui/PLAN.md).
//
// Step 1 is the frame: open an archive (argument or «Открыть…») and see what
// it holds in one line (since step 3 with the packed size and ratio, from
// zipper's listing). The list, navigation and commands come in later steps.
//
//	go run . [archive]                            # from showcase/zipgui
//	goWidgets_BACKEND=headless go run . [archive] # no window system
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/cocoa"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
	_ "github.com/unxed/goWidgets/backends/qt"
	_ "github.com/unxed/goWidgets/backends/win32"
	"github.com/unxed/goWidgets/showcase/zipgui/engine"
)

const hint = "Откройте архив: кнопка «Открыть…» или zipgui <архив>"

var archiveFilter = goWidgets.FileFilter{
	Name: "Архивы",
	Patterns: []string{
		"*.zip", "*.7z", "*.rar", "*.tar", "*.tgz", "*.txz", "*.tzst",
		"*.tar.gz", "*.tar.xz", "*.tar.zst", "*.tar.bz2",
	},
}

func main() {
	app, err := goWidgets.NewApp()
	if err != nil {
		log.Fatal(err)
	}
	win, err := app.NewWindow("zipgui", 640, 400)
	if err != nil {
		log.Fatal(err)
	}

	open, err := win.AddButton("Открыть…")
	if err != nil {
		log.Fatal(err)
	}
	status, err := win.AddLabel(hint)
	if err != nil {
		log.Fatal(err)
	}

	// Command row on top, the summary line under it across the window.
	const pad = 8
	if err := win.Constrain(
		open.Left().Eq(win.Left().Plus(pad)),
		open.Top().Eq(win.Top().Plus(pad)),
		status.Left().Eq(open.Left()),
		status.Top().Eq(open.Bottom().Plus(pad)),
		status.Right().Eq(win.Right().Minus(pad)),
	); err != nil {
		log.Fatal(err)
	}
	open.HugWidth()

	show := func(path string) {
		s, err := engine.Open(path)
		if err != nil {
			status.Text.Set(fmt.Sprintf("Не удалось открыть %s: %v", filepath.Base(path), err))
			return
		}
		win.Title.Set("zipgui — " + filepath.Base(path))
		status.Text.Set(describe(filepath.Base(path), s))
	}
	open.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if path, ok := win.OpenFile("Открыть архив", archiveFilter); ok {
			show(path)
		}
	})
	if len(os.Args) > 1 {
		show(os.Args[1])
	}
	open.Focus()

	if err := app.Run(win); err != nil {
		log.Fatal(err)
	}
}

// describe is the summary line for an opened archive. The packed size and
// the ratio (packed to unpacked, as WinRAR's Ratio column) appear only when
// the format tells them: a zip does, a tar stream does not.
func describe(name string, s engine.Summary) string {
	line := fmt.Sprintf("%s — файлов: %d, папок: %d, %s", name, s.Files, s.Dirs, humanSize(s.Size))
	if s.Packed >= 0 && s.Size > 0 {
		line += fmt.Sprintf(", сжато: %s (%d %%)", humanSize(s.Packed), s.Packed*100/s.Size)
	}
	return line
}

// humanSize prints a byte count the way the file list will: bytes as they
// are, larger sizes in binary units with one decimal.
func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d Б", n)
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ"}
	v := float64(n) / 1024
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
