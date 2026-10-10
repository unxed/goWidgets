package histogram

import (
	"image"
	"image/color"
	"testing"
)

func TestCountUsesVisiblePixelsAndNonZeroBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(4, 7, 7, 8))
	src.SetNRGBA(4, 7, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	src.SetNRGBA(5, 7, color.NRGBA{R: 10, G: 40, B: 30, A: 128})
	src.SetNRGBA(6, 7, color.NRGBA{R: 200, G: 210, B: 220, A: 0})

	got := Count(src)
	if got == nil || got.Pixels != 2 {
		t.Fatalf("distribution = %#v, want two visible pixels", got)
	}
	if got.Red[10] != 2 || got.Green[20] != 1 || got.Green[40] != 1 || got.Blue[30] != 2 {
		t.Errorf("unexpected RGB counts: R10=%d G20=%d G40=%d B30=%d", got.Red[10], got.Green[20], got.Green[40], got.Blue[30])
	}
	if got.Red[200] != 0 || got.Green[210] != 0 || got.Blue[220] != 0 {
		t.Error("fully transparent pixel must not affect the histogram")
	}
}

func TestSummaryAndPaint(t *testing.T) {
	distribution := &Distribution{}
	distribution.Red[12] = 4
	distribution.Green[24] = 3
	distribution.Blue[36] = 2
	distribution.Pixels = 4
	if summary := distribution.Summary(); summary == "" || summary == (*Distribution)(nil).Summary() {
		t.Fatalf("Summary() = %q, want channel details", summary)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 320, 160))
	Paint(dst, distribution)
	background := dst.RGBAAt(0, 0)
	if background != (color.RGBA{R: 25, G: 29, B: 37, A: 255}) {
		t.Errorf("chart background = %#v, want dark opaque background", background)
	}
	colored := 0
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			pixel := dst.RGBAAt(x, y)
			if pixel.R > 100 || pixel.G > 100 || pixel.B > 150 {
				colored++
			}
		}
	}
	if colored == 0 {
		t.Fatal("Paint produced no visible channel curves")
	}
}

func TestCountRejectsNilAndEmpty(t *testing.T) {
	if Count(nil) != nil || Count(image.NewRGBA(image.Rectangle{})) != nil {
		t.Error("nil and empty images must return nil")
	}
}
