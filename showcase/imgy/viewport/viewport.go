// Package viewport contains the image-to-window geometry used by imgy.
package viewport

import "math"

// Size is an image or viewport extent in a shared coordinate unit.
type Size struct {
	W, H float64
}

// Point is an image or viewport location in the same coordinate unit.
type Point struct {
	X, Y float64
}

// Mode selects the initial image scale.
type Mode uint8

const (
	Fit    Mode = iota // show the whole image
	Actual             // one image pixel per viewport unit
	Fill               // cover the viewport, cropping the excess
)

// Transform maps image coordinates to viewport coordinates.
type Transform struct {
	Scale  float64
	Offset Point
}

// Viewport is the current view of one image. Zoom is relative to Mode's base
// scale; Pan is in viewport units from the mode's centered position.
type Viewport struct {
	Image Size
	Area  Size
	Mode  Mode
	Zoom  float64
	Pan   Point
}

const (
	minZoom = 0.02
	maxZoom = 64
)

// New creates a fit-to-window viewport.
func New(image, area Size) Viewport {
	return Viewport{Image: image, Area: area, Mode: Fit, Zoom: 1}
}

// Transform returns the scale and centered offset for the current view.
func (v Viewport) Transform() Transform {
	if v.Image.W <= 0 || v.Image.H <= 0 || v.Area.W <= 0 || v.Area.H <= 0 {
		return Transform{}
	}
	sx, sy := v.Area.W/v.Image.W, v.Area.H/v.Image.H
	base := 1.0
	switch v.Mode {
	case Fit:
		base = math.Min(sx, sy)
	case Fill:
		base = math.Max(sx, sy)
	}
	zoom := v.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	scale := base * zoom
	return Transform{
		Scale: scale,
		Offset: Point{
			X: (v.Area.W-v.Image.W*scale)/2 + v.Pan.X,
			Y: (v.Area.H-v.Image.H*scale)/2 + v.Pan.Y,
		},
	}
}

// ImagePoint converts a viewport point to image coordinates.
func (v Viewport) ImagePoint(point Point) Point {
	t := v.Transform()
	if t.Scale == 0 {
		return Point{}
	}
	return Point{X: (point.X - t.Offset.X) / t.Scale, Y: (point.Y - t.Offset.Y) / t.Scale}
}

// ZoomAt changes relative zoom while keeping the image point under cursor
// fixed. Factors outside the supported range are clamped.
func (v *Viewport) ZoomAt(cursor Point, factor float64) {
	if v == nil || factor <= 0 || math.IsNaN(factor) || math.IsInf(factor, 0) {
		return
	}
	old := v.Transform()
	if old.Scale <= 0 {
		return
	}
	anchor := v.ImagePoint(cursor)
	zoom := v.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	zoom = math.Max(minZoom, math.Min(maxZoom, zoom*factor))
	v.Zoom = zoom
	base := v.Transform()
	v.Pan.X += cursor.X - (anchor.X*base.Scale + base.Offset.X)
	v.Pan.Y += cursor.Y - (anchor.Y*base.Scale + base.Offset.Y)
}

// PanBy moves the image by a viewport-relative displacement.
func (v *Viewport) PanBy(delta Point) {
	if v == nil {
		return
	}
	v.Pan.X += delta.X
	v.Pan.Y += delta.Y
}

// ClampPan keeps the image covering the viewport when it is larger than the
// viewport, and centered on any axis where it is smaller.
func (v *Viewport) ClampPan() {
	if v == nil {
		return
	}
	t := v.Transform()
	w, h := v.Image.W*t.Scale, v.Image.H*t.Scale
	if w <= v.Area.W {
		v.Pan.X = 0
	} else {
		left := (v.Area.W - w) / 2
		v.Pan.X = math.Max(-left-w+v.Area.W, math.Min(-left, v.Pan.X))
	}
	if h <= v.Area.H {
		v.Pan.Y = 0
	} else {
		top := (v.Area.H - h) / 2
		v.Pan.Y = math.Max(-top-h+v.Area.H, math.Min(-top, v.Pan.Y))
	}
}

// SetMode resets pan and relative zoom when changing the base scale mode.
func (v *Viewport) SetMode(mode Mode) {
	if v == nil {
		return
	}
	v.Mode, v.Zoom, v.Pan = mode, 1, Point{}
}
