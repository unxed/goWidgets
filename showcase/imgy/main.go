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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/cocoa"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
	_ "github.com/unxed/goWidgets/backends/qt"
	_ "github.com/unxed/goWidgets/backends/win32"
	"github.com/unxed/goWidgets/showcase/imgy/viewport"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var imageExtensions = map[string]bool{
	".bmp": true, ".gif": true, ".jpeg": true, ".jpg": true,
	".png": true, ".tif": true, ".tiff": true, ".webp": true,
}

var imageFilter = goWidgets.FileFilter{
	Name: "Изображения",
	Patterns: []string{
		"*.bmp", "*.gif", "*.jpeg", "*.jpg", "*.png", "*.tif", "*.tiff", "*.webp",
	},
}

type imageEntry struct {
	name    string
	path    string
	size    int64
	modTime time.Time
}

const (
	sortByName = iota
	sortByType
	sortBySize
	sortByDate
)

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
	sortLabel, _ := win.AddLabel("Сортировка:")
	sortBox, _ := win.AddComboBox([]string{"Имя", "Тип", "Размер", "Дата изменения"}, true)
	sortBox.Select(sortByName)
	sortDirection, _ := win.AddButton("По возрастанию")
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

		sortLabel.Left().Eq(title.Left()),
		sortLabel.Top().Eq(pathEdit.Bottom().Plus(pad)),
		sortBox.Left().Eq(sortLabel.Right().Plus(6)),
		sortBox.Top().Eq(sortLabel.Top()),
		sortBox.Width().Is(180),
		sortDirection.Left().Eq(sortBox.Right().Plus(pad)),
		sortDirection.Top().Eq(sortBox.Top()),

		files.Left().Eq(title.Left()),
		files.Top().Eq(sortLabel.Bottom().Plus(pad)),
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
	sortLabel.HugHeight()
	sortBox.HugHeight()
	sortDirection.HugWidth()
	sortDirection.HugHeight()
	title.HugHeight()
	status.HugHeight()

	var entries []imageEntry
	var currentImage image.Image
	var folder string
	selectedIndex := -1
	sortMode, sortDescending := sortByName, false
	viewState := viewport.New(viewport.Size{}, viewport.Size{})
	var renderGeneration atomic.Uint64
	var qualityTimer *time.Timer
	var qualityFrame *image.RGBA
	var qualityFrameGeneration uint64
	var scheduleQuality func(*image.RGBA)
	var invalidateView func()
	invalidateView = func() {
		renderGeneration.Add(1)
		qualityFrame = nil
		qualityFrameGeneration = 0
		if qualityTimer != nil {
			qualityTimer.Stop()
			qualityTimer = nil
		}
		if canvas != nil {
			canvas.Invalidate()
		}
	}
	if canvas != nil {
		canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
			area := viewport.Size{W: float64(frame.Image.Bounds().Dx()), H: float64(frame.Image.Bounds().Dy())}
			if area != viewState.Area {
				renderGeneration.Add(1)
				qualityFrame = nil
				qualityFrameGeneration = 0
				if qualityTimer != nil {
					qualityTimer.Stop()
					qualityTimer = nil
				}
				viewState.Area = area
			}
			generation := renderGeneration.Load()
			if qualityFrame != nil && qualityFrameGeneration == generation && qualityFrame.Bounds().Size() == frame.Image.Bounds().Size() {
				copy(frame.Image.Pix, qualityFrame.Pix)
				return
			}
			paintViewport(frame.Image, currentImage, viewState)
			if scheduleQuality != nil {
				scheduleQuality(frame.Image)
			}
		})
	}
	scheduleQuality = func(frame *image.RGBA) {
		if currentImage == nil || qualityFrame != nil || qualityTimer != nil {
			return
		}
		generation := renderGeneration.Load()
		src, view, bounds := currentImage, viewState, frame.Bounds()
		qualityTimer = time.AfterFunc(180*time.Millisecond, func() {
			quality := image.NewRGBA(bounds)
			paintViewportQuality(quality, src, view, func() bool { return generation == renderGeneration.Load() })
			if generation != renderGeneration.Load() {
				return
			}
			app.QueueUpdate(func() {
				if generation != renderGeneration.Load() {
					return
				}
				qualityTimer = nil
				qualityFrame, qualityFrameGeneration = quality, generation
				canvas.Invalidate()
			})
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
		selectedIndex = -1
		for _, item := range read {
			if item.IsDir() || !imageExtensions[strings.ToLower(filepath.Ext(item.Name()))] {
				continue
			}
			path := filepath.Join(dir, item.Name())
			st, err := item.Info()
			if err == nil {
				entries = append(entries, imageEntry{name: item.Name(), path: path, size: st.Size(), modTime: st.ModTime()})
			}
		}
		sortEntries(entries, sortMode, sortDescending)
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
		viewState.Image = viewport.Size{}
		viewState.Pan = viewport.Point{}
		viewState.Zoom = 1
		if canvas != nil {
			invalidateView()
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
		selectedIndex = i
		e := entries[i]
		if canvas != nil {
			img, dimensions, err := decodeImage(e.path)
			if err != nil {
				currentImage = nil
				invalidateView()
				status.Text.Set("Не удалось декодировать изображение: " + err.Error())
				return
			}
			currentImage = img
			mode := viewState.Mode
			viewState = viewport.New(viewport.Size{W: float64(img.Bounds().Dx()), H: float64(img.Bounds().Dy())}, viewState.Area)
			viewState.Mode = mode
			invalidateView()
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
	setViewMode := func(mode viewport.Mode) {
		viewState.SetMode(mode)
		if canvas != nil {
			invalidateView()
		}
	}
	navigate := func(index int) {
		if len(entries) == 0 {
			return
		}
		if index < 0 {
			index = 0
		}
		if index >= len(entries) {
			index = len(entries) - 1
		}
		files.Select(index)
		selectEntry(index)
	}
	applySorting := func() {
		selectedPath := ""
		if selectedIndex >= 0 && selectedIndex < len(entries) {
			selectedPath = entries[selectedIndex].path
		}
		sortEntries(entries, sortMode, sortDescending)
		labels := make([]string, len(entries))
		selectedIndex = -1
		for i := range entries {
			labels[i] = entries[i].name
			if entries[i].path == selectedPath {
				selectedIndex = i
			}
		}
		files.SetItems(labels)
		if selectedIndex >= 0 {
			files.Select(selectedIndex)
			selectEntry(selectedIndex)
		}
	}

	files.Selected.On(app.Scope(), selectEntry)
	sortBox.Selected.On(app.Scope(), func(index int) {
		if index >= sortByName && index <= sortByDate {
			sortMode = index
			applySorting()
		}
	})
	sortDirection.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		sortDescending = !sortDescending
		if sortDescending {
			sortDirection.Text.Set("По убыванию")
		} else {
			sortDirection.Text.Set("По возрастанию")
		}
		applySorting()
	})
	if canvas != nil {
		var dragging bool
		var lastPointer goWidgets.MouseInfo
		canvas.MouseWheel.On(app.Scope(), func(mouse goWidgets.MouseInfo) {
			if mouse.Mods&goWidgets.ModControl == 0 {
				if step := wheelNavigationDelta(mouse.Delta); step != 0 {
					navigate(selectedIndex + step)
				}
				return
			}
			if currentImage == nil {
				return
			}
			scale := win.Scale().Scale
			if scale <= 0 {
				scale = 1
			}
			viewState.ZoomAt(viewport.Point{X: mouse.X * scale, Y: mouse.Y * scale}, math.Pow(1.2, mouse.Delta))
			viewState.ClampPan()
			invalidateView()
		})
		canvas.MouseDown.On(app.Scope(), func(mouse goWidgets.MouseInfo) {
			if currentImage != nil {
				if factor := shiftClickZoomFactor(mouse); factor != 0 {
					scale := win.Scale().Scale
					if scale <= 0 {
						scale = 1
					}
					viewState.ZoomAt(viewport.Point{X: mouse.X * scale, Y: mouse.Y * scale}, factor)
					viewState.ClampPan()
					invalidateView()
					return
				}
			}
			if mouse.Button == goWidgets.MouseLeft && currentImage != nil {
				dragging, lastPointer = true, mouse
			}
		})
		canvas.MouseMove.On(app.Scope(), func(mouse goWidgets.MouseInfo) {
			if !dragging {
				return
			}
			scale := win.Scale().Scale
			if scale <= 0 {
				scale = 1
			}
			viewState.PanBy(viewport.Point{X: (mouse.X - lastPointer.X) * scale, Y: (mouse.Y - lastPointer.Y) * scale})
			viewState.ClampPan()
			lastPointer = mouse
			invalidateView()
		})
		canvas.MouseUp.On(app.Scope(), func(mouse goWidgets.MouseInfo) {
			if mouse.Button == goWidgets.MouseLeft {
				dragging = false
			}
		})
	}
	openFolder.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if folder, ok := win.SelectFolder("Выберите папку с изображениями", pathEdit.Text.Get()); ok {
			loadFolder(folder)
		}
	})
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
		} else {
			if mode, zoom, ok := zoomShortcut(key.Char); ok {
				setViewMode(mode)
				if mode == viewport.Actual {
					viewState.Zoom = zoom
					viewState.ClampPan()
					invalidateView()
				}
			} else if factor := zoomStepFactor(key.Char); factor != 0 && currentImage != nil {
				viewState.ZoomAt(viewport.Point{X: viewState.Area.W / 2, Y: viewState.Area.H / 2}, factor)
				viewState.ClampPan()
				invalidateView()
			} else if step := keyNavigationDelta(key.VirtualKeyCode); step != 0 {
				navigate(selectedIndex + step)
			} else {
				switch key.VirtualKeyCode {
				case 0x24: // Home
					navigate(0)
				case 0x23: // End
					navigate(len(entries) - 1)
				}
			}
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

func zoomShortcut(char rune) (viewport.Mode, float64, bool) {
	switch {
	case char >= '1' && char <= '9':
		return viewport.Actual, float64(char - '0'), true
	case char == 'a' || char == 'A' || char == '/':
		return viewport.Actual, 1, true
	case char == 'b' || char == 'B' || char == '*' || char == 'f' || char == 'F':
		return viewport.Fit, 1, true
	default:
		return 0, 0, false
	}
}

func zoomStepFactor(char rune) float64 {
	switch char {
	case '+':
		return 1.2
	case '-':
		return 1 / 1.2
	default:
		return 0
	}
}

func shiftClickZoomFactor(mouse goWidgets.MouseInfo) float64 {
	if mouse.Mods&goWidgets.ModShift == 0 {
		return 0
	}
	switch mouse.Button {
	case goWidgets.MouseLeft:
		return 1.2
	case goWidgets.MouseRight:
		return 1 / 1.2
	default:
		return 0
	}
}

func wheelNavigationDelta(delta float64) int {
	if delta > 0 {
		return -1
	}
	if delta < 0 {
		return 1
	}
	return 0
}

func keyNavigationDelta(virtualKeyCode uint16) int {
	switch virtualKeyCode {
	case 0x08, 0x25, 0x21: // Backspace, Left, Page Up
		return -1
	case 0x20, 0x27, 0x22: // Space, Right, Page Down
		return 1
	default:
		return 0
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
	v := viewport.New(
		viewport.Size{W: float64(srcSizeX(src)), H: float64(srcSizeY(src))},
		viewport.Size{W: float64(dst.Bounds().Dx()), H: float64(dst.Bounds().Dy())},
	)
	paintViewport(dst, src, v)
}

func srcSizeX(src image.Image) int {
	if src == nil {
		return 0
	}
	return src.Bounds().Dx()
}

func srcSizeY(src image.Image) int {
	if src == nil {
		return 0
	}
	return src.Bounds().Dy()
}

func paintViewport(dst *image.RGBA, src image.Image, view viewport.Viewport) {
	paintViewportFiltered(dst, src, view, false, nil)
}

func paintViewportQuality(dst *image.RGBA, src image.Image, view viewport.Viewport, current func() bool) {
	paintViewportFiltered(dst, src, view, true, current)
}

func paintViewportFiltered(dst *image.RGBA, src image.Image, view viewport.Viewport, quality bool, current func() bool) {
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
	view.Image = viewport.Size{W: float64(sb.Dx()), H: float64(sb.Dy())}
	view.Area = viewport.Size{W: float64(db.Dx()), H: float64(db.Dy())}
	transform := view.Transform()
	if transform.Scale <= 0 {
		return
	}
	for y := db.Min.Y; y < db.Max.Y; y++ {
		if current != nil && y%16 == 0 && !current() {
			return
		}
		sourceY := (float64(y-db.Min.Y) + 0.5 - transform.Offset.Y) / transform.Scale
		if sourceY < 0 || sourceY >= float64(sb.Dy()) {
			continue
		}
		sy := sb.Min.Y + int(math.Floor(sourceY))
		for x := db.Min.X; x < db.Max.X; x++ {
			sourceX := (float64(x-db.Min.X) + 0.5 - transform.Offset.X) / transform.Scale
			if sourceX < 0 || sourceX >= float64(sb.Dx()) {
				continue
			}
			sx := sb.Min.X + int(math.Floor(sourceX))
			var r, g, b, a uint32
			if quality {
				r, g, b, a = sampleCatmullRom(src, sb, float64(sb.Min.X)+sourceX-0.5, float64(sb.Min.Y)+sourceY-0.5)
			} else {
				r, g, b, a = src.At(sx, sy).RGBA()
			}
			alpha := uint8(a >> 8)
			di := dst.PixOffset(x, y)
			dst.Pix[di+0] = overLight(uint8(r>>8), alpha)
			dst.Pix[di+1] = overLight(uint8(g>>8), alpha)
			dst.Pix[di+2] = overLight(uint8(b>>8), alpha)
			dst.Pix[di+3] = 255
		}
	}
}

func sampleCatmullRom(src image.Image, bounds image.Rectangle, x, y float64) (r, g, b, a uint32) {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	var total, red, green, blue, alpha float64
	for iy := y0 - 1; iy <= y0+2; iy++ {
		wy := catmullRomWeight(y - float64(iy))
		py := iy
		if py < bounds.Min.Y {
			py = bounds.Min.Y
		} else if py >= bounds.Max.Y {
			py = bounds.Max.Y - 1
		}
		for ix := x0 - 1; ix <= x0+2; ix++ {
			weight := wy * catmullRomWeight(x-float64(ix))
			px := ix
			if px < bounds.Min.X {
				px = bounds.Min.X
			} else if px >= bounds.Max.X {
				px = bounds.Max.X - 1
			}
			pr, pg, pb, pa := src.At(px, py).RGBA()
			red += float64(pr) * weight
			green += float64(pg) * weight
			blue += float64(pb) * weight
			alpha += float64(pa) * weight
			total += weight
		}
	}
	if total == 0 {
		return
	}
	return clamp16(red / total), clamp16(green / total), clamp16(blue / total), clamp16(alpha / total)
}

func catmullRomWeight(x float64) float64 {
	x = math.Abs(x)
	if x < 1 {
		return 1.5*x*x*x - 2.5*x*x + 1
	}
	if x < 2 {
		return -0.5*x*x*x + 2.5*x*x - 4*x + 2
	}
	return 0
}

func clamp16(value float64) uint32 {
	if value < 0 {
		return 0
	}
	if value > 65535 {
		return 65535
	}
	return uint32(value + 0.5)
}

func naturalLess(a, b string) bool {
	x, y := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	for i, j := 0, 0; i < len(x) && j < len(y); {
		if x[i] >= '0' && x[i] <= '9' && y[j] >= '0' && y[j] <= '9' {
			xEnd, yEnd := i, j
			for xEnd < len(x) && x[xEnd] >= '0' && x[xEnd] <= '9' {
				xEnd++
			}
			for yEnd < len(y) && y[yEnd] >= '0' && y[yEnd] <= '9' {
				yEnd++
			}
			xSig, ySig := i, j
			for xSig < xEnd-1 && x[xSig] == '0' {
				xSig++
			}
			for ySig < yEnd-1 && y[ySig] == '0' {
				ySig++
			}
			if xEnd-xSig != yEnd-ySig {
				return xEnd-xSig < yEnd-ySig
			}
			for k := 0; k < xEnd-xSig; k++ {
				if x[xSig+k] != y[ySig+k] {
					return x[xSig+k] < y[ySig+k]
				}
			}
			if xEnd-i != yEnd-j {
				return xEnd-i < yEnd-j
			}
			i, j = xEnd, yEnd
			continue
		}
		if x[i] != y[j] {
			return x[i] < y[j]
		}
		i, j = i+1, j+1
	}
	if len(x) != len(y) {
		return len(x) < len(y)
	}
	return a < b
}

func sortEntries(entries []imageEntry, mode int, descending bool) {
	less := func(a, b imageEntry) bool {
		switch mode {
		case sortByType:
			aType, bType := strings.ToLower(filepath.Ext(a.name)), strings.ToLower(filepath.Ext(b.name))
			if aType != bType {
				return aType < bType
			}
		case sortBySize:
			if a.size != b.size {
				return a.size < b.size
			}
		case sortByDate:
			if !a.modTime.Equal(b.modTime) {
				return a.modTime.Before(b.modTime)
			}
		default:
			return naturalLess(a.name, b.name)
		}
		return naturalLess(a.name, b.name)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if descending {
			return less(entries[j], entries[i])
		}
		return less(entries[i], entries[j])
	})
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
