// Package transform contains dependency-free pixel operations for pixy.
package transform

import "image"

// RotateClockwise rotates src by 90 degrees into a new zero-based RGBA image.
func RotateClockwise(src image.Image) *image.RGBA {
	if src == nil || src.Bounds().Empty() {
		return image.NewRGBA(image.Rectangle{})
	}
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, sb.Dy(), sb.Dx()))
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			dst.Set(sb.Dy()-1-y, x, src.At(sb.Min.X+x, sb.Min.Y+y))
		}
	}
	return dst
}

// MirrorHorizontal returns a copy reflected across the vertical axis.
func MirrorHorizontal(src image.Image) *image.RGBA {
	if src == nil || src.Bounds().Empty() {
		return image.NewRGBA(image.Rectangle{})
	}
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			dst.Set(sb.Dx()-1-x, y, src.At(sb.Min.X+x, sb.Min.Y+y))
		}
	}
	return dst
}

// MirrorVertical returns a copy reflected across the horizontal axis.
func MirrorVertical(src image.Image) *image.RGBA {
	if src == nil || src.Bounds().Empty() {
		return image.NewRGBA(image.Rectangle{})
	}
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	for y := 0; y < sb.Dy(); y++ {
		for x := 0; x < sb.Dx(); x++ {
			dst.Set(x, sb.Dy()-1-y, src.At(sb.Min.X+x, sb.Min.Y+y))
		}
	}
	return dst
}
