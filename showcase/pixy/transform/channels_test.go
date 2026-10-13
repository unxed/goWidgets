package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestAdjustChannelsIndependentlyPreservesAlphaAndBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(3, 6, 5, 7))
	src.SetNRGBA(3, 6, color.NRGBA{R: 64, G: 128, B: 192, A: 255})
	src.SetNRGBA(4, 6, color.NRGBA{R: 64, G: 128, B: 192, A: 128})

	got := AdjustChannels(src, 50, -50, 0)
	if got == nil {
		t.Fatal("channel adjustment returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("adjusted bounds = %v, want (0,0)-(2,1)", got.Bounds())
	}
	pixel := got.RGBAAt(0, 0)
	if pixel != (color.RGBA{R: 160, G: 64, B: 192, A: 255}) {
		t.Errorf("adjusted opaque pixel = %#v, want (160,64,192,255)", pixel)
	}
	if alpha := got.RGBAAt(1, 0).A; alpha != 128 {
		t.Errorf("translucent alpha = %d, want 128", alpha)
	}
}

func TestAdjustChannelsRejectsInvalidInput(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if AdjustChannels(nil, 0, 0, 0) != nil || AdjustChannels(image.NewRGBA(image.Rectangle{}), 0, 0, 0) != nil {
		t.Fatal("nil and empty images must return nil")
	}
	for _, values := range [][3]int{{-101, 0, 0}, {0, 101, 0}, {0, 0, -101}} {
		if AdjustChannels(src, values[0], values[1], values[2]) != nil {
			t.Errorf("AdjustChannels%v = non-nil, want nil", values)
		}
	}
}
