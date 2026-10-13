package transform

import (
	"image"
	"image/color"
	"image/draw"
)

// DropShadow returns a zero-based image with a box-blurred alpha shadow behind src.
// Invalid, empty, or excessively large requests return nil.
func DropShadow(src image.Image, radius, offset int, shadow color.NRGBA) *image.NRGBA {
	if src == nil || src.Bounds().Empty() || radius < 1 || radius > 64 || offset < 0 || offset > 128 {
		return nil
	}
	bounds := src.Bounds()
	maskW64 := int64(bounds.Dx()) + 2*int64(radius)
	maskH64 := int64(bounds.Dy()) + 2*int64(radius)
	outW64 := maskW64 + int64(offset)
	outH64 := maskH64 + int64(offset)
	if maskW64 <= 0 || maskH64 <= 0 || outW64 <= 0 || outH64 <= 0 || outW64 > maxResizePixels/outH64 || maskW64 > maxResizePixels/maskH64 {
		return nil
	}
	maskW, maskH := int(maskW64), int(maskH64)
	window := 2*radius + 1
	horizontal := make([]uint8, maskW*maskH)
	for y := 0; y < maskH; y++ {
		sum := 0
		for x := -2 * radius; x <= 0; x++ {
			if x >= 0 && x < bounds.Dx() {
				sum += int(color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y-radius)).(color.NRGBA).A)
			}
		}
		for x := 0; x < maskW; x++ {
			if x > 0 {
				remove := x - 1 - 2*radius
				add := x
				if remove >= 0 && remove < bounds.Dx() {
					sum -= int(color.NRGBAModel.Convert(src.At(bounds.Min.X+remove, bounds.Min.Y+y-radius)).(color.NRGBA).A)
				}
				if add >= 0 && add < bounds.Dx() {
					sum += int(color.NRGBAModel.Convert(src.At(bounds.Min.X+add, bounds.Min.Y+y-radius)).(color.NRGBA).A)
				}
			}
			horizontal[y*maskW+x] = uint8((sum + window/2) / window)
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, int(outW64), int(outH64)))
	for x := 0; x < maskW; x++ {
		sum := 0
		for y := 0; y <= radius && y < maskH; y++ {
			sum += int(horizontal[y*maskW+x])
		}
		for y := 0; y < maskH; y++ {
			if y > 0 {
				remove := y - 1 - radius
				add := y + radius
				if remove >= 0 && remove < maskH {
					sum -= int(horizontal[remove*maskW+x])
				}
				if add >= 0 && add < maskH {
					sum += int(horizontal[add*maskW+x])
				}
			}
			alpha := uint8((sum + window/2) / window)
			alpha = uint8((uint16(alpha)*uint16(shadow.A) + 127) / 255)
			if alpha != 0 {
				pixel := shadow
				pixel.A = alpha
				dst.SetNRGBA(x+offset, y+offset, pixel)
			}
		}
	}
	draw.Draw(dst, image.Rect(radius, radius, radius+bounds.Dx(), radius+bounds.Dy()), src, bounds.Min, draw.Over)
	return dst
}
