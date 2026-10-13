package transform

import (
	"image"
	"image/color"
)

// Invert returns a zero-based copy with its RGB channels inverted, retaining alpha.
// Nil or empty images return nil.
func Invert(src image.Image) *image.RGBA {
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
			dst.Set(x, y, color.NRGBA64{
				R: 65_535 - pixel.R,
				G: 65_535 - pixel.G,
				B: 65_535 - pixel.B,
				A: pixel.A,
			})
		}
	}
	return dst
}
