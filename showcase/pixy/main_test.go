package main

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/showcase/pixy/export"
	"github.com/unxed/goWidgets/showcase/pixy/viewport"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func TestFitImageCentersAndPreservesAspect(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	src.SetRGBA(1, 0, color.RGBA{B: 255, A: 255})
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fitImage(dst, src)

	if got := dst.RGBAAt(0, 0); got != (color.RGBA{R: 248, G: 248, B: 248, A: 255}) {
		t.Fatalf("letterbox pixel = %#v, want opaque neutral background", got)
	}
	wantRow := []color.RGBA{
		{R: 255, A: 255}, {R: 255, A: 255}, {B: 255, A: 255}, {B: 255, A: 255},
	}
	for x, want := range wantRow {
		if got := dst.RGBAAt(x, 1); got != want {
			t.Errorf("preview pixel at (%d,1) = %#v, want %#v", x, got, want)
		}
	}
}

func TestFitImageCompositesTransparency(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	src.SetRGBA(0, 0, color.RGBA{R: 128, A: 128}) // premultiplied half-red
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fitImage(dst, src)
	if got, want := dst.RGBAAt(0, 0), (color.RGBA{R: 251, G: 123, B: 123, A: 255}); got != want {
		t.Fatalf("transparent pixel composited as %#v, want %#v", got, want)
	}
}

func TestQualityPreviewPreservesSolidPixelsAndLetterbox(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{R: 220, G: 40, B: 30, A: 255})
	src.SetRGBA(1, 0, color.RGBA{R: 220, G: 40, B: 30, A: 255})
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	view := viewport.New(viewport.Size{W: 2, H: 1}, viewport.Size{W: 4, H: 4})
	paintViewportQuality(dst, src, view, nil)
	if got, want := dst.RGBAAt(0, 0), (color.RGBA{R: 248, G: 248, B: 248, A: 255}); got != want {
		t.Fatalf("quality letterbox = %#v, want %#v", got, want)
	}
	if got, want := dst.RGBAAt(1, 1), (color.RGBA{R: 220, G: 40, B: 30, A: 255}); got != want {
		t.Fatalf("quality preview = %#v, want %#v", got, want)
	}
}

func TestQualityPreviewStopsWhenSuperseded(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	dst := image.NewRGBA(image.Rect(0, 0, 20, 20))
	view := viewport.New(viewport.Size{W: 10, H: 10}, viewport.Size{W: 20, H: 20})
	calls := 0
	paintViewportQuality(dst, src, view, func() bool {
		calls++
		return false
	})
	if calls != 1 {
		t.Fatalf("cancellation checks = %d, want 1", calls)
	}
}

func TestDecodeImagePNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 17, 9))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	img, dimensions, err := decodeImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := img.Bounds().Size(), image.Pt(17, 9); got != want {
		t.Fatalf("decoded size = %v, want %v", got, want)
	}
	if dimensions != "17 × 9" {
		t.Fatalf("dimensions = %q, want %q", dimensions, "17 × 9")
	}
}

func TestDecodeImageBMPAndTIFF(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 17, 9))
	source.SetRGBA(0, 0, color.RGBA{R: 200, G: 80, B: 30, A: 255})
	encoders := []struct {
		name   string
		encode func(io.Writer, image.Image) error
	}{
		{name: "sample.bmp", encode: bmp.Encode},
		{name: "sample.tiff", encode: func(w io.Writer, img image.Image) error { return tiff.Encode(w, img, nil) }},
	}
	for _, tc := range encoders {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.name)
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.encode(f, source); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			decoded, dimensions, err := decodeImage(path)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := decoded.Bounds().Size(), image.Pt(17, 9); got != want {
				t.Fatalf("decoded size = %v, want %v", got, want)
			}
			if dimensions != "17 × 9" {
				t.Fatalf("dimensions = %q, want %q", dimensions, "17 × 9")
			}
		})
	}
}

