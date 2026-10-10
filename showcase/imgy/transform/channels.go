package transform

import (
	"image"
	"image/color"
)

// AdjustChannels independently moves red, green, and blue toward white or
// black by the requested percentages, retaining alpha. Each adjustment must
// be in [-100, 100]. Invalid or empty input returns nil.
func AdjustChannels(src image.Image, red, green, blue int) *image.RGBA {
	if src == nil || src.Bounds().Empty() || !validAdjustment(red) || !validAdjustment(green) || !validAdjustment(blue) {
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
				R: adjustBrightnessChannel(pixel.R, red),
				G: adjustBrightnessChannel(pixel.G, green),
				B: adjustBrightnessChannel(pixel.B, blue),
				A: pixel.A,
			})
		}
	}
	return dst
}

func validAdjustment(percent int) bool {
	return percent >= -100 && percent <= 100
}
