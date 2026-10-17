package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/unxed/goWidgets/showcase/pixy/viewport"
)

func TestLoupeAvailableOnlyWhenFitted(t *testing.T) {
	view := viewport.New(viewport.Size{W: 100, H: 100}, viewport.Size{W: 400, H: 300})
	if !loupeAvailable(view) {
		t.Fatal("loupeAvailable(fitted) = false, want true")
	}
	view.Zoom = 2
	if loupeAvailable(view) {
		t.Fatal("loupeAvailable(zoomed) = true, want false")
	}
	view.SetMode(viewport.Actual)
	if loupeAvailable(view) {
		t.Fatal("loupeAvailable(actual size) = true, want false")
	}
}

func TestPaintLoupeMagnifiesSourceRegion(t *testing.T) {
	red, blue := color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, red)
	src.SetRGBA(1, 0, blue)
	dst := image.NewRGBA(image.Rect(0, 0, 40, 20))
	paintLoupe(dst, src, viewport.Loupe{
		Dest: viewport.Rect{Min: viewport.Point{X: 10, Y: 5}, Max: viewport.Point{X: 30, Y: 15}},
		Src:  viewport.Rect{Min: viewport.Point{}, Max: viewport.Point{X: 2, Y: 1}},
	})
	if got := dst.RGBAAt(14, 10); got != red {
		t.Fatalf("left half = %+v, want red", got)
	}
	if got := dst.RGBAAt(25, 10); got != blue {
		t.Fatalf("right half = %+v, want blue", got)
	}
	if got := dst.RGBAAt(5, 10); got != (color.RGBA{}) {
		t.Fatalf("pixel outside the loupe = %+v, want untouched", got)
	}
	if got := dst.RGBAAt(10, 10); got == red {
		t.Fatal("loupe border was not drawn")
	}
}

func TestPaintLoupeClipsToDestinationAndIgnoresEmpty(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	paintLoupe(dst, src, viewport.Loupe{
		Dest: viewport.Rect{Min: viewport.Point{X: 4, Y: 4}, Max: viewport.Point{X: 20, Y: 20}},
		Src:  viewport.Rect{Max: viewport.Point{X: 4, Y: 4}},
	})
	paintLoupe(dst, src, viewport.Loupe{})
	paintLoupe(nil, src, viewport.Loupe{})
	paintLoupe(dst, nil, viewport.Loupe{})
}
