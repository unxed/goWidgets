// Command imgy is the first working increment of the goWidgets image viewer.
//
// The first increment implements the core browse loop: choose a folder, browse
// its images, select one, and keep the selected image and metadata visible.
//
// Every relationship is declared through goWidgets constraints. The showcase
// contains no pixel coordinates, so the same shell can become a real image
// viewer as ImageView grows on each backend.
package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/cocoa"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
	_ "github.com/unxed/goWidgets/backends/qt"
	_ "github.com/unxed/goWidgets/backends/win32"
)

var imageExtensions = map[string]bool{
	".bmp": true, ".cur": true, ".gif": true, ".heic": true, ".ico": true,
	".jpeg": true, ".jpg": true, ".jxl": true, ".pdf": true, ".png": true,
	".psd": true, ".svg": true, ".tga": true, ".tif": true, ".tiff": true,
	".webp": true,
}

type imageEntry struct {
	name string
	path string
	size int64
}

func main() {
	root := "."
	if len(os.Args) > 1 && os.Args[1] != "" {
		root = os.Args[1]
	}
	root, _ = filepath.Abs(root)

	app, err := goWidgets.NewApp()
	if err != nil {
		log.Fatal(err)
	}
	win, err := app.NewWindow("imgy — image viewer", 1040, 680)
	if err != nil {
		log.Fatal(err)
	}

	title, _ := win.AddLabel("imgy — image browser")
	pathEdit, _ := win.AddEdit(root)
	open, _ := win.AddButton("Открыть папку")
	files, _ := win.AddListBox(nil)
	preview, _ := win.AddImageView()
	info, _ := win.AddLabel("Файлов: 0")
	fullscreen, _ := win.AddButton("Полный экран")
	status, _ := win.AddLabel("Выберите изображение")

	const pad = 12
	if err := win.Constrain(
		title.Left().Eq(win.Left().Plus(pad)),
		title.Top().Eq(win.Top().Plus(pad)),
		title.Right().Eq(win.Right().Minus(pad)),

		pathEdit.Left().Eq(title.Left()),
		pathEdit.Top().Eq(title.Bottom().Plus(pad)),
		open.Right().Eq(title.Right()),
		open.Top().Eq(pathEdit.Top()),
		pathEdit.Right().Eq(open.Left().Minus(pad)),

		files.Left().Eq(title.Left()),
		files.Top().Eq(pathEdit.Bottom().Plus(pad)),
		files.Width().Is(280),
		files.Bottom().Eq(status.Top().Minus(pad)),

		preview.Left().Eq(files.Right().Plus(pad)),
		preview.Top().Eq(files.Top()),
		preview.Right().Eq(title.Right()),
		preview.Bottom().Eq(status.Top().Minus(pad)),

		info.Left().Eq(preview.Left()),
		info.Bottom().Eq(preview.Bottom().Minus(pad)),
		fullscreen.Right().Eq(preview.Right()),
		fullscreen.Bottom().Eq(preview.Bottom().Minus(pad)),

		status.Left().Eq(title.Left()),
		status.Right().Eq(title.Right()),
		status.Bottom().Eq(win.Bottom().Minus(pad)),
	); err != nil {
		log.Fatal(err)
	}
	info.HugHeight()
	fullscreen.HugWidth()
	open.HugWidth()
	// The title and the status line are one-line rows: the vertical slack
	// belongs to the list and the preview between them. Without these the
	// solver gave it to the status line, which is one label and so cheaper
	// to stretch than the list and the preview together.
	title.HugHeight()
	status.HugHeight()

	var entries []imageEntry
	var folder string
	load := func(dir string) {
		dir, _ = filepath.Abs(dir)
		read, err := os.ReadDir(dir)
		if err != nil {
			status.Text.Set("Не удалось открыть папку: " + err.Error())
			return
		}
		entries = entries[:0]
		for _, item := range read {
			if item.IsDir() || !imageExtensions[strings.ToLower(filepath.Ext(item.Name()))] {
				continue
			}
			path := filepath.Join(dir, item.Name())
			st, err := item.Info()
			if err == nil {
				entries = append(entries, imageEntry{name: item.Name(), path: path, size: st.Size()})
			}
		}
		sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name) })
		folder = dir
		labels := make([]string, len(entries))
		for i := range entries {
			labels[i] = entries[i].name
		}
		files.SetItems(labels)
		files.Select(-1)
		pathEdit.Text.Set(folder)
		info.Text.Set(fmt.Sprintf("Файлов: %d", len(entries)))
		preview.SetPath("")
		status.Text.Set("Папка открыта: " + folder)
	}
	selectEntry := func(i int) {
		if i < 0 || i >= len(entries) {
			return
		}
		e := entries[i]
		preview.SetPath(e.path)
		dimensions := imageDimensions(e.path)
		info.Text.Set(fmt.Sprintf("%s  •  %s  •  %s", e.name, formatBytes(e.size), dimensions))
		status.Text.Set("Выбрано: " + e.path)
	}

	files.Selected.On(app.Scope(), selectEntry)
	open.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { load(pathEdit.Text.Get()) })
	pathEdit.Activated.On(app.Scope(), func(string) { load(pathEdit.Text.Get()) })
	fullscreen.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		fullscreen.Text.Set("Полный экран (следующий этап)")
		status.Text.Set("Просмотр и метаданные работают; полноэкранное управление — следующий этап")
	})
	load(root)

	if err := app.Run(win); err != nil {
		log.Fatal(err)
	}
}

func imageDimensions(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "размеры недоступны"
	}
	defer f.Close()
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return "размеры недоступны"
	}
	return fmt.Sprintf("%d × %d", config.Width, config.Height)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KiB", float64(n)/1024)
}