func TestNaturalLessSortsNumberedImages(t *testing.T) {
	names := []string{"scan10.png", "scan2.png", "scan01.png", "scan1.png"}
	sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
	want := []string{"scan1.png", "scan01.png", "scan2.png", "scan10.png"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("natural order = %v, want %v", names, want)
		}
	}
}

func TestWheelNavigatesInScrollDirection(t *testing.T) {
	for _, tt := range []struct {
		delta float64
		want  int
	}{{delta: 1, want: -1}, {delta: -1, want: 1}, {delta: 0, want: 0}} {
		if got := wheelNavigationDelta(tt.delta); got != tt.want {
			t.Errorf("wheelNavigationDelta(%v) = %d, want %d", tt.delta, got, tt.want)
		}
	}
}

func TestKeyboardNavigationMatchesViewerShortcuts(t *testing.T) {
	for _, tt := range []struct {
		key  uint16
		want int
	}{{key: 0x08, want: -1}, {key: 0x25, want: -1}, {key: 0x21, want: -1},
		{key: 0x20, want: 1}, {key: 0x27, want: 1}, {key: 0x22, want: 1}, {key: 0x24, want: 0}} {
		if got := keyNavigationDelta(tt.key); got != tt.want {
			t.Errorf("keyNavigationDelta(%#x) = %d, want %d", tt.key, got, tt.want)
		}
	}
}

func TestArrowKeysPanOnlyWhenImageOverflows(t *testing.T) {
	fit := viewport.New(viewport.Size{W: 100, H: 100}, viewport.Size{W: 200, H: 200})
	if panByArrow(&fit, 0x25) {
		t.Fatal("left arrow panned a fitted image instead of remaining available for navigation")
	}
	if fit.Pan != (viewport.Point{}) {
		t.Fatalf("fitted view pan = %+v, want unchanged", fit.Pan)
	}

	zoomed := viewport.New(viewport.Size{W: 400, H: 300}, viewport.Size{W: 100, H: 100})
	zoomed.SetMode(viewport.Actual)
	for _, tc := range []struct {
		key uint16
		pan viewport.Point
	}{
		{0x25, viewport.Point{X: keyboardPanStep}},
		{0x26, viewport.Point{Y: keyboardPanStep}},
		{0x27, viewport.Point{X: -keyboardPanStep}},
		{0x28, viewport.Point{Y: -keyboardPanStep}},
	} {
		zoomed.Pan = viewport.Point{}
		if !panByArrow(&zoomed, tc.key) {
			t.Errorf("arrow %#x did not pan overflowing image", tc.key)
			continue
		}
		if zoomed.Pan != tc.pan {
			t.Errorf("arrow %#x pan = %+v, want %+v", tc.key, zoomed.Pan, tc.pan)
		}
	}
	if panByArrow(&zoomed, 0x21) {
		t.Fatal("Page Up was consumed as a pan key")
	}
}

