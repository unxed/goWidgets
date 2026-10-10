package transform

import (
	"image"
	"image/color"
)

// Sepia returns a zero-based copy with a warm sepia tone, retaining alpha.
// Nil or empty images return nil.
func Sepia(src image.Image) *image.RGBA {
	if src == nil || src.Bounds().Empty() {
		return nil
	}
	bounds := src.Bounds()
	if int64(bounds.Dx()) > maxResizePixels/int64(bounds.Dy()) {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			pixel := color.NRGBA64Model.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA64)
			r := uint64(pixel.R)
			g := uint64(pixel.G)
			b := uint64(pixel.B)
			red := min(uint64(65_535), (393*r+769*g+189*b+500)/1000)
			green := min(uint64(65_535), (349*r+686*g+168*b+500)/1000)
			blue := min(uint64(65_535), (272*r+534*g+131*b+500)/1000)
			dst.Set(x, y, color.NRGBA64{R: uint16(red), G: uint16(green), B: uint16(blue), A: pixel.A})
		}
	}
	return dst
}
