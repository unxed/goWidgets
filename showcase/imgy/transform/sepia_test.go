package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestSepiaToneAlphaAndNonZeroBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(2, 5, 4, 6))
	src.SetNRGBA(2, 5, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
	src.SetNRGBA(3, 5, color.NRGBA{R: 20, G: 80, B: 240, A: 128})

	got := Sepia(src)
	if got == nil {
		t.Fatal("sepia returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("sepia bounds = %v, want (0,0)-(2,1)", got.Bounds())
	}
	if pixel := got.RGBAAt(0, 0); pixel.A != 255 || pixel.R < 191 || pixel.R > 193 || pixel.G < 170 || pixel.G > 172 || pixel.B < 133 || pixel.B > 135 {
		t.Errorf("opaque sepia pixel = %#v, want warm tone near (192,171,134)", pixel)
	}
	if pixel := got.RGBAAt(1, 0); pixel.A != 128 || pixel.R <= pixel.G || pixel.G <= pixel.B {
		t.Errorf("translucent sepia pixel = %#v, want warm ordered channels and alpha 128", pixel)
	}
}

func TestSepiaRejectsNilAndEmptyImages(t *testing.T) {
	if Sepia(nil) != nil {
		t.Error("Sepia(nil) must return nil")
	}
	if Sepia(image.NewRGBA(image.Rectangle{})) != nil {
		t.Error("Sepia(empty) must return nil")
	}
}
