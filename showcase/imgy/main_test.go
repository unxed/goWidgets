package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"testing"
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
