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
	pixel := got.RGBA64At(0, 0)
	near := func(actual, expected uint16) bool {
		delta := int(actual) - int(expected)
		return delta >= -257 && delta <= 257
	}
	if !near(pixel.R, 37_768) || !near(pixel.G, 42_768) || !near(pixel.B, 47_768) || pixel.A != 65_535 {
		t.Errorf("opaque pixel = %#v, want adjusted channels within one 8-bit step", pixel)
	}
	if alpha := got.RGBAAt(1, 0).A; alpha != 128 {
		t.Errorf("translucent alpha = %d, want preserved alpha quantized to 128", alpha)
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
