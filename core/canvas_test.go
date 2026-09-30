package core

import (
	"image"
	"image/color"
	"testing"
)

func TestConvertRGBAOrders(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 4})
	src.SetRGBA(1, 0, color.RGBA{R: 5, G: 6, B: 7, A: 8})
	tests := []struct {
		order PixelOrder
		want  []byte
	}{
		{PixelRGBA, []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{PixelBGRA, []byte{3, 2, 1, 4, 7, 6, 5, 8}},
		{PixelARGB, []byte{4, 1, 2, 3, 8, 5, 6, 7}},
	}
	for _, tt := range tests {
		dst := make([]byte, len(tt.want))
		if err := ConvertRGBA(dst, src, tt.order); err != nil {
			t.Fatal(err)
		}
		for i := range dst {
			if dst[i] != tt.want[i] {
				t.Errorf("order %d byte %d = %d, want %d", tt.order, i, dst[i], tt.want[i])
			}
		}
	}
}
