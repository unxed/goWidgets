package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestDropShadowAddsSoftOffsetAndPreservesContent(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 7, 7, 9))
	src.SetNRGBA(5, 7, color.NRGBA{R: 220, G: 40, B: 30, A: 255})
	src.SetNRGBA(6, 7, color.NRGBA{R: 20, G: 80, B: 160, A: 255})
	src.SetNRGBA(5, 8, color.NRGBA{R: 60, G: 100, B: 140, A: 255})
	src.SetNRGBA(6, 8, color.NRGBA{R: 200, G: 180, B: 160, A: 255})

	got := DropShadow(src, 1, 1, color.NRGBA{A: 180})
	if got == nil {
		t.Fatal("shadow image is nil")
	}
	if got.Bounds() != image.Rect(0, 0, 5, 5) {
		t.Fatalf("shadow bounds = %v, want (0,0)-(5,5)", got.Bounds())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			want := src.NRGBAAt(5+x, 7+y)
			if pixel := got.NRGBAAt(x+1, y+1); pixel != want {
				t.Errorf("content pixel (%d,%d) = %#v, want %#v", x, y, pixel, want)
			}
		}
	}
	if got.NRGBAAt(0, 0).A != 0 {
		t.Errorf("unshifted corner shadow alpha = %d, want 0", got.NRGBAAt(0, 0).A)
	}
	if pixel := got.NRGBAAt(4, 4); pixel.A == 0 || pixel.R != 0 || pixel.G != 0 || pixel.B != 0 {
		t.Errorf("soft shadow pixel = %#v, want a dark translucent edge", pixel)
	}
}

func TestDropShadowRejectsInvalidInput(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for _, params := range [][2]int{{0, 1}, {65, 1}, {1, -1}, {1, 129}} {
		if DropShadow(src, params[0], params[1], color.NRGBA{A: 128}) != nil {
			t.Errorf("radius/offset %v should be rejected", params)
		}
	}
	if DropShadow(nil, 1, 1, color.NRGBA{A: 128}) != nil || DropShadow(image.NewRGBA(image.Rectangle{}), 1, 1, color.NRGBA{A: 128}) != nil {
		t.Error("nil and empty images must be rejected")
	}
}
