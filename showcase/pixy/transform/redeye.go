package transform

import (
	"image"
	"image/color"
)

// ReduceRedEye neutralizes strongly red pixels inside a circular, image-local
// target while preserving alpha and bright catchlights. Radius is in pixels.
// Invalid, empty, or excessively large input returns nil.
func ReduceRedEye(src image.Image, center image.Point, radius int) *image.RGBA {
	if src == nil || src.Bounds().Empty() || radius < 1 || radius > 512 {
		return nil
	}
	bounds := src.Bounds()
	if int64(bounds.Dx()) > maxResizePixels/int64(bounds.Dy()) {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	radiusSquared := int64(radius) * int64(radius)
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			pixel := color.NRGBA64Model.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA64)
			dx, dy := int64(x-center.X), int64(y-center.Y)
			if dx*dx+dy*dy <= radiusSquared && isRedEyePixel(pixel) {
				pixel.R = uint16((uint32(pixel.G) + uint32(pixel.B)) / 2)
			}
			dst.Set(x, y, pixel)
		}
	}
	return dst
}

func isRedEyePixel(pixel color.NRGBA64) bool {
	r, g, b := uint32(pixel.R), uint32(pixel.G), uint32(pixel.B)
	return r > 12_000 && r < 63_500 && r*2 > g*3 && r*2 > b*3
}
