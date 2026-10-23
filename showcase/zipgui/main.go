// Command zipgui is the goWidgets archiver showcase: a window over the
// github.com/unxed/zipper engine with the UX of a classic single-window
// archiver (ADR-0019, docs/zipgui/PLAN.md).
//
// The window opens an archive (argument or «Открыть…»), lists its root,
// navigates directories inside it, and creates archives through «Добавить файл…»
// (docs/zipgui/PLAN.md, steps 5 and 8a).
//
//	go run . [archive]                            # from showcase/zipgui
//	goWidgets_BACKEND=headless go run . [archive] # no window system
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

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

var sourceFileFilter = goWidgets.FileFilter{
	Name:     "Все файлы",
	Patterns: []string{"*"},
}

func archiveSaveFilters() []goWidgets.FileFilter {
	var filters []goWidgets.FileFilter
	for _, format := range engine.Formats() {
		if !format.Write {
			continue
		}
		filters = append(filters, goWidgets.FileFilter{
			Name:     strings.TrimPrefix(format.Ext, "."),
			Patterns: []string{"*" + format.Ext},
		})
	}
	return filters
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
	app                 *goWidgets.App
	win                 *goWidgets.Window
	table               *goWidgets.Table
	path                *goWidgets.Label
	status              *goWidgets.Label
	addFile             *goWidgets.Button
	selectedFiles       *goWidgets.Label
	archiveName         *goWidgets.Edit
	chooseArchive       *goWidgets.Button
	compression         *goWidgets.ComboBox
	createArchiveButton *goWidgets.Button
	dest                *goWidgets.Edit
	mode                *goWidgets.ComboBox
	extractButton       *goWidgets.Button
	cancelButton        *goWidgets.Button
	testButton          *goWidgets.Button

	archivePath string
	sources     []string
	entries     []engine.Entry
	totalSize   int64
	tree        *listing.Tree
	nav         listing.Nav
	cancel      context.CancelFunc
}

