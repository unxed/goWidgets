package transform

import (
	"image"
	"image/color"
)

// Grayscale returns a zero-based copy converted with Rec.601 luma weights.
// Alpha is retained; nil or empty images return nil.
func Grayscale(src image.Image) *image.RGBA {
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
			luma := uint16((uint64(pixel.R)*299 + uint64(pixel.G)*587 + uint64(pixel.B)*114 + 500) / 1000)
			dst.Set(x, y, color.NRGBA64{R: luma, G: luma, B: luma, A: pixel.A})
		}
	}
	return dst
}
