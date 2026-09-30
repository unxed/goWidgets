package thumbnailgrid

import (
	"reflect"
	"testing"
)

func TestVisibleRangeVirtualizesRows(t *testing.T) {
	g := Grid{Count: 10_000, Width: 520, Height: 260, CellWidth: 120, CellHeight: 90, Gap: 10, Scroll: 1_005}
	first, end := g.VisibleRange()
	if first != 40 || end != 52 {
		t.Fatalf("VisibleRange() = [%d,%d), want [40,52)", first, end)
	}
	if end-first >= g.Count {
		t.Fatalf("visible range is not virtualized: [%d,%d)", first, end)
	}
}

func TestHitTestAccountsForScrollAndGaps(t *testing.T) {
	g := Grid{Count: 20, Width: 260, Height: 180, CellWidth: 120, CellHeight: 80, Gap: 10, Scroll: 90}
	for _, tc := range []struct {
		name string
		x, y float64
		want int
		ok   bool
	}{
		{"scrolled first row", 12, 12, 2, true},
		{"horizontal gap", 125, 12, 0, false},
		{"vertical gap", 12, 85, 0, false},
		{"outside", 260, 12, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := g.HitTest(tc.x, tc.y)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("HitTest(%v,%v) = %d,%v; want %d,%v", tc.x, tc.y, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCellRectUsesViewportCoordinates(t *testing.T) {
	g := Grid{Count: 20, Width: 260, Height: 180, CellWidth: 120, CellHeight: 80, Gap: 10, Scroll: 90}
	x, y, w, h, ok := g.CellRect(2)
	if !ok || x != 0 || y != 10 || w != 120 || h != 80 {
		t.Fatalf("CellRect(2) = %v,%v %vx%v %v; want 0,10 120x80 true", x, y, w, h, ok)
	}
	if _, _, _, _, ok := g.CellRect(20); ok {
		t.Fatal("CellRect should reject an index past Count")
	}
}

func TestSingleControlAndRangeSelection(t *testing.T) {
	initial := Grid{}
	initial.SetCount(12)
	initial.Select(4, false, true)
	if got, want := initial.SelectedIndices(), []int{4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shift click without anchor = %v, want %v", got, want)
	}

	g := Grid{}
	g.SetCount(12)
	g.Select(2, false, false)
	g.Select(5, true, false)
	g.Select(8, false, true)
	if got, want := g.SelectedIndices(), []int{5, 6, 7, 8}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shift selection = %v, want %v", got, want)
	}
	g.Select(6, true, false)
	if g.IsSelected(6) {
		t.Fatal("Ctrl-click should toggle the selected item off")
	}
	g.Select(1, false, false)
	if got, want := g.SelectedIndices(), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plain click selection = %v, want %v", got, want)
	}
}

func TestCountChangeClampsSelectionAndScroll(t *testing.T) {
	g := Grid{Width: 200, Height: 100, CellWidth: 90, CellHeight: 80, Gap: 10, Scroll: 5_000}
	g.SetCount(100)
	g.Select(99, false, false)
	g.SetCount(2)
	if got, want := g.SelectedIndices(), []int{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection after shrinking list = %v, want %v", got, want)
	}
	if g.Scroll != 0 {
		t.Fatalf("Scroll after shrinking content = %v, want 0", g.Scroll)
	}
}
