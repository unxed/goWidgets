package core

import (
	"errors"
	"image"
)

// ErrCanvasUnsupported is returned when the selected driver cannot draw a
// pixel-buffer Canvas.
var ErrCanvasUnsupported = errors.New("core: canvas is not supported by this backend")

// PixelOrder describes the byte order expected by a native image API.
type PixelOrder uint8

const (
	PixelRGBA PixelOrder = iota
	PixelBGRA
	PixelARGB
)

// ConvertRGBA copies an RGBA image into a tightly packed byte buffer in order.
func ConvertRGBA(dst []byte, src *image.RGBA, order PixelOrder) error {
	if src == nil {
		return errors.New("core: nil RGBA image")
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if len(dst) < w*h*4 {
		return errors.New("core: destination buffer is too small")
	}
	if order > PixelARGB {
		return errors.New("core: unknown pixel order")
	}
	for y := 0; y < h; y++ {
		row := src.Pix[y*src.Stride : y*src.Stride+w*4]
		for x := 0; x < w; x++ {
			r, g, b, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
			switch order {
			case PixelRGBA:
				i := (y*w + x) * 4
				dst[i], dst[i+1], dst[i+2], dst[i+3] = r, g, b, a
			case PixelBGRA:
				i := (y*w + x) * 4
				dst[i], dst[i+1], dst[i+2], dst[i+3] = b, g, r, a
			case PixelARGB:
				i := (y*w + x) * 4
				dst[i], dst[i+1], dst[i+2], dst[i+3] = a, r, g, b
			}
		}
	}
	return nil
}
