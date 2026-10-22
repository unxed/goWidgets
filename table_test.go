package goWidgets_test

import (
	"context"
	"testing"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/headless"
	"github.com/unxed/goWidgets/core"
)

func TestTableHeadless(t *testing.T) {
	t.Setenv("goWidgets_BACKEND", "headless")
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Fatal(err)
	}
	win, err := app.NewWindow("table", 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	table, err := win.AddTable([]goWidgets.TableColumn{{Title: "Name"}, {Title: "Size", Align: goWidgets.AlignRight}})
	if err != nil {
		t.Fatal(err)
	}
	if table.Fallback() || !app.Diagnostics().Caps.GridView {
		t.Fatal("headless must provide the native TableWindow path and GridView")
	}
	rows := [][]string{{"a.txt", "12"}, {"b.txt", "34"}}
	table.SetRows(rows)
	rows[0][0] = "mutated"
	gotRows := table.Rows()
	if gotRows[0][0] != "a.txt" {
		t.Fatalf("Rows did not copy input: %v", gotRows)
	}
	gotRows[0][0] = "also mutated"
	if table.Rows()[0][0] != "a.txt" {
		t.Fatal("Rows exposed internal storage")
	}
	table.SetSortIndicator(1, true)
	cols, stored, selected, sortCol, descending, ok := headless.TableState()
	if !ok || len(cols) != 2 || cols[1].Align != goWidgets.AlignRight || len(stored) != 2 || selected != -1 || sortCol != 1 || !descending {
		t.Fatalf("TableState = cols %v rows %v selected %d sort %d descending %v ok %v", cols, stored, selected, sortCol, descending, ok)
	}
	var selectedRows, activatedRows, headers []int
	table.Selected.On(app.Scope(), func(i int) { selectedRows = append(selectedRows, i) })
	table.Activated.On(app.Scope(), func(i int) { activatedRows = append(activatedRows, i) })
	table.HeaderClicked.On(app.Scope(), func(i int) { headers = append(headers, i) })
	table.Select(0)
	if !headless.PickTableRow(1) || !headless.OpenTableRow(1) || !headless.ClickTableHeader(1) {
		t.Fatal("headless synthetic table events were not accepted")
	}
	pumpUntilIdle(t, app)
	if table.SelectedIndex() != 1 || len(selectedRows) != 1 || selectedRows[0] != 1 || len(activatedRows) != 1 || activatedRows[0] != 1 || len(headers) != 1 || headers[0] != 1 {
		t.Fatalf("selection %d, Selected %v, Activated %v, HeaderClicked %v", table.SelectedIndex(), selectedRows, activatedRows, headers)
	}
}

func TestTableListBoxFallback(t *testing.T) {
	testNoGrid = nil
	core.RegisterDriver("table-test-no-grid", func() core.PlatformDriver { return &noGridDriver{} })
	t.Setenv("goWidgets_BACKEND", "table-test-no-grid")
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Fatal(err)
	}
	win, err := app.NewWindow("fallback", 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	table, err := win.AddTable([]goWidgets.TableColumn{{Title: "Name"}, {Title: "Size", Align: goWidgets.AlignRight}})
	if err != nil {
		t.Fatal(err)
	}
	if !table.Fallback() || app.Diagnostics().Caps.GridView {
		t.Fatal("driver without TableWindow must use the ListBox fallback and not advertise GridView")
	}
	table.SetRows([][]string{{"a", "2"}, {"long", "12"}})
	want := []string{"a   " + "  │  " + "   2", "long" + "  │  " + "  12"}
	if got := table.Rows(); len(got) != 2 || got[0][0] != "a" {
		t.Fatalf("Rows should retain cell data: %v", got)
	}
	if got := testNoGrid.items; !equalStrings(got, want) {
		t.Fatalf("fallback items = %q, want %q", got, want)
	}
	var selected, activated, headers []int
	table.Selected.On(app.Scope(), func(i int) { selected = append(selected, i) })
	table.Activated.On(app.Scope(), func(i int) { activated = append(activated, i) })
	table.HeaderClicked.On(app.Scope(), func(i int) { headers = append(headers, i) })
	testNoGrid.events <- core.BackendEvent{Kind: core.EventSelected, H: 2, Int: 1}
	testNoGrid.events <- core.BackendEvent{Kind: core.EventItemActivated, H: 2, Int: 1}
	pumpUntilIdle(t, app)
	if len(selected) != 1 || selected[0] != 1 || len(activated) != 1 || activated[0] != 1 || len(headers) != 0 {
		t.Fatalf("fallback events Selected %v, Activated %v, HeaderClicked %v", selected, activated, headers)
	}
	table.Select(1)
	if table.SelectedIndex() != 1 {
		t.Fatalf("SelectedIndex = %d, want 1", table.SelectedIndex())
	}
}

var testNoGrid *noGridWindow

// noGridDriver and noGridWindow keep the fake platform intentionally minimal;
// embedding the contracts makes any unexpected backend call fail loudly.
type noGridDriver struct{ core.PlatformDriver }

func (*noGridDriver) Name() string { return "table-test-no-grid" }

func (*noGridDriver) Init() error { return nil }

func (*noGridDriver) Capabilities() core.Caps { return core.Caps{} }

func (*noGridDriver) RunMainLoop(context.Context, func()) error { return nil }

func (*noGridDriver) Wake() {}

func (*noGridDriver) CreateWindow(core.WindowSpec) (core.BackendWindow, error) {
	testNoGrid = &noGridWindow{root: 1, events: make(chan core.BackendEvent), next: 1}
	return testNoGrid, nil
}

type noGridWindow struct {
	core.BackendWindow
	root, next core.Handle
	events     chan core.BackendEvent
	items      []string
}

func (*noGridWindow) SetTitle(string) {}

func (*noGridWindow) Show() {}

func (*noGridWindow) Close() {}

func (*noGridWindow) Scale() core.ScaleInfo { return core.ScaleInfo{Scale: 1, FontScale: 1} }

func (w *noGridWindow) CreateWidget(core.WidgetKind, core.Handle) (core.Handle, error) {
	w.next++
	return w.next, nil
}

func (*noGridWindow) DestroyWidget(core.Handle) {}

func (*noGridWindow) SetParent(core.Handle, core.Handle, int) {}

func (*noGridWindow) SetString(core.Handle, core.PropKey, string) {}

func (*noGridWindow) SetBool(core.Handle, core.PropKey, bool) {}

func (*noGridWindow) SetFloat(core.Handle, core.PropKey, float64) {}

func (*noGridWindow) SetInt(core.Handle, core.PropKey, int) {}

func (w *noGridWindow) SetList(_ core.Handle, _ core.PropKey, items []string) {
	w.items = append([]string(nil), items...)
}

func (*noGridWindow) MeasureIntrinsic(core.Handle, core.Size) (core.Size, core.Size) {
	return core.Size{}, core.Size{}
}

func (*noGridWindow) ApplyLayout([]core.BoundsChange) {}

func (*noGridWindow) Focus(core.Handle) {}

func (*noGridWindow) Dialog(core.DialogKind, string, string) core.DialogResult {
	return core.DialogCancel
}

func (*noGridWindow) FileDialog(bool, string, string, []core.FileFilter) (string, bool) {
	return "", false
}

func (w *noGridWindow) RootHandle() core.Handle { return w.root }

func (w *noGridWindow) Events() <-chan core.BackendEvent { return w.events }

func (*noGridDriver) Shutdown() {}

func (*noGridDriver) CreateTray(core.TraySpec) (core.BackendTray, error) {
	return nil, core.ErrNoTray
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
