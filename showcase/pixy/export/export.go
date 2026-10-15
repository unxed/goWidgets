// Package export encodes an edited pixy image to PNG or JPEG (FEA-026).
//
// It is pure image/IO logic: the Save/Save As UI is a separate step. JPEG has no
// alpha channel, so transparent pixels are composited over white; PNG is lossless
// and ignores the quality argument.
package export

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Format is an output image format.
type Format int

const (
	PNG Format = iota + 1
	JPEG
)

// Quality limits for JPEG (the image/jpeg range).
const (
	MinQuality     = 1
	MaxQuality     = 100
	DefaultQuality = 90
)

// ErrNoImage is returned for a nil or empty image.
var ErrNoImage = errors.New("export: nil or empty image")

// FormatFromPath chooses the format from the file extension (.png, .jpg, .jpeg;
// case-insensitive).
func FormatFromPath(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return PNG, nil
	case ".jpg", ".jpeg":
		return JPEG, nil
	}
	return 0, fmt.Errorf("export: unsupported extension %q (want .png, .jpg or .jpeg)", filepath.Ext(path))
}

// Encode writes img to w. quality (MinQuality..MaxQuality) matters only for JPEG
// and is validated only then.
func Encode(w io.Writer, img image.Image, format Format, quality int) error {
	if img == nil || img.Bounds().Empty() {
		return ErrNoImage
	}
	switch format {
	case PNG:
		return png.Encode(w, img)
	case JPEG:
		if quality < MinQuality || quality > MaxQuality {
			return fmt.Errorf("export: JPEG quality %d out of range %d..%d", quality, MinQuality, MaxQuality)
		}
		return jpeg.Encode(w, onWhite(img), &jpeg.Options{Quality: quality})
	}
	return fmt.Errorf("export: unknown format %d", format)
}

// onWhite returns a zero-based opaque copy of img with transparency composited over white.
func onWhite(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
	return dst
}

type countWriter struct{ n int }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

// EncodedSize returns the size in bytes that Encode would produce.
func EncodedSize(img image.Image, format Format, quality int) (int, error) {
	var c countWriter
	if err := Encode(&c, img, format, quality); err != nil {
		return 0, err
	}
	return c.n, nil
}

// Variant is the encoded size of one JPEG quality setting.
type Variant struct {
	Quality int
	Size    int
}

// CompareJPEG returns the encoded size for each quality, in the given order, so the
// caller can pick a quality by result size.
func CompareJPEG(img image.Image, qualities []int) ([]Variant, error) {
	out := make([]Variant, 0, len(qualities))
	for _, q := range qualities {
		size, err := EncodedSize(img, JPEG, q)
		if err != nil {
			return nil, err
		}
		out = append(out, Variant{Quality: q, Size: size})
	}
	return out, nil
}

// Save encodes img to path in the format given by its extension. The file is written
// to a temporary sibling and renamed over path only on success, so a failed save
// never leaves a truncated or half-written file.
func Save(path string, img image.Image, quality int) error {
	format, err := FormatFromPath(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pixy-save-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	err = Encode(tmp, img, format, quality)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmpName, 0o644) // CreateTemp makes the file private (0600)
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
