package transform

import (
	"image"
	"image/color"
	"math"
)

const maxResizePixels = 100_000_000

// Resize scales src to the requested dimensions with bilinear filtering.
// Invalid or excessively large requests return nil.
func Resize(src image.Image, width, height int) *image.RGBA {
	if src == nil || src.Bounds().Empty() || width <= 0 || height <= 0 || int64(width) > maxResizePixels/int64(height) {
		return nil
	}
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := (float64(y)+0.5)*float64(sb.Dy())/float64(height) - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := min(y0+1, sb.Dy()-1)
		for x := 0; x < width; x++ {
			sx := (float64(x)+0.5)*float64(sb.Dx())/float64(width) - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := min(x0+1, sb.Dx()-1)
			c00 := color.RGBA64Model.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y0)).(color.RGBA64)
			c10 := color.RGBA64Model.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y0)).(color.RGBA64)
			c01 := color.RGBA64Model.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y1)).(color.RGBA64)
			c11 := color.RGBA64Model.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y1)).(color.RGBA64)
			dst.SetRGBA64(x, y, color.RGBA64{
				R: bilinear(c00.R, c10.R, c01.R, c11.R, fx, fy),
				G: bilinear(c00.G, c10.G, c01.G, c11.G, fx, fy),
				B: bilinear(c00.B, c10.B, c01.B, c11.B, fx, fy),
				A: bilinear(c00.A, c10.A, c01.A, c11.A, fx, fy),
			})
		}
	}
	return dst
}

func bilinear(c00, c10, c01, c11 uint16, fx, fy float64) uint16 {
	top := float64(c00)*(1-fx) + float64(c10)*fx
	bottom := float64(c01)*(1-fx) + float64(c11)*fx
	return uint16(math.Round(top*(1-fy) + bottom*fy))
}
