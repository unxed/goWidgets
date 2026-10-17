package main

import (
	"image"
	"image/color"
	"math"

	"github.com/unxed/goWidgets/showcase/pixy/viewport"
)

const (
	// loupeSize is the side of the square loupe window in logical pixels.
	loupeSize = 220
	// loupeMagnification is how much stronger than the current view scale the loupe shows the image.
	loupeMagnification = 3
)

// loupeAvailable reports whether the held left button starts the loupe (MOU-003) instead of panning:
// only while the whole image is fitted into the window and not zoomed.
func loupeAvailable(view viewport.Viewport) bool {
	return view.Mode == viewport.Fit && view.Zoom == 1
}

// paintLoupe draws the magnified loupe.Src region of src into loupe.Dest of dst (nearest neighbour,
// so single pixels stay visible) and outlines it. Parts of Dest outside dst are skipped.
func paintLoupe(dst *image.RGBA, src image.Image, loupe viewport.Loupe) {
	if dst == nil || src == nil {
		return
	}
	destW, destH := loupe.Dest.Max.X-loupe.Dest.Min.X, loupe.Dest.Max.Y-loupe.Dest.Min.Y
	srcW, srcH := loupe.Src.Max.X-loupe.Src.Min.X, loupe.Src.Max.Y-loupe.Src.Min.Y
	if destW <= 0 || destH <= 0 || srcW <= 0 || srcH <= 0 {
		return
	}
	bounds, area := src.Bounds(), dst.Bounds()
	x0, y0 := int(math.Floor(loupe.Dest.Min.X)), int(math.Floor(loupe.Dest.Min.Y))
	x1, y1 := int(math.Ceil(loupe.Dest.Max.X)), int(math.Ceil(loupe.Dest.Max.Y))
	for y := max(y0, area.Min.Y); y < min(y1, area.Max.Y); y++ {
		sy := bounds.Min.Y + int(math.Floor(loupe.Src.Min.Y+(float64(y)+0.5-loupe.Dest.Min.Y)/destH*srcH))
		sy = max(bounds.Min.Y, min(bounds.Max.Y-1, sy))
		for x := max(x0, area.Min.X); x < min(x1, area.Max.X); x++ {
			sx := bounds.Min.X + int(math.Floor(loupe.Src.Min.X+(float64(x)+0.5-loupe.Dest.Min.X)/destW*srcW))
			sx = max(bounds.Min.X, min(bounds.Max.X-1, sx))
			dst.SetRGBA(x, y, color.RGBAModel.Convert(src.At(sx, sy)).(color.RGBA))
		}
	}
	drawCropOutline(dst, loupe.Dest.Min, loupe.Dest.Max)
}
