package main

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/unxed/goWidgets/showcase/imgy/viewport"
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
