// Command zipgui is the goWidgets archiver showcase: a window over the
// github.com/unxed/zipper engine with the UX of a classic single-window
// archiver (ADR-0019, docs/zipgui/PLAN.md).
//
// The window opens an archive (argument or «Открыть…»), lists its root, and
// navigates directories inside it (docs/zipgui/PLAN.md, steps 4f and 5).
//
//	go run . [archive]                            # from showcase/zipgui
//	goWidgets_BACKEND=headless go run . [archive] # no window system
package main

import (
	"context"
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
	"github.com/unxed/goWidgets/showcase/zipgui/listing"
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

	ui, err := newZipGUI(app, win)
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		if err := ui.open(os.Args[1]); err != nil {
			log.Print(err)
		}
	}

	if err := app.Run(win); err != nil {
		log.Fatal(err)
	}
}

type zipGUI struct {
	win    *goWidgets.Window
	table  *goWidgets.Table
	path   *goWidgets.Label
	status *goWidgets.Label

	tree *listing.Tree
	nav  listing.Nav
}

func newZipGUI(app *goWidgets.App, win *goWidgets.Window) (*zipGUI, error) {
	open, err := win.AddButton("Открыть…")
	if err != nil {
		return nil, err
	}
	status, err := win.AddLabel(hint)
	if err != nil {
		return nil, err
	}
	pathLabel, err := win.AddLabel("/")
	if err != nil {
		return nil, err
	}
	table, err := win.AddTable([]goWidgets.TableColumn{
		{Title: "Name"},
		{Title: "Size", Align: goWidgets.AlignRight},
		{Title: "Packed", Align: goWidgets.AlignRight},
		{Title: "Modified"},
		{Title: "CRC32"},
	})
	if err != nil {
		return nil, err
	}

	const pad = 8
	if err := win.Constrain(
		open.Left().Eq(win.Left().Plus(pad)),
		open.Top().Eq(win.Top().Plus(pad)),
		status.Left().Eq(open.Left()),
		status.Top().Eq(open.Bottom().Plus(pad)),
		status.Right().Eq(win.Right().Minus(pad)),
		pathLabel.Left().Eq(status.Left()),
		pathLabel.Top().Eq(status.Bottom().Plus(pad)),
		table.Left().Eq(pathLabel.Left()),
		table.Right().Eq(status.Right()),
		table.Top().Eq(pathLabel.Bottom().Plus(pad)),
		table.Bottom().Eq(win.Bottom().Minus(pad)),
	); err != nil {
		return nil, err
	}
	open.HugWidth()

	ui := &zipGUI{win: win, table: table, path: pathLabel, status: status}
	open.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if file, ok := win.OpenFile("Открыть архив", archiveFilter); ok {
			if err := ui.open(file); err != nil {
				status.Text.Set(err.Error())
			}
		}
	})
	table.Activated.On(app.Scope(), func(row int) {
		if ui.tree == nil {
			return
		}
		entries := ui.visible()
		if row < 0 || row >= len(entries) {
			return
		}
		entry := entries[row]
		if !entry.IsDir {
			return // Viewing files is outside the 12-step plan.
		}
		if entry.Name == ".." {
			ui.nav.Up()
		} else {
			ui.nav.Enter(entry.Name)
		}
		ui.render()
	})
	win.KeyPressed().On(app.Scope(), func(key goWidgets.Key) {
		if key.KeyDown && key.VirtualKeyCode == 0x08 && ui.tree != nil { // VK_BACK
			ui.nav.Up()
			ui.render()
		}
	})
	open.Focus()
	return ui, nil
}

func (ui *zipGUI) open(file string) error {
	entries, err := engine.List(context.Background(), file)
	if err != nil {
		return fmt.Errorf("Не удалось открыть %s: %w", filepath.Base(file), err)
	}
	ui.tree = listing.NewTree(entries)
	ui.nav = listing.Nav{}
	ui.win.Title.Set("zipgui — " + filepath.Base(file))
	ui.status.Text.Set(describe(filepath.Base(file), engine.SummaryOf(entries)))
	ui.render()
	return nil
}

func (ui *zipGUI) visible() []engine.Entry {
	entries := ui.tree.Children(ui.nav.Path())
	if ui.nav.Path() == "/" {
		return entries
	}
	return append([]engine.Entry{{Name: "..", IsDir: true}}, entries...)
}

func (ui *zipGUI) render() {
	if ui.tree == nil {
		return
	}
	ui.path.Text.Set(ui.nav.Path())
	entries := ui.visible()
	rows := make([][]string, len(entries))
	for i, entry := range entries {
		if entry.Name == ".." {
			rows[i] = []string{"..", "", "", "", ""}
			continue
		}
		name := entry.Name
		if entry.IsDir {
			name += "/"
		}
		packed := "—"
		if entry.Packed >= 0 {
			packed = humanSize(entry.Packed)
		}
		modified := ""
		if !entry.Modified.IsZero() {
			modified = entry.Modified.Format("2006-01-02 15:04")
		}
		crc := ""
		if entry.CRC32 != 0 {
			crc = fmt.Sprintf("%08X", entry.CRC32)
		}
		rows[i] = []string{name, humanSize(entry.Size), packed, modified, crc}
	}
	ui.table.SetRows(rows)
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