func TestMousePanStartsOnlyForLeftButtonWhenImageOverflows(t *testing.T) {
	fitted := viewport.New(viewport.Size{W: 100, H: 100}, viewport.Size{W: 200, H: 200})
	overflowing := viewport.New(viewport.Size{W: 400, H: 300}, viewport.Size{W: 100, H: 100})
	overflowing.SetMode(viewport.Actual)

	for _, tc := range []struct {
		name  string
		view  viewport.Viewport
		mouse goWidgets.MouseInfo
		want  bool
	}{
		{name: "fitted image", view: fitted, mouse: goWidgets.MouseInfo{Button: goWidgets.MouseLeft}},
		{name: "left button pans overflowing image", view: overflowing, mouse: goWidgets.MouseInfo{Button: goWidgets.MouseLeft}, want: true},
		{name: "other button does not pan", view: overflowing, mouse: goWidgets.MouseInfo{Button: goWidgets.MouseRight}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldStartMousePan(tc.view, tc.mouse); got != tc.want {
				t.Errorf("shouldStartMousePan() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestZoomShortcutsMatchViewerControls(t *testing.T) {
	for _, tt := range []struct {
		key  rune
		mode viewport.Mode
		zoom float64
	}{
		{key: '1', mode: viewport.Actual, zoom: 1},
		{key: '9', mode: viewport.Actual, zoom: 9},
		{key: 'A', mode: viewport.Actual, zoom: 1},
		{key: '/', mode: viewport.Actual, zoom: 1},
		{key: 'B', mode: viewport.Fit, zoom: 1},
		{key: '*', mode: viewport.Fit, zoom: 1},
		{key: 'F', mode: viewport.Fit, zoom: 1},
	} {
		mode, zoom, ok := zoomShortcut(tt.key)
		if !ok || mode != tt.mode || zoom != tt.zoom {
			t.Errorf("zoomShortcut(%q) = (%v, %v, %v), want (%v, %v, true)", tt.key, mode, zoom, ok, tt.mode, tt.zoom)
		}
	}
	for _, tt := range []struct {
		key  rune
		want float64
	}{{key: '+', want: 1.2}, {key: '-', want: 1 / 1.2}, {key: 'x', want: 0}} {
		if got := zoomStepFactor(tt.key); got != tt.want {
			t.Errorf("zoomStepFactor(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestShiftClickZoomUsesMouseButton(t *testing.T) {
	for _, tt := range []struct {
		name  string
		mouse goWidgets.MouseInfo
		want  float64
	}{
		{name: "shift left zooms in", mouse: goWidgets.MouseInfo{Button: goWidgets.MouseLeft, Mods: goWidgets.ModShift}, want: 1.2},
		{name: "shift right zooms out", mouse: goWidgets.MouseInfo{Button: goWidgets.MouseRight, Mods: goWidgets.ModShift}, want: 1 / 1.2},
		{name: "plain left does not zoom", mouse: goWidgets.MouseInfo{Button: goWidgets.MouseLeft}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := shiftClickZoomFactor(tt.mouse); got != tt.want {
				t.Errorf("shiftClickZoomFactor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortEntriesByTypeSizeAndDate(t *testing.T) {
	base := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	entries := []imageEntry{
		{name: "z.png", size: 10, modTime: base.Add(time.Hour)},
		{name: "b.jpg", size: 30, modTime: base},
		{name: "a.jpg", size: 20, modTime: base.Add(2 * time.Hour)},
	}

	tests := []struct {
		name       string
		mode       int
		descending bool
		want       []string
	}{
		{name: "type ascending", mode: sortByType, want: []string{"a.jpg", "b.jpg", "z.png"}},
		{name: "size descending", mode: sortBySize, descending: true, want: []string{"b.jpg", "a.jpg", "z.png"}},
		{name: "date ascending", mode: sortByDate, want: []string{"b.jpg", "z.png", "a.jpg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := append([]imageEntry(nil), entries...)
			sortEntries(got, tt.mode, tt.descending)
			gotNames := []string{got[0].name, got[1].name, got[2].name}
			for i, want := range tt.want {
				if gotNames[i] != want {
					t.Fatalf("sort order = %v, want %v", gotNames, tt.want)
				}
			}
		})
	}
}

func TestSuggestedSaveNameKeepsWritableFormats(t *testing.T) {
	cases := map[string]string{
		filepath.Join("dir", "photo.jpg"):  "photo.jpg",
		filepath.Join("dir", "photo.JPEG"): "photo.JPEG",
		filepath.Join("dir", "pic.png"):    "pic.png",
		filepath.Join("dir", "scan.tiff"):  "scan.png",
		"anim.gif":                         "anim.png",
		"noext":                            "noext.png",
	}
	for in, want := range cases {
		if got := suggestedSaveName(in); got != want {
			t.Errorf("suggestedSaveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveImageWritesDecodableFiles(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 200, G: 30, B: 40, A: 255})
		}
	}
	dir := t.TempDir()

	pngPath, err := saveImage(filepath.Join(dir, "out.png"), src, export.DefaultQuality)
	if err != nil {
		t.Fatalf("save png: %v", err)
	}
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if got.Bounds() != src.Bounds() {
		t.Fatalf("png bounds = %v, want %v", got.Bounds(), src.Bounds())
	}
	if r, g, b, _ := got.At(1, 1).RGBA(); r>>8 != 200 || g>>8 != 30 || b>>8 != 40 {
		t.Fatalf("png pixel = %d,%d,%d, want 200,30,40", r>>8, g>>8, b>>8)
	}

	jpgPath, err := saveImage(filepath.Join(dir, "out.jpg"), src, export.DefaultQuality)
	if err != nil {
		t.Fatalf("save jpeg: %v", err)
	}
	f, err = os.Open(jpgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || format != "jpeg" || cfg.Width != 4 || cfg.Height != 3 {
		t.Fatalf("jpeg config = %+v, %q, %v", cfg, format, err)
	}

	bare, err := saveImage(filepath.Join(dir, "bare"), src, export.DefaultQuality)
	if err != nil {
		t.Fatalf("save without extension: %v", err)
	}
	if want := filepath.Join(dir, "bare.png"); bare != want {
		t.Fatalf("path without extension = %q, want %q", bare, want)
	}
	if _, err := os.Stat(bare); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	if _, err := saveImage(filepath.Join(dir, "bad.bmp"), src, export.DefaultQuality); err == nil {
		t.Fatal("saving to .bmp succeeded, want an unsupported-extension error")
	}
	if _, err := saveImage(filepath.Join(dir, "nil.png"), nil, export.DefaultQuality); err == nil {
		t.Fatal("saving a nil image succeeded, want an error")
	}
}

func TestParseJPEGQuality(t *testing.T) {
	ok := []struct {
		in   string
		want int
	}{
		{"1", 1},
		{"100", 100},
		{"90", 90},
		{" 75 ", 75},
		{"\t42\n", 42},
	}
	for _, c := range ok {
		got, err := parseJPEGQuality(c.in)
		if err != nil || got != c.want {
			t.Errorf("parseJPEGQuality(%q) = %d, %v, want %d", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{"0", "101", "-5", "", "   ", "abc", "9x", "7.5", "1 0", "999999999999999999999"} {
		if got, err := parseJPEGQuality(in); err == nil {
			t.Errorf("parseJPEGQuality(%q) = %d, want an error", in, got)
		}
	}
	if got, err := parseJPEGQuality(strconv.Itoa(export.DefaultQuality)); err != nil || got != 90 {
		t.Errorf("default quality = %d, %v, want 90", got, err)
	}
}

func TestFormatJPEGComparison(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	variants, err := export.CompareJPEG(src, jpegCompareQualities)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 3 {
		t.Fatalf("got %d variants, want 3", len(variants))
	}
	got := formatJPEGComparison([]export.Variant{{Quality: 50, Size: 512}, {Quality: 90, Size: 2048}})
	want := "Размер JPEG: качество 50 — 512 B; качество 90 — 2.0 KiB"
	if got != want {
		t.Errorf("formatJPEGComparison = %q, want %q", got, want)
	}
}

func TestIsHistogramShortcut(t *testing.T) {
	cases := []struct {
		name string
		key  goWidgets.Key
		want bool
	}{
		{"lower i", goWidgets.Key{KeyDown: true, Char: 'i'}, true},
		{"upper I", goWidgets.Key{KeyDown: true, Char: 'I'}, true},
		{"ctrl+i", goWidgets.Key{KeyDown: true, Char: 'i', ControlKeyState: 0x0008}, false},
		{"alt+i", goWidgets.Key{KeyDown: true, Char: 'i', ControlKeyState: 0x0002}, false},
		{"other letter", goWidgets.Key{KeyDown: true, Char: 'f'}, false},
		{"no char", goWidgets.Key{KeyDown: true, VirtualKeyCode: 0x49}, false},
	}
	for _, tc := range cases {
		if got := isHistogramShortcut(tc.key); got != tc.want {
			t.Errorf("%s: isHistogramShortcut = %v, want %v", tc.name, got, tc.want)
		}
	}
}