func newZipGUI(app *goWidgets.App, win *goWidgets.Window) (*zipGUI, error) {
	open, err := win.AddButton("Открыть…")
	if err != nil {
		return nil, err
	}
	testButton, err := win.AddButton("Проверить")
	if err != nil {
		return nil, err
	}
	addFile, err := win.AddButton("Добавить файл…")
	if err != nil {
		return nil, err
	}
	status, err := win.AddLabel(hint)
	if err != nil {
		return nil, err
	}
	archiveName, err := win.AddEdit("")
	if err != nil {
		return nil, err
	}
	chooseArchive, err := win.AddButton("Имя архива…")
	if err != nil {
		return nil, err
	}
	selectedFiles, err := win.AddLabel("Файлов: 0")
	if err != nil {
		return nil, err
	}
	compressionLabel, err := win.AddLabel("Сжатие:")
	if err != nil {
		return nil, err
	}
	compression, err := win.AddComboBox([]string{"Быстро", "Обычно", "Максимум"}, true)
	if err != nil {
		return nil, err
	}
	compression.Select(1)
	compression.Enabled.Set(false)
	createArchiveButton, err := win.AddButton("Создать архив")
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
	destination, err := win.AddEdit("")
	if err != nil {
		return nil, err
	}
	browse, err := win.AddButton("Обзор…")
	if err != nil {
		return nil, err
	}
	mode, err := win.AddComboBox([]string{"Заменять", "Пропускать"}, true)
	if err != nil {
		return nil, err
	}
	mode.Select(0)
	extractButton, err := win.AddButton("Извлечь")
	if err != nil {
		return nil, err
	}
	cancelButton, err := win.AddButton("Отмена")
	if err != nil {
		return nil, err
	}
	cancelButton.Enabled.Set(false)

	const pad = 8
	if err := win.Constrain(
		open.Left().Eq(win.Left().Plus(pad)),
		open.Top().Eq(win.Top().Plus(pad)),
		testButton.Left().Eq(open.Right().Plus(pad)),
		testButton.Top().Eq(open.Top()),
		addFile.Left().Eq(testButton.Right().Plus(pad)),
		addFile.Top().Eq(open.Top()),
		status.Left().Eq(open.Left()),
		status.Top().Eq(open.Bottom().Plus(pad)),
		status.Right().Eq(win.Right().Minus(pad)),
		pathLabel.Left().Eq(status.Left()),
		pathLabel.Top().Eq(status.Bottom().Plus(pad)),
		table.Left().Eq(pathLabel.Left()),
		table.Right().Eq(status.Right()),
		table.Top().Eq(pathLabel.Bottom().Plus(pad)),
		table.Bottom().Eq(archiveName.Top().Minus(pad)),
		archiveName.Left().Eq(table.Left()),
		archiveName.Right().Eq(chooseArchive.Left().Minus(pad)),
		archiveName.Top().Eq(table.Bottom().Plus(pad)),
		chooseArchive.Right().Eq(table.Right()),
		chooseArchive.Top().Eq(archiveName.Top()),
		selectedFiles.Left().Eq(table.Left()),
		selectedFiles.Top().Eq(archiveName.Bottom().Plus(pad)),
		compressionLabel.Left().Eq(selectedFiles.Right().Plus(pad)),
		compressionLabel.Top().Eq(selectedFiles.Top()),
		compression.Left().Eq(compressionLabel.Right().Plus(pad)),
		compression.Top().Eq(selectedFiles.Top()),
		createArchiveButton.Left().Eq(compression.Right().Plus(pad)),
		createArchiveButton.Right().Eq(table.Right()),
		createArchiveButton.Top().Eq(selectedFiles.Top()),
		destination.Left().Eq(table.Left()),
		destination.Right().Eq(browse.Left().Minus(pad)),
		destination.Top().Eq(selectedFiles.Bottom().Plus(pad)),
		browse.Right().Eq(table.Right()),
		browse.Top().Eq(destination.Top()),
		mode.Left().Eq(table.Left()),
		mode.Top().Eq(destination.Bottom().Plus(pad)),
		extractButton.Left().Eq(mode.Right().Plus(pad)),
		extractButton.Top().Eq(mode.Top()),
		cancelButton.Left().Eq(extractButton.Right().Plus(pad)),
		cancelButton.Right().Eq(table.Right()),
		cancelButton.Top().Eq(mode.Top()),
		cancelButton.Bottom().Eq(win.Bottom().Minus(pad)),
	); err != nil {
		return nil, err
	}
	open.HugWidth()
	testButton.HugWidth()
	addFile.HugWidth()
	browse.HugWidth()
	chooseArchive.HugWidth()
	selectedFiles.HugWidth()
	compressionLabel.HugWidth()
	compression.HugWidth()
	createArchiveButton.HugWidth()
	mode.HugWidth()
	extractButton.HugWidth()
	cancelButton.HugWidth()

	ui := &zipGUI{
		app: app, win: win, table: table, path: pathLabel, status: status,
		addFile: addFile, selectedFiles: selectedFiles, archiveName: archiveName,
		chooseArchive: chooseArchive, compression: compression, createArchiveButton: createArchiveButton,
		dest: destination, mode: mode, extractButton: extractButton,
		cancelButton: cancelButton, testButton: testButton,
	}
	open.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if file, ok := win.OpenFile("Открыть архив", archiveFilter); ok {
			if err := ui.open(file); err != nil {
				status.Text.Set(err.Error())
			}
		}
	})
	addFile.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if file, ok := win.OpenFile("Добавить файл", sourceFileFilter); ok {
			ui.addSource(file)
		}
	})
	chooseArchive.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if file, ok := win.SaveFile("Создать архив", "new-archive.zip", archiveSaveFilters()...); ok {
			archiveName.Text.Set(file)
		}
	})
	archiveName.Text.OnChange(app.Scope(), func(filename, _ string) {
		ui.updateCompressionAvailability(filename)
	})
	createArchiveButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { ui.createArchive() })
	browse.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if folder, ok := win.SelectFolder("Выберите папку для извлечения", destination.Text.Get()); ok {
			destination.Text.Set(folder)
		}
	})
	extractButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { ui.extract() })
	testButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { ui.testArchive() })
	cancelButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if ui.cancel != nil {
			ui.cancel()
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

func (ui *zipGUI) testArchive() {
	if ui.archivePath == "" || ui.tree == nil {
		ui.status.Text.Set("Сначала откройте архив")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	archivePath := ui.archivePath
	totalSize := ui.totalSize
	ui.cancel = cancel
	ui.extractButton.Enabled.Set(false)
	ui.testButton.Enabled.Set(false)
	ui.cancelButton.Enabled.Set(true)
	var progressBytes, progressFiles atomic.Int64
	go func() {
		var failures []engine.TestError
		var testErr error
		done := make(chan struct{})
		go func() {
			failures, testErr = engine.Test(ctx, archivePath, func(bytes, files int64) {
				progressBytes.Store(bytes)
				progressFiles.Store(files)
			})
			close(done)
		}()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				ui.app.QueueUpdate(func() {
					ui.cancel = nil
					ui.extractButton.Enabled.Set(true)
					ui.testButton.Enabled.Set(true)
					ui.cancelButton.Enabled.Set(false)
					cancel()
					if errors.Is(testErr, context.Canceled) {
						ui.status.Text.Set("Проверка отменена")
						return
					}
					if testErr != nil {
						ui.win.Message("Проверка архива", "Ошибка проверки: "+testErr.Error())
						return
					}
					ui.win.Message("Проверка архива", describeTestResults(failures))
				})
				return
			case <-ticker.C:
				bytes, files := progressBytes.Load(), progressFiles.Load()
				percent := 0
				if totalSize > 0 {
					percent = int(bytes * 100 / totalSize)
					if percent > 100 {
						percent = 100
					}
				}
				message := fmt.Sprintf("Проверка: %d%% — файлов: %d", percent, files)
				ui.app.QueueUpdate(func() { ui.status.Text.Set(message) })
			}
		}
	}()
}

