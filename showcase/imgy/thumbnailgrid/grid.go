// Package thumbnailgrid contains the platform-independent geometry and
// selection model for imgy's virtualized Canvas thumbnail browser.
package thumbnailgrid

import (
	"math"
	"sort"
)

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
