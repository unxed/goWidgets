// Command imgy is the first interactive increment of the goWidgets image viewer.
//
// The browser shell stays constraint-based; Canvas pixels are only used for
// image content, never for application controls or layout.
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
	".gif": true, ".jpeg": true, ".jpg": true, ".png": true,
}

var imageFilter = goWidgets.FileFilter{
	Name: "Изображения",
	Patterns: []string{
		"*.gif", "*.jpeg", "*.jpg", "*.png",
	},
}

type imageEntry struct {
	name string
	path string
	size int64
}

func main() {
	start := "."
	if len(os.Args) > 1 && os.Args[1] != "" {
		start = os.Args[1]
	}
	start, _ = filepath.Abs(start)
	initialFile := ""
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		initialFile = start
		start = filepath.Dir(start)
	}

	app, err := goWidgets.NewApp()
	if err != nil {
		log.Fatal(err)
	}
	win, err := app.NewWindow("imgy — image viewer", 1040, 680)
	if err != nil {
		log.Fatal(err)
	}

	title, _ := win.AddLabel("imgy — просмотр изображений")
	pathEdit, _ := win.AddEdit(start)
	openFolder, _ := win.AddButton("Открыть папку")
	openImage, _ := win.AddButton("Открыть файл…")
	files, _ := win.AddListBox(nil)
	var previewBox goWidgets.Box
	var canvas *goWidgets.Canvas
	var imageView *goWidgets.ImageView
	canvas, err = win.AddCanvas()
	if err == nil {
		previewBox = canvas
	} else {
		// Backends not yet at Canvas parity keep the established native preview
		// until their Canvas stage is implemented.
		imageView, err = win.AddImageView()
		if err != nil {
			log.Fatal(err)
		}
		previewBox = imageView
	}
	info, _ := win.AddLabel("Файлов: 0")
	status, _ := win.AddLabel("Выберите изображение")

	const pad = 12
	if err := win.Constrain(
		title.Left().Eq(win.Left().Plus(pad)),
		title.Top().Eq(win.Top().Plus(pad)),
		title.Right().Eq(win.Right().Minus(pad)),

		pathEdit.Left().Eq(title.Left()),
		pathEdit.Top().Eq(title.Bottom().Plus(pad)),
		openImage.Right().Eq(title.Right()),
		openImage.Top().Eq(pathEdit.Top()),
		openFolder.Right().Eq(openImage.Left().Minus(pad)),
		openFolder.Top().Eq(pathEdit.Top()),
		pathEdit.Right().Eq(openFolder.Left().Minus(pad)),

		files.Left().Eq(title.Left()),
		files.Top().Eq(pathEdit.Bottom().Plus(pad)),
		files.Width().Is(280),
		files.Bottom().Eq(status.Top().Minus(pad)),

		previewBox.Left().Eq(files.Right().Plus(pad)),
		previewBox.Top().Eq(files.Top()),
		previewBox.Right().Eq(title.Right()),
		previewBox.Bottom().Eq(status.Top().Minus(pad)),

		info.Left().Eq(previewBox.Left()),
		info.Bottom().Eq(previewBox.Bottom().Minus(pad)),

		status.Left().Eq(title.Left()),
		status.Right().Eq(title.Right()),
		status.Bottom().Eq(win.Bottom().Minus(pad)),
	); err != nil {
		log.Fatal(err)
	}
	info.HugHeight()
	openFolder.HugWidth()
	openImage.HugWidth()
	title.HugHeight()
	status.HugHeight()

	var entries []imageEntry
	var currentImage image.Image
	var folder string
	if canvas != nil {
		canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
			fitImage(frame.Image, currentImage)
		})
	}
	loadFolder := func(dir string) bool {
		dir, _ = filepath.Abs(dir)
		read, err := os.ReadDir(dir)
		if err != nil {
			status.Text.Set("Не удалось открыть папку: " + err.Error())
			return false
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
		currentImage = nil
		if canvas != nil {
			canvas.Invalidate()
		} else {
			imageView.SetPath("")
		}
		status.Text.Set("Папка открыта: " + folder)
		return true
	}
	selectEntry := func(i int) {
		if i < 0 || i >= len(entries) {
			return
		}
		e := entries[i]
		if canvas != nil {
			img, dimensions, err := decodeImage(e.path)
			if err != nil {
				currentImage = nil
				canvas.Invalidate()
				status.Text.Set("Не удалось декодировать изображение: " + err.Error())
				return
			}
			currentImage = img
			canvas.Invalidate()
			info.Text.Set(fmt.Sprintf("%s  •  %s  •  %s", e.name, formatBytes(e.size), dimensions))
		} else {
			imageView.SetPath(e.path)
			info.Text.Set(fmt.Sprintf("%s  •  %s  •  %s", e.name, formatBytes(e.size), imageDimensions(e.path)))
		}
		status.Text.Set("Выбрано: " + e.path)
	}
	selectPath := func(path string) {
		if !loadFolder(filepath.Dir(path)) {
			return
		}
		name := strings.ToLower(filepath.Base(path))
		for i, entry := range entries {
			if strings.ToLower(entry.name) == name {
				files.Select(i)
				selectEntry(i)
				return
			}
		}
		status.Text.Set("Файл не найден в папке: " + path)
	}
	chooseImage := func() {
		path, ok := win.OpenFile("Открыть изображение", imageFilter)
		if ok {
			selectPath(path)
		}
	}

	files.Selected.On(app.Scope(), selectEntry)
	openFolder.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { loadFolder(pathEdit.Text.Get()) })
	openImage.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { chooseImage() })
	pathEdit.Activated.On(app.Scope(), func(string) { loadFolder(pathEdit.Text.Get()) })
	win.KeyPressed().On(app.Scope(), func(key goWidgets.Key) {
		if !key.KeyDown {
			return
		}
		if key.Ctrl() && key.VirtualKeyCode == 0x4F { // Ctrl+O
			chooseImage()
		} else if key.VirtualKeyCode == 0x1B { // Escape
			app.Quit()
		}
	})

	loadFolder(start)
	if initialFile != "" {
		selectPath(initialFile)
	}
	if err := app.Run(win); err != nil {
		log.Fatal(err)
	}
}

func decodeImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, "", err
	}
	b := img.Bounds()
	return img, fmt.Sprintf("%d × %d", b.Dx(), b.Dy()), nil
}

// fitImage paints a centered, aspect-preserving nearest-neighbor preview into
// the Canvas frame. The opaque neutral background also composites transparent
// source pixels consistently across native presentation APIs.
func fitImage(dst *image.RGBA, src image.Image) {
	if dst == nil {
		return
	}
	clear(dst.Pix)
	for i := 0; i < len(dst.Pix); i += 4 {
		dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = 248, 248, 248, 255
	}
	if src == nil || src.Bounds().Empty() || dst.Bounds().Empty() {
		return
	}
	sb, db := src.Bounds(), dst.Bounds()
	scaleX, scaleY := float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy())
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	w, h := int(float64(sb.Dx())*scale), int(float64(sb.Dy())*scale)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	left, top := db.Min.X+(db.Dx()-w)/2, db.Min.Y+(db.Dy()-h)/2
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + y*sb.Dy()/h
		for x := 0; x < w; x++ {
			sx := sb.Min.X + x*sb.Dx()/w
			r, g, b, a := src.At(sx, sy).RGBA()
			alpha := uint8(a >> 8)
			di := dst.PixOffset(left+x, top+y)
			dst.Pix[di+0] = overLight(uint8(r>>8), alpha)
			dst.Pix[di+1] = overLight(uint8(g>>8), alpha)
			dst.Pix[di+2] = overLight(uint8(b>>8), alpha)
			dst.Pix[di+3] = 255
		}
	}
}

func overLight(premultiplied, alpha uint8) uint8 {
	value := uint16(premultiplied) + 248*(255-uint16(alpha))/255
	if value > 255 {
		value = 255
	}
	return uint8(value)
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