func (ui *zipGUI) addSource(filename string) {
	filename, err := filepath.Abs(filename)
	if err != nil {
		ui.status.Text.Set("Не удалось выбрать файл: " + err.Error())
		return
	}
	for _, selected := range ui.sources {
		if selected == filename {
			ui.status.Text.Set("Этот файл уже выбран")
			return
		}
	}
	ui.sources = append(ui.sources, filename)
	ui.selectedFiles.Text.Set(fmt.Sprintf("Файлов: %d", len(ui.sources)))
	ui.status.Text.Set("Выберите имя архива и нажмите «Создать архив»")
}

func (ui *zipGUI) updateCompressionAvailability(filename string) {
	format, ok := engine.WritableFormat(strings.TrimSpace(filename))
	ui.compression.Enabled.Set(ok && format.Level)
	if strings.TrimSpace(filename) == "" {
		return
	}
	if ok && !format.Level {
		ui.status.Text.Set("Для " + format.Ext + " уровень сжатия недоступен; будет использован уровень формата по умолчанию")
	} else if !ok {
		ui.status.Text.Set("Выберите расширение поддерживаемого формата архива")
	}
}

func (ui *zipGUI) createArchive() {
	if len(ui.sources) == 0 {
		ui.status.Text.Set("Сначала добавьте хотя бы один файл")
		return
	}
	destination := strings.TrimSpace(ui.archiveName.Text.Get())
	format, ok := engine.WritableFormat(destination)
	if !ok {
		ui.status.Text.Set("Укажите имя с расширением поддерживаемого формата архива")
		return
	}
	level := 0
	if format.Level {
		levels := [...]int{1, 5, 9}
		index := ui.compression.SelectedIndex()
		if index < 0 || index >= len(levels) {
			index = 1
		}
		level = levels[index]
	}
	sources := append([]string(nil), ui.sources...)
	ui.addFile.Enabled.Set(false)
	ui.chooseArchive.Enabled.Set(false)
	ui.archiveName.Enabled.Set(false)
	ui.compression.Enabled.Set(false)
	ui.createArchiveButton.Enabled.Set(false)
	ui.status.Text.Set("Создаётся архив…")
	go func() {
		err := engine.Create(context.Background(), destination, sources, level)
		ui.app.QueueUpdate(func() {
			ui.addFile.Enabled.Set(true)
			ui.chooseArchive.Enabled.Set(true)
			ui.archiveName.Enabled.Set(true)
			ui.createArchiveButton.Enabled.Set(true)
			if err != nil {
				ui.updateCompressionAvailability(destination)
				ui.status.Text.Set("Не удалось создать архив: " + err.Error())
				return
			}
			ui.sources = nil
			ui.selectedFiles.Text.Set("Файлов: 0")
			ui.archiveName.Text.Set("")
			if err := ui.open(destination); err != nil {
				ui.status.Text.Set("Архив создан, но не удалось открыть: " + err.Error())
			}
		})
	}()
}

