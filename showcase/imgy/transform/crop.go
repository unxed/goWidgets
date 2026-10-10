package transform

import "image"

// Crop copies the intersection of rect and src.Bounds() into a zero-based RGBA.
func Crop(src image.Image, rect image.Rectangle) *image.RGBA {
	if src == nil {
		return image.NewRGBA(image.Rectangle{})
	}
	bounds := rect.Intersect(src.Bounds())
	if bounds.Empty() {
		return image.NewRGBA(image.Rectangle{})
	}
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			dst.Set(x, y, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}
