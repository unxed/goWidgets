package goWidgets_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/headless"
)

// bench/layout of §2.5: a layout recompute for 500 nodes must fit in 4 ms
// (p95). The window is a 10-column grid of 50 rows, every cell chained to
// its neighbours, so the system is as coupled as a real form and 500 nodes
// wide. Each iteration resizes the window and pumps one frame — the
// incremental path (edit variables + re-solve + diff), which is what a user
// dragging a corner exercises.
//
//	go test -run xxx -bench Layout -benchmem .
const benchCols, benchRows = 10, 50

// buildGrid creates the 500-node form and lays it out once.
func buildGrid(b *testing.B) *goWidgets.App {
	b.Helper()
	app, err := goWidgets.NewApp()
	if err != nil {
		b.Fatal(err)
	}
	win, err := app.NewWindow("bench", 1000, 2000)
	if err != nil {
		b.Fatal(err)
	}
	const cols, rows = benchCols, benchRows
	grid := make([][]goWidgets.Box, rows)
	for r := range grid {
		grid[r] = make([]goWidgets.Box, cols)
		for c := range grid[r] {
			l, _ := win.AddLabel("cell")
			grid[r][c] = l
		}
	}
	var cs []*goWidgets.Constraint
	cs = append(cs,
		grid[0][0].Left().Eq(win.Left().Plus(8)),
		grid[0][0].Top().Eq(win.Top().Plus(8)),
		grid[0][cols-1].Right().Eq(win.Right().Minus(8)),
	)
	for r := 0; r < rows; r++ {
		cs = append(cs, goWidgets.Row(4, grid[r]...)...)
		cs = append(cs, goWidgets.EqualWidths(grid[r]...)...)
		if r > 0 {
			cs = append(cs, goWidgets.Column(4, grid[r-1][0], grid[r][0])...)
		}
	}
	if err := win.Constrain(cs...); err != nil {
		b.Fatal(err)
	}
	for app.PumpOnce() {
	}
	if got := headless.Golden(); got == "" {
		b.Fatal("nothing laid out")
	}
	return app
}

// The first frame builds the whole system: every node's size constraints
// and every user constraint enter a fresh solver. This is the cost of
// opening a 500-node window, not of resizing it.
func BenchmarkLayoutBuild500Nodes(b *testing.B) {
	b.Setenv("goWidgets_BACKEND", "headless")
	for i := 0; i < b.N; i++ {
		buildGrid(b)
	}
}

func BenchmarkLayoutResize500Nodes(b *testing.B) {
	b.Setenv("goWidgets_BACKEND", "headless")
	app := buildGrid(b)
	const cols, rows = benchCols, benchRows

	// pumpFrame waits for the resize to cross the backend's event goroutine,
	// then runs frames until idle. Without the wait a pump sees nothing
	// pending and the loop measures an empty call.
	pumpFrame := func() {
		for !app.PumpOnce() {
			runtime.Gosched()
		}
		for app.PumpOnce() {
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		headless.Resize(1000+float64(i%2)*50, 2000)
		pumpFrame()
	}
	b.StopTimer()

	// Every iteration must have moved every column: a bench that measures a
	// frame that did nothing proves nothing.
	if lines := strings.Count(headless.Golden(), "\n") + 1; lines < b.N*cols*rows {
		b.Fatalf("only %d bounds changes for %d resizes of %d nodes", lines, b.N, cols*rows)
	}
}
