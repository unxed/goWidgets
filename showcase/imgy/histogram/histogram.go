// Package histogram counts visible RGB channel values and paints an RGB chart.
package histogram

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// Distribution stores one count for each 8-bit channel level.
type Distribution struct {
	Red, Green, Blue [256]uint64
	Pixels           uint64
}

// Count counts RGB levels of non-fully-transparent pixels.
func Count(src image.Image) *Distribution {
	if src == nil || src.Bounds().Empty() {
		return nil
	}
	result := &Distribution{}
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			pixel := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			if pixel.A == 0 {
				continue
			}
			result.Red[pixel.R]++
			result.Green[pixel.G]++
			result.Blue[pixel.B]++
			result.Pixels++
		}
	}
	return result
}

// Summary describes the pixel count and peak intensity for each channel.
func (d *Distribution) Summary() string {
	if d == nil {
		return "Нет данных гистограммы"
	}
	red, redCount := peak(d.Red)
	green, greenCount := peak(d.Green)
	blue, blueCount := peak(d.Blue)
	return fmt.Sprintf("Видимых пикселей: %d · пики R %d (%d), G %d (%d), B %d (%d)", d.Pixels, red, redCount, green, greenCount, blue, blueCount)
}

func peak(values [256]uint64) (int, uint64) {
	level, count := 0, uint64(0)
	for i, value := range values {
		if value > count {
			level, count = i, value
		}
	}
	return level, count
}

// Paint renders channel curves and level grid into dst.
func Paint(dst *image.RGBA, d *Distribution) {
	if dst == nil || dst.Bounds().Empty() {
		return
	}
	bounds := dst.Bounds()
	background := color.RGBA{R: 25, G: 29, B: 37, A: 255}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.SetRGBA(x, y, background)
		}
	}
	if d == nil || bounds.Dx() < 8 || bounds.Dy() < 8 {
		return
	}
	plot := image.Rect(bounds.Min.X+36, bounds.Min.Y+12, bounds.Max.X-12, bounds.Max.Y-24)
	if plot.Empty() {
		return
	}
	grid := color.RGBA{R: 63, G: 69, B: 81, A: 255}
	for line := 0; line <= 4; line++ {
		y := plot.Min.Y + line*(plot.Dy()-1)/4
		for x := plot.Min.X; x < plot.Max.X; x++ {
			dst.SetRGBA(x, y, grid)
		}
	}
	for _, level := range []int{0, 64, 128, 192, 255} {
		x := plot.Min.X + level*(plot.Dx()-1)/255
		for y := plot.Min.Y; y < plot.Max.Y; y++ {
			dst.SetRGBA(x, y, grid)
		}
	}
	maxCount := uint64(0)
	for i := 0; i < 256; i++ {
		maxCount = max(maxCount, d.Red[i], d.Green[i], d.Blue[i])
	}
	if maxCount == 0 {
		return
	}
	channels := [][256]uint64{d.Red, d.Green, d.Blue}
	colors := []color.RGBA{{R: 255, G: 84, B: 84, A: 255}, {R: 92, G: 224, B: 121, A: 255}, {R: 88, G: 160, B: 255, A: 255}}
	for channelIndex, values := range channels {
		previous := image.Point{}
		for level, count := range values {
			x := plot.Min.X + level*(plot.Dx()-1)/255
			ratio := float64(count) / float64(maxCount)
			y := plot.Max.Y - 1 - int(math.Round(ratio*float64(plot.Dy()-1)))
			point := image.Pt(x, y)
			if level > 0 {
				drawLine(dst, previous, point, colors[channelIndex])
			}
			previous = point
		}
	}
}

func drawLine(dst *image.RGBA, from, to image.Point, ink color.RGBA) {
	dx, dy := abs(to.X-from.X), abs(to.Y-from.Y)
	sx, sy := -1, -1
	if from.X < to.X {
		sx = 1
	}
	if from.Y < to.Y {
		sy = 1
	}
	error := dx - dy
	for {
		if image.Pt(from.X, from.Y).In(dst.Bounds()) {
			dst.SetRGBA(from.X, from.Y, ink)
		}
		if from == to {
			break
		}
		twice := 2 * error
		if twice > -dy {
			error -= dy
			from.X += sx
		}
		if twice < dx {
			error += dx
			from.Y += sy
		}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