func describeTestResults(failures []engine.TestError) string {
	if len(failures) == 0 {
		return "Ошибок нет"
	}
	const limit = 20
	var lines strings.Builder
	for i, failure := range failures {
		if i == limit {
			fmt.Fprintf(&lines, "\nи ещё %d", len(failures)-limit)
			break
		}
		if i > 0 {
			lines.WriteByte('\n')
		}
		fmt.Fprintf(&lines, "%s: %v", failure.Name, failure.Err)
	}
	return lines.String()
}

func (ui *zipGUI) open(file string) error {
	entries, err := engine.List(context.Background(), file)
	if err != nil {
		return fmt.Errorf("Не удалось открыть %s: %w", filepath.Base(file), err)
	}
	ui.tree = listing.NewTree(entries)
	ui.archivePath = file
	ui.entries = entries
	ui.totalSize = 0
	for _, entry := range entries {
		if !entry.IsDir {
			ui.totalSize += entry.Size
		}
	}
	ui.nav = listing.Nav{}
	ui.win.Title.Set("zipgui — " + filepath.Base(file))
	ui.status.Text.Set(describe(filepath.Base(file), engine.SummaryOf(entries)))
	ui.render()
	return nil
}

func (ui *zipGUI) extract() {
	if ui.archivePath == "" || ui.tree == nil {
		ui.status.Text.Set("Сначала откройте архив")
		return
	}
	destination := strings.TrimSpace(ui.dest.Text.Get())
	if destination == "" {
		ui.status.Text.Set("Укажите папку для извлечения")
		return
	}
	var include []string
	selected := ui.table.SelectedIndex()
	visible := ui.visible()
	if selected >= 0 && selected < len(visible) && visible[selected].Name != ".." {
		name := visible[selected].Name
		if dir := strings.TrimPrefix(ui.nav.Path(), "/"); dir != "" {
			name = path.Join(dir, name)
		}
		include = []string{name}
	}
	keepOld := ui.mode.SelectedIndex() == 1
	progressTotal := ui.totalSize
	if len(include) > 0 {
		progressTotal = 0
		for _, entry := range ui.entries {
			name := strings.Trim(entry.Name, "/")
			if !entry.IsDir && (name == include[0] || strings.HasPrefix(name, include[0]+"/")) {
				progressTotal += entry.Size
			}
		}
	}
	extractor, err := engine.NewExtractor(ui.archivePath, destination, include, keepOld)
	if err != nil {
		ui.status.Text.Set(err.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ui.cancel = cancel
	ui.extractButton.Enabled.Set(false)
	ui.cancelButton.Enabled.Set(true)
	go func() {
		defer extractor.Close()
		done := make(chan error, 1)
		go func() { done <- extractor.Extract(ctx) }()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				ui.app.QueueUpdate(func() {
					ui.cancel = nil
					ui.extractButton.Enabled.Set(true)
					ui.cancelButton.Enabled.Set(false)
					cancel()
					if errors.Is(err, context.Canceled) {
						ui.status.Text.Set("Извлечение отменено")
					} else if err != nil {
						ui.status.Text.Set("Ошибка извлечения: " + err.Error())
					} else {
						ui.status.Text.Set("Извлечение завершено")
					}
				})
				return
			case <-ticker.C:
				written, _ := extractor.Written()
				current := extractor.CurrentFile()
				percent := 0
				if progressTotal > 0 {
					percent = int(written * 100 / progressTotal)
					if percent > 100 {
						percent = 100
					}
				}
				message := fmt.Sprintf("%d%% — %s", percent, current)
				ui.app.QueueUpdate(func() { ui.status.Text.Set(message) })
			}
		}
	}()
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
