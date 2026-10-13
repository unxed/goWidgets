package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestGrayscaleLumaAlphaAndNonZeroBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(7, 9, 9, 10))
	src.SetNRGBA(7, 9, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
	src.SetNRGBA(8, 9, color.NRGBA{R: 255, A: 128})

	got := Grayscale(src)
	if got == nil {
		t.Fatal("grayscale returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("grayscale bounds = %v, want (0,0)-(2,1)", got.Bounds())
	}
	if pixel := got.RGBAAt(0, 0); pixel != (color.RGBA{R: 141, G: 141, B: 141, A: 255}) {
		t.Errorf("opaque gray pixel = %#v, want Rec.601 gray 141", pixel)
	}
	if pixel := got.RGBAAt(1, 0); pixel.R != pixel.G || pixel.G != pixel.B || pixel.A != 128 {
		t.Errorf("translucent gray pixel = %#v, want equal channels and alpha 128", pixel)
	}
}

func TestGrayscaleRejectsNilAndEmptyImages(t *testing.T) {
	if Grayscale(nil) != nil {
		t.Error("Grayscale(nil) must return nil")
	}
	if Grayscale(image.NewRGBA(image.Rectangle{})) != nil {
		t.Error("Grayscale(empty) must return nil")
	}
}
