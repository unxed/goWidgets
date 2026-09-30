package viewport

import (
	"math"
	"testing"
)

func closeTo(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

func TestModes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       Mode
		wantScale  float64
		wantOffset Point
	}{
		{name: "fit", mode: Fit, wantScale: 0.5, wantOffset: Point{Y: 50}},
		{name: "actual", mode: Actual, wantScale: 1, wantOffset: Point{X: -200, Y: -50}},
		{name: "fill", mode: Fill, wantScale: 0.75, wantOffset: Point{X: -100}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := New(Size{W: 800, H: 400}, Size{W: 400, H: 300})
			v.SetMode(tc.mode)
			got := v.Transform()
			if !closeTo(got.Scale, tc.wantScale) || !closeTo(got.Offset.X, tc.wantOffset.X) || !closeTo(got.Offset.Y, tc.wantOffset.Y) {
				t.Fatalf("Transform() = %+v, want scale %v offset %+v", got, tc.wantScale, tc.wantOffset)
			}
		})
	}
}

func TestZoomKeepsCursorAnchor(t *testing.T) {
	v := New(Size{W: 800, H: 400}, Size{W: 400, H: 300})
	cursor := Point{X: 100, Y: 100}
	anchor := v.ImagePoint(cursor)
	v.ZoomAt(cursor, 2)
	got := v.Transform()
	if !closeTo(got.Scale, 1) {
		t.Fatalf("zoom scale = %v, want 1", got.Scale)
	}
	if mapped := (Point{X: anchor.X*got.Scale + got.Offset.X, Y: anchor.Y*got.Scale + got.Offset.Y}); !closeTo(mapped.X, cursor.X) || !closeTo(mapped.Y, cursor.Y) {
		t.Fatalf("anchored image point = %+v, want cursor %+v", mapped, cursor)
	}
}

func TestPanAndClamp(t *testing.T) {
	v := New(Size{W: 800, H: 400}, Size{W: 400, H: 300})
	v.ZoomAt(Point{X: 200, Y: 150}, 2)
	v.PanBy(Point{X: -1000, Y: 1000})
	v.ClampPan()
	offset := v.Transform().Offset
	if !closeTo(offset.X, -400) || !closeTo(offset.Y, 0) {
		t.Fatalf("clamped offset = %+v, want (-400, 0)", offset)
	}
}

func TestZoomClampsAndIgnoresInvalidFactor(t *testing.T) {
	v := New(Size{W: 100, H: 100}, Size{W: 100, H: 100})
	cursor := Point{X: 50, Y: 50}
	v.ZoomAt(cursor, math.NaN())
	if v.Zoom != 1 {
		t.Fatalf("NaN zoom factor changed zoom to %v", v.Zoom)
	}
	v.ZoomAt(cursor, 1e9)
	if v.Zoom != maxZoom {
		t.Fatalf("zoom = %v, want max %v", v.Zoom, maxZoom)
	}
	v.ZoomAt(cursor, 1e-9)
	if v.Zoom != minZoom {
		t.Fatalf("zoom = %v, want min %v", v.Zoom, minZoom)
	}
}
