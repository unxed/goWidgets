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

func TestMirrorHorizontal(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 6, 10))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetRGBA(4+x, 7+y, color.RGBA{R: uint8(20 + x + y*2), A: 255})
		}
	}
	got := MirrorHorizontal(src)
	if got.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatalf("mirrored bounds = %v, want (0,0)-(2,3)", got.Bounds())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			if pixel, want := got.RGBAAt(1-x, y), src.RGBAAt(4+x, 7+y); pixel != want {
				t.Errorf("pixel at (%d,%d) = %#v, want %#v", 1-x, y, pixel, want)
			}
		}
	}
}

func TestMirrorVertical(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 6, 10))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetRGBA(4+x, 7+y, color.RGBA{G: uint8(30 + x + y*2), A: 255})
		}
	}
	got := MirrorVertical(src)
	if got.Bounds() != image.Rect(0, 0, 2, 3) {
		t.Fatalf("mirrored bounds = %v, want (0,0)-(2,3)", got.Bounds())
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			if pixel, want := got.RGBAAt(x, 2-y), src.RGBAAt(4+x, 7+y); pixel != want {
				t.Errorf("pixel at (%d,%d) = %#v, want %#v", x, 2-y, pixel, want)
			}
		}
	}
}

func TestCropIntersectsSourceBounds(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 7, 11))
	for y := 7; y < 11; y++ {
		for x := 4; x < 7; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), A: 255})
		}
	}
	got := Crop(src, image.Rect(5, 8, 9, 10))
	if got.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("cropped bounds = %v, want (0,0)-(2,2)", got.Bounds())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if pixel, want := got.RGBAAt(x, y), src.RGBAAt(5+x, 8+y); pixel != want {
				t.Errorf("pixel at (%d,%d) = %#v, want %#v", x, y, pixel, want)
			}
		}
	}
}

func TestResizeBilinearAndNonZeroBounds(t *testing.T) {
	src := image.NewRGBA(image.Rect(4, 7, 6, 9))
	src.SetRGBA(4, 7, color.RGBA{R: 255, A: 255})
	src.SetRGBA(5, 7, color.RGBA{G: 255, A: 255})
	src.SetRGBA(4, 8, color.RGBA{B: 255, A: 255})
	src.SetRGBA(5, 8, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	got := Resize(src, 3, 3)
	if got == nil {
		t.Fatal("resize returned nil")
	}
	if got.Bounds() != image.Rect(0, 0, 3, 3) {
		t.Fatalf("resized bounds = %v, want (0,0)-(3,3)", got.Bounds())
	}
	if center := got.RGBAAt(1, 1); center.R < 32_000 || center.R > 33_000 || center.G < 32_000 || center.G > 33_000 || center.B < 32_000 || center.B > 33_000 || center.A != 65_535 {
		t.Errorf("center pixel = %#v, want opaque gray", center)
	}
	if corner := got.RGBAAt(0, 0); corner != src.RGBAAt(4, 7) {
		t.Errorf("top-left pixel = %#v, want %#v", corner, src.RGBAAt(4, 7))
	}
}

func TestResizeRejectsInvalidOrExcessiveDimensions(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	for _, size := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {10_001, 10_000}} {
		if got := Resize(src, size[0], size[1]); got != nil {
			t.Errorf("Resize(%d, %d) = non-nil, want nil", size[0], size[1])
		}
	}
}
