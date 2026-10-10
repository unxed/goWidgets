package transform

import (
	"image"
	"image/color"
)

// AdjustBrightness applies an offset in [-100, 100] percent while preserving
// alpha. Positive values move channels toward white; negative values toward
// black. Invalid input returns nil.
func AdjustBrightness(src image.Image, percent int) *image.RGBA {
	if src == nil || src.Bounds().Empty() || percent < -100 || percent > 100 {
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
				R: adjustBrightnessChannel(pixel.R, percent),
				G: adjustBrightnessChannel(pixel.G, percent),
				B: adjustBrightnessChannel(pixel.B, percent),
				A: pixel.A,
			})
		}
	}
	return dst
}

func adjustBrightnessChannel(channel uint16, percent int) uint16 {
	if percent >= 0 {
		return uint16(uint32(channel) + (uint32(65_535-channel)*uint32(percent)+50)/100)
	}
	return uint16((uint32(channel)*uint32(100+percent) + 50) / 100)
}
