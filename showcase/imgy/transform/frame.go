package transform

import (
	"image"
	"image/color"
)

// AddFrame returns a zero-based image with a solid border around src.
// Invalid or excessively large requests return nil.
func AddFrame(src image.Image, width int, border color.Color) *image.RGBA {
	if src == nil || src.Bounds().Empty() || width <= 0 || width > 256 || border == nil {
		return nil
	}
	bounds := src.Bounds()
	frame := int64(width)
	w := int64(bounds.Dx()) + 2*frame
	h := int64(bounds.Dy()) + 2*frame
	if w <= 0 || h <= 0 || w > maxResizePixels/h {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	fill := color.NRGBAModel.Convert(border).(color.NRGBA)
	for y := 0; y < int(h); y++ {
		for x := 0; x < int(w); x++ {
			if x < width || y < width || x >= int(w)-width || y >= int(h)-width {
				dst.SetNRGBA(x, y, fill)
				continue
			}
			dst.Set(x, y, src.At(bounds.Min.X+x-width, bounds.Min.Y+y-width))
		}
	}
	return dst
}
