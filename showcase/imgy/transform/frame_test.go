package transform

import (
	"image"
	"image/color"
	"testing"
)

func TestAddFrameAddsBorderAndPreservesPixels(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 7, 7, 9))
	src.SetNRGBA(5, 7, color.NRGBA{R: 200, G: 30, B: 40, A: 128})
	src.SetNRGBA(6, 7, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	src.SetNRGBA(5, 8, color.NRGBA{R: 40, G: 50, B: 60, A: 0})
	src.SetNRGBA(6, 8, color.NRGBA{R: 70, G: 80, B: 90, A: 255})
	border := color.NRGBA{R: 12, G: 34, B: 56, A: 200}

	got := AddFrame(src, 1, border)
	if got == nil {
		t.Fatal("framed image is nil")
	}
	if got.Bounds() != image.Rect(0, 0, 4, 4) {
		t.Fatalf("framed bounds = %v, want (0,0)-(4,4)", got.Bounds())
	}
	if pixel := color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA); pixel != border {
		t.Errorf("border pixel = %#v, want %#v", pixel, border)
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			want := color.NRGBAModel.Convert(src.At(5+x, 7+y)).(color.NRGBA)
			if pixel := color.NRGBAModel.Convert(got.At(x+1, y+1)).(color.NRGBA); pixel != want {
				t.Errorf("content pixel (%d,%d) = %#v, want %#v", x, y, pixel, want)
			}
		}
	}
}

func TestAddFrameRejectsInvalidInput(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for _, width := range []int{-1, 0, 257} {
		if AddFrame(src, width, color.Black) != nil {
			t.Errorf("width %d should be rejected", width)
		}
	}
	if AddFrame(nil, 1, color.Black) != nil || AddFrame(src, 1, nil) != nil || AddFrame(image.NewRGBA(image.Rectangle{}), 1, color.Black) != nil {
		t.Error("nil and empty inputs must be rejected")
	}
}
