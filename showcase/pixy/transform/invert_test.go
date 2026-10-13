package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestInvertChannelsAlphaAndNonZeroBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(2, 5, 4, 6))
	src.SetNRGBA(2, 5, color.NRGBA{R: 10, G: 100, B: 250, A: 255})
	src.SetNRGBA(3, 5, color.NRGBA{R: 20, G: 80, B: 240, A: 128})

	got := Invert(src)
	if got == nil {
		t.Fatal("invert returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("invert bounds = %v, want (0,0)-(2,1)", got.Bounds())
	}
	if pixel := got.RGBAAt(0, 0); pixel != (color.RGBA{R: 245, G: 155, B: 5, A: 255}) {
		t.Errorf("opaque inverted pixel = %#v, want (245,155,5,255)", pixel)
	}
	pixel := color.NRGBA64Model.Convert(got.At(1, 0)).(color.NRGBA64)
	if pixel.A != 128*257 || pixel.R <= pixel.G || pixel.G <= pixel.B {
		t.Errorf("translucent inverted pixel = %#v, want preserved alpha and inverted channel order", pixel)
	}
}

func TestInvertRejectsNilAndEmptyImages(t *testing.T) {
	if Invert(nil) != nil {
		t.Error("Invert(nil) must return nil")
	}
	if Invert(image.NewRGBA(image.Rectangle{})) != nil {
		t.Error("Invert(empty) must return nil")
	}
}
