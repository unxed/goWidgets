// Command pixy is the first interactive increment of the goWidgets image viewer.
//
// The browser shell stays constraint-based; Canvas pixels are only used for
// image content, never for application controls or layout.
package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/cocoa"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
	_ "github.com/unxed/goWidgets/backends/qt"
	_ "github.com/unxed/goWidgets/backends/win32"
	imagehistogram "github.com/unxed/goWidgets/showcase/pixy/histogram"
	"github.com/unxed/goWidgets/showcase/pixy/thumbnailgrid"
	"github.com/unxed/goWidgets/showcase/pixy/transform"
	"github.com/unxed/goWidgets/showcase/pixy/viewport"
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
	win, err := app.NewWindow("pixy — image viewer", 1040, 680)
	if err != nil {
		log.Fatal(err)
	}
	// Create the canvas first so native controls added afterward remain above
	// it in Cocoa's subview z-order if their Auto Layout frames touch.
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

	title, _ := win.AddLabel("pixy — просмотр изображений")
	pathEdit, _ := win.AddEdit(start)
	openFolder, _ := win.AddButton("Открыть папку")
	openImage, _ := win.AddButton("Открыть файл…")
	thumbToggle, _ := win.AddButton("Миниатюры")
	histogramButton, _ := win.AddButton("Гистограмма")
	frameLabel, _ := win.AddLabel("Рамка:")
	frameWidthEdit, _ := win.AddEdit("8 #202020")
	frameButton, _ := win.AddButton("Применить")
	rotateButton, _ := win.AddButton("Повернуть 90°")
	undoButton, _ := win.AddButton("Отменить")
	redoButton, _ := win.AddButton("Повторить")
	flipButton, _ := win.AddButton("Горизонтально")
	flipVerticalButton, _ := win.AddButton("Вертикально")
	cropButton, _ := win.AddButton("Кадрировать")
	resizeLabel, _ := win.AddLabel("Размер, px:")
	resizeWidth, _ := win.AddEdit("1")
	resizeTimes, _ := win.AddLabel("×")
	resizeHeight, _ := win.AddEdit("1")
	resizeKeepRatio, _ := win.AddCheckBox("Сохранять пропорции", true)
	resizeButton, _ := win.AddButton("Изменить размер")
	brightnessLabel, _ := win.AddLabel("Яркость, %:")
	brightnessEdit, _ := win.AddEdit("0")
	brightnessButton, _ := win.AddButton("Применить яркость")
	grayscaleButton, _ := win.AddButton("Оттенки серого")
	sepiaButton, _ := win.AddButton("Сепия")
	invertButton, _ := win.AddButton("Инверсия")
	redChannelLabel, _ := win.AddLabel("R, %:")
	redChannelEdit, _ := win.AddEdit("0")
	greenChannelLabel, _ := win.AddLabel("G, %:")
	greenChannelEdit, _ := win.AddEdit("0")
	blueChannelLabel, _ := win.AddLabel("B, %:")
	blueChannelEdit, _ := win.AddEdit("0")
	channelsButton, _ := win.AddButton("Применить RGB")
	redEyeButton, _ := win.AddButton("Красные глаза")
	shadowButton, _ := win.AddButton("Тень")
	sortLabel, _ := win.AddLabel("Сортировка:")
	sortBox, _ := win.AddComboBox([]string{"Имя", "Тип", "Размер", "Дата изменения"}, true)
	sortBox.Select(sortByName)
	sortDirection, _ := win.AddButton("По возрастанию")
	files, _ := win.AddListBox(nil)
	if canvas == nil {
		thumbToggle.Visible.Set(false)
		histogramButton.Visible.Set(false)
		frameLabel.Visible.Set(false)
		frameWidthEdit.Visible.Set(false)
		frameButton.Visible.Set(false)
		rotateButton.Visible.Set(false)
		undoButton.Visible.Set(false)
		redoButton.Visible.Set(false)
		flipButton.Visible.Set(false)
		flipVerticalButton.Visible.Set(false)
		cropButton.Visible.Set(false)
		resizeLabel.Visible.Set(false)
		resizeWidth.Visible.Set(false)
		resizeTimes.Visible.Set(false)
		resizeHeight.Visible.Set(false)
		resizeKeepRatio.Visible.Set(false)
		resizeButton.Visible.Set(false)
		brightnessLabel.Visible.Set(false)
		brightnessEdit.Visible.Set(false)
		brightnessButton.Visible.Set(false)
		grayscaleButton.Visible.Set(false)
		sepiaButton.Visible.Set(false)
		invertButton.Visible.Set(false)
		redChannelLabel.Visible.Set(false)
		redChannelEdit.Visible.Set(false)
		greenChannelLabel.Visible.Set(false)
		greenChannelEdit.Visible.Set(false)
		blueChannelLabel.Visible.Set(false)
		blueChannelEdit.Visible.Set(false)
		channelsButton.Visible.Set(false)
		redEyeButton.Visible.Set(false)
		shadowButton.Visible.Set(false)
	}
	thumbMode := os.Getenv("PIXY_SHOW_THUMBNAILS") == "1" && canvas != nil
	if thumbMode {
		thumbToggle.Text.Set("Просмотр")
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
		thumbToggle.Left().Eq(sortDirection.Right().Plus(pad)),
		thumbToggle.Top().Eq(sortBox.Top()),
		sortDirection.Width().Is(150),
		sortDirection.Height().Is(32),
		thumbToggle.Width().Is(116),
		thumbToggle.Height().Is(32),
		histogramButton.Left().Eq(thumbToggle.Right().Plus(pad)),
		histogramButton.Top().Eq(thumbToggle.Top()),
		histogramButton.Width().Is(130),
		histogramButton.Height().Is(32),
		frameLabel.Left().Eq(histogramButton.Right().Plus(pad)),
		frameLabel.Top().Eq(sortBox.Top()),
		frameWidthEdit.Left().Eq(frameLabel.Right().Plus(6)),
		frameWidthEdit.Top().Eq(sortBox.Top()),
		frameWidthEdit.Width().Is(96),
		frameWidthEdit.Height().Is(32),
		frameButton.Left().Eq(frameWidthEdit.Right().Plus(pad)),
		frameButton.Top().Eq(sortBox.Top()),
		frameButton.Width().Is(100),
		frameButton.Height().Is(32),
		rotateButton.Left().Eq(title.Left()),
		rotateButton.Top().Eq(sortBox.Bottom().Plus(6)),
		rotateButton.Width().Is(142),
		rotateButton.Height().Is(32),
		undoButton.Left().Eq(rotateButton.Right().Plus(6)),
		undoButton.Top().Eq(rotateButton.Top()),
		undoButton.Width().Is(100),
		undoButton.Height().Is(32),
		redoButton.Left().Eq(undoButton.Right().Plus(6)),
		redoButton.Top().Eq(rotateButton.Top()),
		redoButton.Width().Is(105),
		redoButton.Height().Is(32),
		flipButton.Left().Eq(redoButton.Right().Plus(6)),
		flipButton.Top().Eq(rotateButton.Top()),
		flipButton.Width().Is(150),
		flipButton.Height().Is(32),
		flipVerticalButton.Left().Eq(flipButton.Right().Plus(6)),
		flipVerticalButton.Top().Eq(rotateButton.Top()),
		flipVerticalButton.Width().Is(140),
		flipVerticalButton.Height().Is(32),
		cropButton.Left().Eq(flipVerticalButton.Right().Plus(6)),
		cropButton.Top().Eq(rotateButton.Top()),
		cropButton.Width().Is(235),
		cropButton.Height().Is(32),
		resizeLabel.Left().Eq(title.Left()),
		resizeLabel.Top().Eq(cropButton.Bottom().Plus(6)),
		resizeWidth.Left().Eq(resizeLabel.Right().Plus(6)),
		resizeWidth.Top().Eq(resizeLabel.Top()),
		resizeWidth.Width().Is(82),
		resizeWidth.Height().Is(32),
		resizeTimes.Left().Eq(resizeWidth.Right().Plus(6)),
		resizeTimes.Top().Eq(resizeWidth.Top()),
		resizeHeight.Left().Eq(resizeTimes.Right().Plus(6)),
		resizeHeight.Top().Eq(resizeWidth.Top()),
		resizeHeight.Width().Is(82),
		resizeHeight.Height().Is(32),
		resizeKeepRatio.Left().Eq(resizeHeight.Right().Plus(pad)),
		resizeKeepRatio.Top().Eq(resizeWidth.Top()),
		resizeKeepRatio.Width().Is(190),
		resizeKeepRatio.Height().Is(32),
		resizeButton.Left().Eq(resizeKeepRatio.Right().Plus(pad)),
		resizeButton.Top().Eq(resizeWidth.Top()),
		resizeButton.Width().Is(155),
		resizeButton.Height().Is(32),
		brightnessLabel.Left().Eq(title.Left()),
		brightnessLabel.Top().Eq(resizeButton.Bottom().Plus(6)),
		brightnessEdit.Left().Eq(brightnessLabel.Right().Plus(6)),
		brightnessEdit.Top().Eq(brightnessLabel.Top()),
		brightnessEdit.Width().Is(82),
		brightnessEdit.Height().Is(32),
		brightnessButton.Left().Eq(brightnessEdit.Right().Plus(pad)),
		brightnessButton.Top().Eq(brightnessEdit.Top()),
		brightnessButton.Width().Is(175),
		brightnessButton.Height().Is(32),
		grayscaleButton.Left().Eq(brightnessButton.Right().Plus(pad)),
		grayscaleButton.Top().Eq(brightnessButton.Top()),
		grayscaleButton.Width().Is(150),
		grayscaleButton.Height().Is(32),
		sepiaButton.Left().Eq(grayscaleButton.Right().Plus(pad)),
		sepiaButton.Top().Eq(grayscaleButton.Top()),
		sepiaButton.Width().Is(105),
		sepiaButton.Height().Is(32),
		invertButton.Left().Eq(sepiaButton.Right().Plus(pad)),
		invertButton.Top().Eq(sepiaButton.Top()),
		invertButton.Width().Is(110),
		invertButton.Height().Is(32),
		redChannelLabel.Left().Eq(title.Left()),
		redChannelLabel.Top().Eq(invertButton.Bottom().Plus(6)),
		redChannelEdit.Left().Eq(redChannelLabel.Right().Plus(6)),
		redChannelEdit.Top().Eq(redChannelLabel.Top()),
		redChannelEdit.Width().Is(68),
		redChannelEdit.Height().Is(32),
		greenChannelLabel.Left().Eq(redChannelEdit.Right().Plus(pad)),
		greenChannelLabel.Top().Eq(redChannelLabel.Top()),
		greenChannelEdit.Left().Eq(greenChannelLabel.Right().Plus(6)),
		greenChannelEdit.Top().Eq(greenChannelLabel.Top()),
		greenChannelEdit.Width().Is(68),
		greenChannelEdit.Height().Is(32),
		blueChannelLabel.Left().Eq(greenChannelEdit.Right().Plus(pad)),
		blueChannelLabel.Top().Eq(redChannelLabel.Top()),
		blueChannelEdit.Left().Eq(blueChannelLabel.Right().Plus(6)),
		blueChannelEdit.Top().Eq(blueChannelLabel.Top()),
		blueChannelEdit.Width().Is(68),
		blueChannelEdit.Height().Is(32),
		channelsButton.Left().Eq(blueChannelEdit.Right().Plus(pad)),
		channelsButton.Top().Eq(blueChannelEdit.Top()),
		channelsButton.Width().Is(145),
		channelsButton.Height().Is(32),
		redEyeButton.Left().Eq(channelsButton.Right().Plus(pad)),
		redEyeButton.Top().Eq(channelsButton.Top()),
		redEyeButton.Width().Is(150),
		redEyeButton.Height().Is(32),
		shadowButton.Left().Eq(redEyeButton.Right().Plus(6)),
		shadowButton.Top().Eq(redEyeButton.Top()),
		shadowButton.Width().Is(90),
		shadowButton.Height().Is(32),

		files.Left().Eq(title.Left()),
		files.Top().Eq(channelsButton.Bottom().Plus(pad)),
		files.Width().Is(280),
		files.Bottom().Eq(info.Top().Minus(pad)),

		previewBox.Left().Eq(files.Right().Plus(pad)),
		previewBox.Top().Eq(files.Top()),
		previewBox.Right().Eq(title.Right()),
		previewBox.Bottom().Eq(info.Top().Minus(pad)),
		info.Left().Eq(previewBox.Left()),
		info.Bottom().Eq(status.Top().Minus(pad)),

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
	frameLabel.HugHeight()
	resizeLabel.HugHeight()
	resizeTimes.HugHeight()
	brightnessLabel.HugHeight()
	redChannelLabel.HugHeight()
	greenChannelLabel.HugHeight()
	blueChannelLabel.HugHeight()
	sortBox.HugHeight()
	title.HugHeight()
	status.HugHeight()

	var entries []imageEntry
	var currentImage image.Image
	var histogramMode bool
	var currentHistogram *imagehistogram.Distribution
	resizeWidthEdited := true
	syncingResizeFields := false
	var undoStack, redoStack []image.Image
	var cropMode, cropSelecting, cropValid bool
	var redEyeMode bool
	var cropStart, cropEnd viewport.Point
	var folder string
	selectedIndex := -1
	thumbGrid := thumbnailgrid.Grid{CellWidth: 144, CellHeight: 112, Gap: 10}
	var thumbGeneration atomic.Uint64
	var thumbMu sync.Mutex
	thumbs := map[int]*image.RGBA{}
	thumbLoading := map[int]bool{}
	var requestThumbnails func(int, int)
	var selectEntry func(int)
	resetThumbnails := func() {
		thumbGeneration.Add(1)
		thumbMu.Lock()
		thumbs = map[int]*image.RGBA{}
		thumbLoading = map[int]bool{}
		thumbMu.Unlock()
		thumbGrid.SetCount(len(entries))
		thumbGrid.Scroll = 0
		clear(thumbGrid.Selected)
		if canvas != nil {
			canvas.Invalidate()
		}
	}
	requestThumbnails = func(first, end int) {
		generation := thumbGeneration.Load()
		for i := first; i < end; i++ {
			thumbMu.Lock()
			if thumbs[i] != nil || thumbLoading[i] || i >= len(entries) {
				thumbMu.Unlock()
				continue
			}
			thumbLoading[i] = true
			path := entries[i].path
			thumbMu.Unlock()
			go func(index int, file string) {
				var thumb *image.RGBA
				if decoded, _, err := decodeImage(file); err == nil {
					thumb = thumbnailgrid.FitThumbnail(decoded, 128, 84)
				}
				app.QueueUpdate(func() {
					thumbMu.Lock()
					delete(thumbLoading, index)
					if generation == thumbGeneration.Load() && thumb != nil {
						thumbs[index] = thumb
					}
					thumbMu.Unlock()
					if generation == thumbGeneration.Load() && canvas != nil {
						canvas.Invalidate()
					}
				})
			}(i, path)
		}
	}
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
	updateHistoryButtons := func() {
		undoButton.Enabled.Set(len(undoStack) > 0)
		redoButton.Enabled.Set(len(redoStack) > 0)
	}
	undoButton.Enabled.Set(false)
	redoButton.Enabled.Set(false)
	refreshCurrentImage := func(modified bool) {
		if currentImage == nil || selectedIndex < 0 || selectedIndex >= len(entries) {
			return
		}
		mode := viewState.Mode
		viewState = viewport.New(viewport.Size{W: float64(currentImage.Bounds().Dx()), H: float64(currentImage.Bounds().Dy())}, viewState.Area)
		viewState.Mode = mode
		invalidateView()
		entry := entries[selectedIndex]
		bounds := currentImage.Bounds()
		syncingResizeFields = true
		resizeWidth.Text.Set(strconv.Itoa(bounds.Dx()))
		resizeHeight.Text.Set(strconv.Itoa(bounds.Dy()))
		syncingResizeFields = false
		info.Text.Set(fmt.Sprintf("%s  •  %s  •  %d × %d", entry.name, formatBytes(entry.size), bounds.Dx(), bounds.Dy()))
		if histogramMode {
			currentHistogram = imagehistogram.Count(currentImage)
			if currentHistogram != nil {
				info.Text.Set(fmt.Sprintf("Гистограмма RGB · %d видимых пикселей", currentHistogram.Pixels))
			}
		}
		if modified {
			status.Text.Set("Изменено в памяти: " + entry.path)
		} else {
			status.Text.Set("Выбрано: " + entry.path)
		}
	}
	applyEdit := func(next image.Image) {
		if currentImage == nil || next == nil {
			return
		}
		undoStack = append(undoStack, currentImage)
		currentImage = next
		redoStack = nil
		cropMode, cropSelecting, cropValid = false, false, false
		cropButton.Text.Set("Кадрировать")
		refreshCurrentImage(true)
		updateHistoryButtons()
	}
	selectedCrop := func() (image.Rectangle, bool) {
		if currentImage == nil || !cropValid {
			return image.Rectangle{}, false
		}
		p0, p1 := viewState.ImagePoint(cropStart), viewState.ImagePoint(cropEnd)
		bounds := currentImage.Bounds()
		x0 := max(0, int(math.Floor(math.Min(p0.X, p1.X))))
		y0 := max(0, int(math.Floor(math.Min(p0.Y, p1.Y))))
		x1 := min(bounds.Dx(), int(math.Ceil(math.Max(p0.X, p1.X))))
		y1 := min(bounds.Dy(), int(math.Ceil(math.Max(p0.Y, p1.Y))))
		if x1 <= x0 || y1 <= y0 {
			return image.Rectangle{}, false
		}
		return image.Rect(bounds.Min.X+x0, bounds.Min.Y+y0, bounds.Min.X+x1, bounds.Min.Y+y1), true
	}
	applyCrop := func() {
		rect, ok := selectedCrop()
		if !ok {
			status.Text.Set("Сначала выделите область кадрирования")
			return
		}
		applyEdit(transform.Crop(currentImage, rect))
		cropMode, cropSelecting, cropValid = false, false, false
		cropButton.Text.Set("Кадрировать")
		status.Text.Set("Кадрировано в памяти: " + entries[selectedIndex].path)
		canvas.Invalidate()
	}
	doUndo := func() {
		if len(undoStack) == 0 {
			return
		}
		redoStack = append(redoStack, currentImage)
		currentImage = undoStack[len(undoStack)-1]
		undoStack = undoStack[:len(undoStack)-1]
		refreshCurrentImage(len(undoStack) > 0)
		updateHistoryButtons()
	}
	doRedo := func() {
		if len(redoStack) == 0 {
			return
		}
		undoStack = append(undoStack, currentImage)
		currentImage = redoStack[len(redoStack)-1]
		redoStack = redoStack[:len(redoStack)-1]
		refreshCurrentImage(true)
		updateHistoryButtons()
	}
	if canvas != nil {
		canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
			if histogramMode {
				imagehistogram.Paint(frame.Image, currentHistogram)
				return
			}
			if thumbMode {
				scale := frame.Scale
				if scale <= 0 {
					scale = 1
				}
				thumbGrid.Width = float64(frame.Image.Bounds().Dx()) / scale
				thumbGrid.Height = float64(frame.Image.Bounds().Dy()) / scale
				thumbGrid.ClampScroll()
				first, end := thumbGrid.VisibleRange()
				thumbMu.Lock()
				visible := make(map[int]*image.RGBA, end-first)
				for i := first; i < end; i++ {
					if thumbs[i] != nil {
						visible[i] = thumbs[i]
					}
				}
				thumbMu.Unlock()
				thumbnailgrid.Paint(frame.Image, &thumbGrid, visible, scale)
				requestThumbnails(first, end)
				return
			}
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
			} else {
				paintViewport(frame.Image, currentImage, viewState)
				if scheduleQuality != nil {
					scheduleQuality(frame.Image)
				}
			}
			if cropMode && cropValid {
				drawCropOutline(frame.Image, cropStart, cropEnd)
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
		currentImage = nil
		undoStack, redoStack = nil, nil
		cropMode, cropSelecting, cropValid = false, false, false
		redEyeMode = false
		histogramMode, currentHistogram = false, nil
		cropButton.Text.Set("Кадрировать")
		redEyeButton.Text.Set("Красные глаза")
		histogramButton.Text.Set("Гистограмма")
		updateHistoryButtons()
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
		resetThumbnails()
		folder = dir
		labels := make([]string, len(entries))
		for i := range entries {
			labels[i] = entries[i].name
		}
		files.SetItems(labels)
		files.Select(-1)
		pathEdit.Text.Set(folder)
		info.Text.Set(fmt.Sprintf("Файлов: %d", len(entries)))
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
	selectEntry = func(i int) {
		if i < 0 || i >= len(entries) {
			return
		}
		selectedIndex = i
		e := entries[i]
		if canvas != nil {
			cropMode, cropSelecting, cropValid = false, false, false
			redEyeMode = false
			histogramMode, currentHistogram = false, nil
			cropButton.Text.Set("Кадрировать")
			redEyeButton.Text.Set("Красные глаза")
			histogramButton.Text.Set("Гистограмма")
			undoStack, redoStack = nil, nil
			updateHistoryButtons()
			img, _, err := decodeImage(e.path)
			if err != nil {
				currentImage = nil
				invalidateView()
				status.Text.Set("Не удалось декодировать изображение: " + err.Error())
				return
			}
			currentImage = img
			refreshCurrentImage(false)
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
				thumbGrid.Select(i, false, false)
				files.Select(i)
				selectEntry(i)
				if canvas != nil {
					canvas.Invalidate()
				}
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
		thumbGrid.Select(index, false, false)
		if canvas != nil {
			canvas.Invalidate()
		}
		selectEntry(index)
	}
	applySorting := func() {
		selectedPath := ""
		if selectedIndex >= 0 && selectedIndex < len(entries) {
			selectedPath = entries[selectedIndex].path
		}
		sortEntries(entries, sortMode, sortDescending)
		resetThumbnails()
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
			thumbGrid.Select(selectedIndex, false, false)
			selectEntry(selectedIndex)
			if canvas != nil {
				canvas.Invalidate()
			}
		}
	}

	files.Selected.On(app.Scope(), func(index int) {
		thumbGrid.Select(index, false, false)
		if canvas != nil {
			canvas.Invalidate()
		}
		selectEntry(index)
	})
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
	thumbToggle.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil {
			return
		}
		thumbMode = !thumbMode
		if thumbMode {
			histogramMode, currentHistogram = false, nil
			histogramButton.Text.Set("Гистограмма")
			thumbToggle.Text.Set("Просмотр")
		} else {
			thumbToggle.Text.Set("Миниатюры")
		}
		canvas.Invalidate()
	})
	histogramButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		if histogramMode {
			histogramMode, currentHistogram = false, nil
			histogramButton.Text.Set("Гистограмма")
			refreshCurrentImage(len(undoStack) > 0)
			return
		}
		if thumbMode {
			thumbMode = false
			thumbToggle.Text.Set("Миниатюры")
		}
		currentHistogram = imagehistogram.Count(currentImage)
		if currentHistogram == nil {
			status.Text.Set("Не удалось построить гистограмму")
			return
		}
		histogramMode = true
		histogramButton.Text.Set("Изображение")
		info.Text.Set(fmt.Sprintf("Гистограмма RGB · %d видимых пикселей", currentHistogram.Pixels))
		status.Text.Set(currentHistogram.Summary())
		canvas.Invalidate()
	})
	rotateButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas != nil && currentImage != nil {
			applyEdit(transform.RotateClockwise(currentImage))
		}
	})
	undoButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { doUndo() })
	redoButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { doRedo() })
	flipButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas != nil && currentImage != nil {
			applyEdit(transform.MirrorHorizontal(currentImage))
			status.Text.Set("Отражено по горизонтали в памяти: " + entries[selectedIndex].path)
		}
	})
	flipVerticalButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas != nil && currentImage != nil {
			applyEdit(transform.MirrorVertical(currentImage))
			status.Text.Set("Отражено по вертикали в памяти: " + entries[selectedIndex].path)
		}
	})
	cropButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		if cropMode {
			applyCrop()
			return
		}
		redEyeMode = false
		redEyeButton.Text.Set("Красные глаза")
		cropMode, cropSelecting, cropValid = true, false, false
		cropButton.Text.Set("Применить кадрирование")
		status.Text.Set("Выделите область мышью, затем нажмите «Применить кадрирование» или Enter")
		canvas.Invalidate()
	})
	redEyeButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		redEyeMode = !redEyeMode
		if redEyeMode {
			if cropMode {
				cropMode, cropSelecting, cropValid = false, false, false
				cropButton.Text.Set("Кадрировать")
			}
			redEyeButton.Text.Set("Кликните глаз")
			status.Text.Set("Щёлкните по красному зрачку; Esc завершит выбор")
		} else {
			redEyeButton.Text.Set("Красные глаза")
			status.Text.Set("Коррекция красных глаз завершена")
		}
		canvas.Invalidate()
	})
	resizeWidth.Changed.On(app.Scope(), func(string) {
		if !syncingResizeFields {
			resizeWidthEdited = true
		}
	})
	resizeHeight.Changed.On(app.Scope(), func(string) {
		if !syncingResizeFields {
			resizeWidthEdited = false
		}
	})
	applyResize := func() {
		if canvas == nil || currentImage == nil {
			return
		}
		width, widthErr := strconv.Atoi(strings.TrimSpace(resizeWidth.Text.Get()))
		height, heightErr := strconv.Atoi(strings.TrimSpace(resizeHeight.Text.Get()))
		if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			status.Text.Set("Введите положительные размеры изображения")
			return
		}
		if width > 100_000_000 || height > 100_000_000 {
			status.Text.Set("Размер слишком велик (максимум 100 млн пикселей)")
			return
		}
		if resizeKeepRatio.Checked.Get() {
			bounds := currentImage.Bounds()
			if resizeWidthEdited {
				scaled := math.Round(float64(width) * float64(bounds.Dy()) / float64(bounds.Dx()))
				if scaled > 100_000_000 {
					status.Text.Set("Размер слишком велик (максимум 100 млн пикселей)")
					return
				}
				height = max(1, int(scaled))
			} else {
				scaled := math.Round(float64(height) * float64(bounds.Dx()) / float64(bounds.Dy()))
				if scaled > 100_000_000 {
					status.Text.Set("Размер слишком велик (максимум 100 млн пикселей)")
					return
				}
				width = max(1, int(scaled))
			}
		}
		resized := transform.Resize(currentImage, width, height)
		if resized == nil {
			status.Text.Set("Размер слишком велик (максимум 100 млн пикселей)")
			return
		}
		applyEdit(resized)
		status.Text.Set(fmt.Sprintf("Изменено в памяти: %d × %d — %s", width, height, entries[selectedIndex].path))
	}
	resizeButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { applyResize() })
	resizeWidth.Activated.On(app.Scope(), func(string) { applyResize() })
	resizeHeight.Activated.On(app.Scope(), func(string) { applyResize() })
	applyBrightness := func() {
		if canvas == nil || currentImage == nil {
			return
		}
		percent, err := strconv.Atoi(strings.TrimSpace(brightnessEdit.Text.Get()))
		if err != nil || percent < -100 || percent > 100 {
			status.Text.Set("Введите яркость от −100 до 100 процентов")
			return
		}
		if percent == 0 {
			status.Text.Set("Яркость не изменена")
			return
		}
		adjusted := transform.AdjustBrightness(currentImage, percent)
		if adjusted == nil {
			status.Text.Set("Не удалось скорректировать яркость")
			return
		}
		applyEdit(adjusted)
		status.Text.Set(fmt.Sprintf("Яркость %+d%% применена в памяти: %s", percent, entries[selectedIndex].path))
	}
	brightnessButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { applyBrightness() })
	brightnessEdit.Activated.On(app.Scope(), func(string) { applyBrightness() })
	applyFrame := func() {
		if canvas == nil || currentImage == nil {
			return
		}
		width, border, err := parseFrameSettings(frameWidthEdit.Text.Get())
		if err != nil {
			status.Text.Set("Задайте рамку как: ширина #RRGGBB, например 8 #202020")
			return
		}
		framed := transform.AddFrame(currentImage, width, border)
		if framed == nil {
			status.Text.Set("Не удалось добавить рамку: слишком большое изображение")
			return
		}
		applyEdit(framed)
		status.Text.Set(fmt.Sprintf("Рамка %d px #%02X%02X%02X применена в памяти: %s", width, border.R, border.G, border.B, entries[selectedIndex].path))
	}
	frameButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { applyFrame() })
	frameWidthEdit.Activated.On(app.Scope(), func(string) { applyFrame() })
	shadowButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		shadowed := transform.DropShadow(currentImage, 8, 5, color.NRGBA{A: 160})
		if shadowed == nil {
			status.Text.Set("Не удалось добавить тень: слишком большое изображение")
			return
		}
		applyEdit(shadowed)
		status.Text.Set("Тень добавлена в памяти: " + entries[selectedIndex].path)
	})
	grayscaleButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		applyEdit(transform.Grayscale(currentImage))
		status.Text.Set("Переведено в оттенки серого в памяти: " + entries[selectedIndex].path)
	})
	sepiaButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		applyEdit(transform.Sepia(currentImage))
		status.Text.Set("Применена сепия в памяти: " + entries[selectedIndex].path)
	})
	invertButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		if canvas == nil || currentImage == nil {
			return
		}
		applyEdit(transform.Invert(currentImage))
		status.Text.Set("Цвета инвертированы в памяти: " + entries[selectedIndex].path)
	})
	applyChannels := func() {
		if canvas == nil || currentImage == nil {
			return
		}
		values := [3]int{}
		for i, edit := range []*goWidgets.Edit{redChannelEdit, greenChannelEdit, blueChannelEdit} {
			value, err := strconv.Atoi(strings.TrimSpace(edit.Text.Get()))
			if err != nil || value < -100 || value > 100 {
				status.Text.Set("Введите коррекцию каждого RGB-канала от −100 до 100 процентов")
				return
			}
			values[i] = value
		}
		if values == [3]int{} {
			status.Text.Set("RGB-каналы не изменены")
			return
		}
		adjusted := transform.AdjustChannels(currentImage, values[0], values[1], values[2])
		if adjusted == nil {
			status.Text.Set("Не удалось скорректировать RGB-каналы")
			return
		}
		applyEdit(adjusted)
		status.Text.Set(fmt.Sprintf("Коррекция RGB %+d%% / %+d%% / %+d%% применена в памяти: %s", values[0], values[1], values[2], entries[selectedIndex].path))
	}
	channelsButton.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { applyChannels() })
	redChannelEdit.Activated.On(app.Scope(), func(string) { applyChannels() })
	greenChannelEdit.Activated.On(app.Scope(), func(string) { applyChannels() })
	blueChannelEdit.Activated.On(app.Scope(), func(string) { applyChannels() })
	if canvas != nil {
		var dragging bool
		var lastPointer goWidgets.MouseInfo
		canvas.MouseWheel.On(app.Scope(), func(mouse goWidgets.MouseInfo) {
			if thumbMode {
				thumbGrid.Scroll -= mouse.Delta * (thumbGrid.CellHeight + thumbGrid.Gap) * 3
				thumbGrid.ClampScroll()
				canvas.Invalidate()
				return
			}
			if histogramMode {
				return
			}
			if mouse.Mods&goWidgets.ModControl == 0 {
				if step := wheelNavigationDelta(mouse.Delta); step != 0 {
					navigate(selectedIndex + step)
				}
				return
			}
			if currentImage == nil {
				return
			}
			if cropMode {
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
			if thumbMode {
				if mouse.Button != goWidgets.MouseLeft {
					return
				}
				index, ok := thumbGrid.HitTest(mouse.X, mouse.Y)
				if !ok {
					return
				}
				thumbGrid.Select(index, mouse.Mods&goWidgets.ModControl != 0, mouse.Mods&goWidgets.ModShift != 0)
				files.Select(index)
				selectEntry(index)
				canvas.Invalidate()
				return
			}
			if histogramMode {
				return
			}
			if redEyeMode && mouse.Button == goWidgets.MouseLeft && currentImage != nil {
				scale := win.Scale().Scale
				if scale <= 0 {
					scale = 1
				}
				imagePoint := viewState.ImagePoint(viewport.Point{X: mouse.X * scale, Y: mouse.Y * scale})
				if imagePoint.X < 0 || imagePoint.Y < 0 || imagePoint.X >= float64(currentImage.Bounds().Dx()) || imagePoint.Y >= float64(currentImage.Bounds().Dy()) {
					status.Text.Set("Щёлкните по изображению внутри красного зрачка")
					return
				}
				radius := 10
				if viewScale := viewState.Transform().Scale; viewScale > 0 {
					radius = int(math.Round(10 / viewScale))
				}
				radius = max(2, min(64, radius))
				adjusted := transform.ReduceRedEye(currentImage, image.Pt(int(math.Floor(imagePoint.X)), int(math.Floor(imagePoint.Y))), radius)
				if adjusted == nil {
					status.Text.Set("Не удалось скорректировать красный глаз")
					return
				}
				applyEdit(adjusted)
				status.Text.Set("Коррекция красного глаза применена в памяти; щёлкните по следующему или нажмите Esc")
				return
			}
			if cropMode && mouse.Button == goWidgets.MouseLeft && currentImage != nil {
				scale := win.Scale().Scale
				if scale <= 0 {
					scale = 1
				}
				cropStart = viewport.Point{X: mouse.X * scale, Y: mouse.Y * scale}
				cropEnd = cropStart
				cropSelecting, cropValid = true, true
				canvas.Invalidate()
				return
			}
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
			if cropSelecting {
				scale := win.Scale().Scale
				if scale <= 0 {
					scale = 1
				}
				cropEnd = viewport.Point{X: mouse.X * scale, Y: mouse.Y * scale}
				canvas.Invalidate()
				return
			}
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
				if cropSelecting {
					scale := win.Scale().Scale
					if scale <= 0 {
						scale = 1
					}
					cropEnd = viewport.Point{X: mouse.X * scale, Y: mouse.Y * scale}
					cropSelecting = false
					canvas.Invalidate()
					if _, ok := selectedCrop(); ok {
						status.Text.Set("Область выбрана. Нажмите «Применить кадрирование» или Enter")
					}
					return
				}
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
		} else if key.Ctrl() && key.VirtualKeyCode == 0x5A { // Ctrl+Z
			doUndo()
		} else if key.Ctrl() && key.VirtualKeyCode == 0x59 { // Ctrl+Y
			doRedo()
		} else if key.VirtualKeyCode == 0x1B { // Escape
			if cropMode {
				cropMode, cropSelecting, cropValid = false, false, false
				cropButton.Text.Set("Кадрировать")
				status.Text.Set("Кадрирование отменено")
				canvas.Invalidate()
			} else if redEyeMode {
				redEyeMode = false
				redEyeButton.Text.Set("Красные глаза")
				status.Text.Set("Выбор области красного глаза завершён")
				canvas.Invalidate()
			} else if histogramMode {
				histogramMode, currentHistogram = false, nil
				histogramButton.Text.Set("Гистограмма")
				refreshCurrentImage(len(undoStack) > 0)
			} else {
				app.Quit()
			}
		} else if cropMode && key.VirtualKeyCode == 0x0D { // Enter
			applyCrop()
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

func drawCropOutline(dst *image.RGBA, start, end viewport.Point) {
	if dst == nil {
		return
	}
	x0 := max(dst.Bounds().Min.X, int(math.Floor(math.Min(start.X, end.X))))
	y0 := max(dst.Bounds().Min.Y, int(math.Floor(math.Min(start.Y, end.Y))))
	x1 := min(dst.Bounds().Max.X, int(math.Ceil(math.Max(start.X, end.X))))
	y1 := min(dst.Bounds().Max.Y, int(math.Ceil(math.Max(start.Y, end.Y))))
	if x1 <= x0 || y1 <= y0 {
		return
	}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.RGBA{A: 255}
	for n := 0; n < 2; n++ {
		c := black
		if n == 1 {
			c = white
		}
		left, right := x0+n, x1-1-n
		top, bottom := y0+n, y1-1-n
		if left > right || top > bottom {
			break
		}
		for x := left; x <= right; x++ {
			dst.SetRGBA(x, top, c)
			dst.SetRGBA(x, bottom, c)
		}
		for y := top; y <= bottom; y++ {
			dst.SetRGBA(left, y, c)
			dst.SetRGBA(right, y, c)
		}
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
