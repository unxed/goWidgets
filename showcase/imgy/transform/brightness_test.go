package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestAdjustBrightnessPreservesAlphaAndUsesNonZeroBounds(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(4, 7, 6, 8))
	src.SetNRGBA64(4, 7, color.NRGBA64{R: 10_000, G: 20_000, B: 30_000, A: 65_535})
	src.SetNRGBA64(5, 7, color.NRGBA64{R: 10_000, G: 20_000, B: 30_000, A: 32_768})

	got := AdjustBrightness(src, 50)
	if got == nil {
		t.Fatal("brightness adjustment returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("adjusted bounds = %v, want (0,0)-(2,1)", got.Bounds())
	}
	if pixel := color.NRGBA64Model.Convert(got.At(0, 0)).(color.NRGBA64); pixel != (color.NRGBA64{R: 37_768, G: 42_768, B: 47_768, A: 65_535}) {
		t.Errorf("opaque pixel = %#v, want brightened channels with alpha preserved", pixel)
	}
	if pixel := color.NRGBA64Model.Convert(got.At(1, 0)).(color.NRGBA64); pixel.A != 32_768 {
		t.Errorf("translucent alpha = %d, want 32768", pixel.A)
	}
}

func TestAdjustBrightnessExtremesAndInvalidInput(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 64, G: 128, B: 192, A: 255})
	if got := AdjustBrightness(src, -100).RGBAAt(0, 0); got != (color.RGBA{A: 255}) {
		t.Errorf("full negative adjustment = %#v, want black with source alpha", got)
	}
	if got := AdjustBrightness(src, 100).RGBAAt(0, 0); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("full positive adjustment = %#v, want white with source alpha", got)
	}
	for _, percent := range []int{-101, 101} {
		if got := AdjustBrightness(src, percent); got != nil {
			t.Errorf("AdjustBrightness(%d) = non-nil, want nil", percent)
		}
	}
}
