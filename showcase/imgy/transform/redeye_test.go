package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestReduceRedEyeTargetsLocalCircleAndPreservesAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 7, 8, 8))
	src.SetNRGBA(5, 7, color.NRGBA{R: 230, G: 25, B: 30, A: 255})
	src.SetNRGBA(6, 7, color.NRGBA{R: 230, G: 25, B: 30, A: 128})
	src.SetNRGBA(7, 7, color.NRGBA{R: 230, G: 25, B: 30, A: 255})

	got := ReduceRedEye(src, image.Pt(1, 0), 1)
	if got == nil {
		t.Fatal("red-eye reduction returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 3, 1) {
		t.Fatalf("adjusted bounds = %v, want (0,0)-(3,1)", got.Bounds())
	}
	center := color.NRGBA64Model.Convert(got.At(1, 0)).(color.NRGBA64)
	if center.R > center.G || center.R > center.B || center.A != 128*257 {
		t.Errorf("target pixel = %#v, want neutralized red and preserved alpha", center)
	}
	for _, x := range []int{0, 2} {
		pixel := got.RGBAAt(x, 0)
		if pixel.R != 230 || pixel.G != 25 || pixel.B != 30 {
			t.Errorf("outside pixel %d = %#v, want unchanged red pixel", x, pixel)
		}
	}
}

func TestReduceRedEyePreservesCatchlightsAndRejectsInvalidInput(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if got := ReduceRedEye(src, image.Point{}, 1).RGBAAt(0, 0); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("catchlight = %#v, want unchanged white", got)
	}
	for _, radius := range []int{0, 513} {
		if ReduceRedEye(src, image.Point{}, radius) != nil {
			t.Errorf("radius %d should be rejected", radius)
		}
	}
	if ReduceRedEye(nil, image.Point{}, 1) != nil || ReduceRedEye(image.NewRGBA(image.Rectangle{}), image.Point{}, 1) != nil {
		t.Error("nil and empty images must return nil")
	}
}
