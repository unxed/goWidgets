// Package thumbnailgrid contains the platform-independent geometry and
// selection model for imgy's virtualized Canvas thumbnail browser.
package thumbnailgrid

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// FitThumbnail makes a centered, aspect-preserving nearest-neighbour preview
// no larger than width by height. The transparent padding is left for the
// Canvas card background to show through.
func FitThumbnail(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, max(0, width), max(0, height)))
	if src == nil || width <= 0 || height <= 0 || src.Bounds().Empty() {
		return dst
	}
	sb := src.Bounds()
	scale := math.Min(float64(width)/float64(sb.Dx()), float64(height)/float64(sb.Dy()))
	dw := max(1, int(math.Round(float64(sb.Dx())*scale)))
	dh := max(1, int(math.Round(float64(sb.Dy())*scale)))
	left, top := (width-dw)/2, (height-dh)/2
	for y := 0; y < dh; y++ {
		sy := sb.Min.Y + min(sb.Dy()-1, y*sb.Dy()/dh)
		for x := 0; x < dw; x++ {
			sx := sb.Min.X + min(sb.Dx()-1, x*sb.Dx()/dw)
			dst.Set(left+x, top+y, src.At(sx, sy))
		}
	}
	return dst
}

// Paint draws the visible portion of the thumbnail grid into dst. It only
// iterates the rows intersecting the viewport, not the full directory.
func Paint(dst *image.RGBA, g *Grid, thumbnails map[int]*image.RGBA, scale float64) {
	if dst == nil || g == nil {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	fill(dst, dst.Bounds(), color.RGBA{R: 34, G: 36, B: 40, A: 255})
	first, end := g.VisibleRange()
	for i := first; i < end; i++ {
		x, y, w, h, ok := g.CellRect(i)
		if !ok {
			continue
		}
		card := scaledRect(dst, x, y, w, h, scale)
		fill(dst, card, color.RGBA{R: 58, G: 61, B: 66, A: 255})
		border := color.RGBA{R: 100, G: 104, B: 110, A: 255}
		if g.IsSelected(i) {
			border = color.RGBA{R: 65, G: 150, B: 245, A: 255}
		}
		stroke(dst, card, border, max(1, int(math.Round(2*scale))))
		thumb := thumbnails[i]
		if thumb == nil {
			continue
		}
		pad := 8.0
		areaX, areaY := x+pad, y+pad
		areaW, areaH := w-2*pad, h-28
		if areaW <= 0 || areaH <= 0 {
			continue
		}
		drawThumbnail(dst, thumb, scaledRect(dst, areaX, areaY, areaW, areaH, scale))
	}
}

func drawThumbnail(dst *image.RGBA, src *image.RGBA, area image.Rectangle) {
	if src == nil || src.Bounds().Empty() || area.Empty() {
		return
	}
	dstRect := aspectRect(src.Bounds().Dx(), src.Bounds().Dy(), area)
	for y := dstRect.Min.Y; y < dstRect.Max.Y; y++ {
		sy := src.Bounds().Min.Y + (y-dstRect.Min.Y)*src.Bounds().Dy()/dstRect.Dy()
		for x := dstRect.Min.X; x < dstRect.Max.X; x++ {
			sx := src.Bounds().Min.X + (x-dstRect.Min.X)*src.Bounds().Dx()/dstRect.Dx()
			dst.Set(x, y, src.At(sx, sy))
		}
	}
}

func aspectRect(width, height int, bounds image.Rectangle) image.Rectangle {
	if width <= 0 || height <= 0 || bounds.Empty() {
		return image.Rectangle{}
	}
	scale := math.Min(float64(bounds.Dx())/float64(width), float64(bounds.Dy())/float64(height))
	w, h := max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
	return image.Rect(bounds.Min.X+(bounds.Dx()-w)/2, bounds.Min.Y+(bounds.Dy()-h)/2, bounds.Min.X+(bounds.Dx()-w)/2+w, bounds.Min.Y+(bounds.Dy()-h)/2+h)
}

func scaledRect(dst *image.RGBA, x, y, w, h, scale float64) image.Rectangle {
	r := image.Rect(int(math.Round(x*scale)), int(math.Round(y*scale)), int(math.Round((x+w)*scale)), int(math.Round((y+h)*scale)))
	return r.Intersect(dst.Bounds())
}

func fill(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

func stroke(dst *image.RGBA, r image.Rectangle, c color.RGBA, thickness int) {
	if r.Empty() {
		return
	}
	for n := 0; n < thickness && r.Dx() > n*2 && r.Dy() > n*2; n++ {
		fill(dst, image.Rect(r.Min.X+n, r.Min.Y+n, r.Max.X-n, r.Min.Y+n+1), c)
		fill(dst, image.Rect(r.Min.X+n, r.Max.Y-n-1, r.Max.X-n, r.Max.Y-n), c)
		fill(dst, image.Rect(r.Min.X+n, r.Min.Y+n, r.Min.X+n+1, r.Max.Y-n), c)
		fill(dst, image.Rect(r.Max.X-n-1, r.Min.Y+n, r.Max.X-n, r.Max.Y-n), c)
	}
}

// Grid describes a fixed-cell grid in device-independent pixels. Scroll is
// the vertical offset of the content from the top of the viewport.
type Grid struct {
	Count      int
	Width      float64
	Height     float64
	CellWidth  float64
	CellHeight float64
	Gap        float64
	Scroll     float64
	Selected   map[int]bool
	anchor     int
	hasAnchor  bool
}

// VisibleRange returns the half-open item range that intersects the viewport.
// The returned indices are clamped to [0, Count), regardless of Scroll.
func (g *Grid) VisibleRange() (first, end int) {
	cols, rowHeight := g.metrics()
	if g.Count <= 0 || cols <= 0 || rowHeight <= 0 || g.Height <= 0 {
		return 0, 0
	}
	firstRow := int(math.Floor(math.Max(0, g.Scroll) / rowHeight))
	lastRow := int(math.Ceil((math.Max(0, g.Scroll) + g.Height) / rowHeight))
	first, end = firstRow*cols, lastRow*cols
	if first > g.Count {
		first = g.Count
	}
	if end > g.Count {
		end = g.Count
	}
	return first, end
}

// HitTest maps viewport-local DIP coordinates to an item index. It returns
// false for gaps, unused cells, and points outside the viewport.
func (g *Grid) HitTest(x, y float64) (int, bool) {
	cols, rowHeight := g.metrics()
	if cols <= 0 || rowHeight <= 0 || x < 0 || y < 0 || x >= g.Width || y >= g.Height {
		return 0, false
	}
	y += math.Max(0, g.Scroll)
	stepX, stepY := g.CellWidth+g.Gap, rowHeight
	col, row := int(x/stepX), int(y/stepY)
	if col >= cols || x-float64(col)*stepX >= g.CellWidth || y-float64(row)*stepY >= g.CellHeight {
		return 0, false
	}
	index := row*cols + col
	return index, index < g.Count
}

// CellRect returns the viewport-local DIP rectangle for an item. Items beyond
// Count and invalid grid metrics have no rectangle.
func (g *Grid) CellRect(index int) (x, y, width, height float64, ok bool) {
	cols, rowHeight := g.metrics()
	if index < 0 || index >= g.Count || cols <= 0 || rowHeight <= 0 {
		return 0, 0, 0, 0, false
	}
	col, row := index%cols, index/cols
	x = float64(col) * (g.CellWidth + g.Gap)
	y = float64(row)*rowHeight - math.Max(0, g.Scroll)
	return x, y, g.CellWidth, g.CellHeight, true
}

// SetCount updates the item count, removing selections that no longer exist.
func (g *Grid) SetCount(count int) {
	if count < 0 {
		count = 0
	}
	g.Count = count
	if g.Selected == nil {
		g.Selected = make(map[int]bool)
	}
	for i := range g.Selected {
		if i < 0 || i >= count {
			delete(g.Selected, i)
		}
	}
	if g.anchor >= count {
		g.hasAnchor = false
	}
	g.ClampScroll()
}

// ClampScroll keeps the viewport within the content's vertical extent.
func (g *Grid) ClampScroll() {
	_, rowHeight := g.metrics()
	rows := 0
	if g.Count > 0 && rowHeight > 0 {
		cols, _ := g.metrics()
		rows = (g.Count + cols - 1) / cols
	}
	maxScroll := math.Max(0, float64(rows)*rowHeight-g.Height)
	g.Scroll = math.Max(0, math.Min(g.Scroll, maxScroll))
}

// Select applies a click to index. Ctrl toggles an item; Shift selects the
// inclusive range from the current anchor. Ctrl+Shift extends that range.
func (g *Grid) Select(index int, ctrl, shift bool) {
	if index < 0 || index >= g.Count {
		return
	}
	if g.Selected == nil {
		g.Selected = make(map[int]bool)
	}
	switch {
	case shift:
		if !g.hasAnchor || g.anchor < 0 || g.anchor >= g.Count {
			g.anchor = index
			g.hasAnchor = true
		}
		if !ctrl {
			clear(g.Selected)
		}
		start, end := g.anchor, index
		if start > end {
			start, end = end, start
		}
		for i := start; i <= end; i++ {
			g.Selected[i] = true
		}
	case ctrl:
		if g.Selected[index] {
			delete(g.Selected, index)
		} else {
			g.Selected[index] = true
		}
		g.anchor = index
		g.hasAnchor = true
	default:
		clear(g.Selected)
		g.Selected[index] = true
		g.anchor = index
		g.hasAnchor = true
	}
}

// IsSelected reports whether an item is in the current selection.
func (g *Grid) IsSelected(index int) bool { return g.Selected[index] }

// SelectedIndices returns a sorted copy of the selection.
func (g *Grid) SelectedIndices() []int {
	indices := make([]int, 0, len(g.Selected))
	for i := range g.Selected {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	return indices
}

func (g *Grid) metrics() (columns int, rowHeight float64) {
	stepX := g.CellWidth + g.Gap
	rowHeight = g.CellHeight + g.Gap
	if stepX <= 0 || rowHeight <= 0 || g.Width <= 0 {
		return 0, rowHeight
	}
	columns = int(math.Floor((g.Width + g.Gap) / stepX))
	if columns < 1 {
		columns = 1
	}
	return columns, rowHeight
}
