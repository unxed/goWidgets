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

func rectCloseTo(got, want Rect) bool {
	return closeTo(got.Min.X, want.Min.X) && closeTo(got.Min.Y, want.Min.Y) &&
		closeTo(got.Max.X, want.Max.X) && closeTo(got.Max.Y, want.Max.Y)
}

func TestLoupeAt(t *testing.T) {
	// Fit scale is 0.5 with offset (0, 50): viewport (200,150) is image (400,200).
	for _, tc := range []struct {
		name          string
		cursor        Point
		size          Size
		magnification float64
		wantDest      Rect
		wantSrc       Rect
	}{
		{
			name: "centered", cursor: Point{X: 200, Y: 150}, size: Size{W: 100, H: 100}, magnification: 4,
			wantDest: Rect{Min: Point{X: 150, Y: 100}, Max: Point{X: 250, Y: 200}},
			wantSrc:  Rect{Min: Point{X: 375, Y: 175}, Max: Point{X: 425, Y: 225}},
		},
		{
			name: "image corner keeps both rectangles inside", cursor: Point{X: 10, Y: 60}, size: Size{W: 100, H: 100}, magnification: 4,
			wantDest: Rect{Min: Point{X: 0, Y: 10}, Max: Point{X: 100, Y: 110}},
			wantSrc:  Rect{Min: Point{X: 0, Y: 0}, Max: Point{X: 50, Y: 50}},
		},
		{
			name: "magnification clamped to 16", cursor: Point{X: 200, Y: 150}, size: Size{W: 100, H: 100}, magnification: 100,
			wantDest: Rect{Min: Point{X: 150, Y: 100}, Max: Point{X: 250, Y: 200}},
			wantSrc:  Rect{Min: Point{X: 393.75, Y: 193.75}, Max: Point{X: 406.25, Y: 206.25}},
		},
		{
			name: "magnification clamped to 1", cursor: Point{X: 200, Y: 150}, size: Size{W: 100, H: 100}, magnification: 0.1,
			wantDest: Rect{Min: Point{X: 150, Y: 100}, Max: Point{X: 250, Y: 200}},
			wantSrc:  Rect{Min: Point{X: 300, Y: 100}, Max: Point{X: 500, Y: 300}},
		},
		{
			name: "loupe larger than viewport shrinks to it", cursor: Point{X: 200, Y: 150}, size: Size{W: 1000, H: 1000}, magnification: 2,
			wantDest: Rect{Min: Point{X: 0, Y: 0}, Max: Point{X: 400, Y: 300}},
			wantSrc:  Rect{Min: Point{X: 200, Y: 50}, Max: Point{X: 600, Y: 350}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := New(Size{W: 800, H: 400}, Size{W: 400, H: 300})
			got, ok := v.LoupeAt(tc.cursor, tc.size, tc.magnification)
			if !ok {
				t.Fatal("LoupeAt() ok = false, want true")
			}
			if !rectCloseTo(got.Dest, tc.wantDest) || !rectCloseTo(got.Src, tc.wantSrc) {
				t.Fatalf("LoupeAt() = %+v, want dest %+v src %+v", got, tc.wantDest, tc.wantSrc)
			}
		})
	}
}

func TestLoupeAtRejectsOutsideAndInvalid(t *testing.T) {
	v := New(Size{W: 800, H: 400}, Size{W: 400, H: 300})
	inside, size := Point{X: 200, Y: 150}, Size{W: 100, H: 100}
	for _, tc := range []struct {
		name          string
		view          Viewport
		cursor        Point
		size          Size
		magnification float64
	}{
		{name: "letterbox above image", view: v, cursor: Point{X: 200, Y: 10}, size: size, magnification: 4},
		{name: "outside viewport", view: v, cursor: Point{X: -5, Y: 150}, size: size, magnification: 4},
		{name: "empty size", view: v, cursor: inside, size: Size{}, magnification: 4},
		{name: "zero magnification", view: v, cursor: inside, size: size, magnification: 0},
		{name: "NaN magnification", view: v, cursor: inside, size: size, magnification: math.NaN()},
		{name: "infinite magnification", view: v, cursor: inside, size: size, magnification: math.Inf(1)},
		{name: "empty viewport", view: New(Size{W: 800, H: 400}, Size{}), cursor: inside, size: size, magnification: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := tc.view.LoupeAt(tc.cursor, tc.size, tc.magnification); ok || got != (Loupe{}) {
				t.Fatalf("LoupeAt() = %+v, %v, want zero value and false", got, ok)
			}
		})
	}
}

func TestLoupeFollowsPointerAndZoom(t *testing.T) {
	v := New(Size{W: 800, H: 400}, Size{W: 400, H: 300})
	v.SetMode(Actual) // scale 1, offset (-200, -50)
	first, ok := v.LoupeAt(Point{X: 200, Y: 150}, Size{W: 80, H: 80}, 2)
	if !ok {
		t.Fatal("first LoupeAt() ok = false")
	}
	moved, ok := v.LoupeAt(Point{X: 260, Y: 150}, Size{W: 80, H: 80}, 2)
	if !ok {
		t.Fatal("moved LoupeAt() ok = false")
	}
	if !closeTo(moved.Dest.Min.X-first.Dest.Min.X, 60) || !closeTo(moved.Src.Min.X-first.Src.Min.X, 60) {
		t.Fatalf("moving the pointer by 60 moved dest by %v and src by %v, want 60 and 60",
			moved.Dest.Min.X-first.Dest.Min.X, moved.Src.Min.X-first.Src.Min.X)
	}
	// A 80-unit loupe at scale 1 and magnification 2 shows 40 image pixels.
	if w := first.Src.Max.X - first.Src.Min.X; !closeTo(w, 40) {
		t.Fatalf("src width = %v, want 40", w)
	}
}
