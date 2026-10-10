package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestRotateClockwise(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 6, 10))
	src.SetRGBA(4, 7, color.RGBA{R: 255, A: 255})
	src.SetRGBA(5, 7, color.RGBA{G: 255, A: 255})
	src.SetRGBA(4, 8, color.RGBA{B: 255, A: 255})
	src.SetRGBA(5, 8, color.RGBA{R: 255, G: 255, A: 255})
	src.SetRGBA(4, 9, color.RGBA{G: 255, B: 255, A: 255})
	src.SetRGBA(5, 9, color.RGBA{R: 255, B: 255, A: 255})

	got := RotateClockwise(src)
	if got.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("rotated bounds = %v, want (0,0)-(3,2)", got.Bounds())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			want := src.RGBAAt(4+x, 7+y)
			if pixel := got.RGBAAt(2-y, x); pixel != want {
				t.Errorf("pixel at (%d,%d) = %#v, want %#v", 2-y, x, pixel, want)
			}
		}
	}
}

func TestRotateClockwiseEmpty(t *testing.T) {
	got := RotateClockwise(image.NewRGBA(image.Rectangle{}))
	if got.Bounds() != (image.Rectangle{}) {
		t.Fatalf("empty rotated bounds = %v", got.Bounds())
	}
}
