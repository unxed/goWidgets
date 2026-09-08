package goWidgets_test

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/headless"
	"github.com/unxed/goWidgets/vreactive"
)

// The golden layout test of §9.1: the headless driver reports deterministic
// intrinsic sizes, so the whole measure→solve→apply pipeline has one correct
// answer that is identical on every machine. Any change in layout behaviour
// shows up here immediately.
const goldenStack = `Label("Статус: простаиваю") x=8.0 y=8.0 w=384.0 h=24.0 visible=true
Button("Гнать все цели") x=8.0 y=40.0 w=384.0 h=36.0 visible=true
Button("Выход") x=8.0 y=84.0 w=384.0 h=36.0 visible=true`

func newHeadlessApp(t *testing.T) *goWidgets.App {
	t.Helper()
	t.Setenv("goWidgets_BACKEND", "headless")
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if got := app.Diagnostics().Name; got != "headless" {
		t.Fatalf("driver = %q, want headless", got)
	}
	return app
}

func TestGoldenStackLayout(t *testing.T) {
	app := newHeadlessApp(t)
	win, err := app.NewWindow("crescent", 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := win.AddLabel("Статус: простаиваю"); err != nil {
		t.Fatal(err)
	}
	if _, err := win.AddButton("Гнать все цели"); err != nil {
		t.Fatal(err)
	}
	if _, err := win.AddButton("Выход"); err != nil {
		t.Fatal(err)
	}

	pumpUntilIdle(t, app)

	got := strings.TrimSpace(headless.Golden())
	if got != goldenStack {
		t.Errorf("layout drifted from golden:\n--- got ---\n%s\n--- want ---\n%s", got, goldenStack)
	}
}

// A text change must invalidate the cached intrinsic size. Asserting on
// rectangles would be wrong here: in a full-width stack neither the width nor
// the height of a row depends on its text, so a correct implementation emits no
// BoundsChange at all. What must happen is the re-measure itself.
func TestTextChangeRemeasures(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	lbl, _ := win.AddLabel("x")
	pumpUntilIdle(t, app)

	before := headless.MeasureCount()
	lbl.Text.Set("значительно более длинный текст статуса")
	pumpUntilIdle(t, app)

	if after := headless.MeasureCount(); after <= before {
		t.Fatalf("text change did not re-measure (%d → %d)", before, after)
	}
}

// Hiding a row must take it out of the flow, so everything below moves up.
func TestHiddenWidgetLeavesTheStack(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	lbl, _ := win.AddLabel("Статус")
	btn, _ := win.AddButton("Гнать")
	pumpUntilIdle(t, app)
	_ = btn

	headless.Reset()
	lbl.Visible.Set(false)
	pumpUntilIdle(t, app)

	got := strings.TrimSpace(headless.Golden())
	want := `Label("Статус") x=0.0 y=0.0 w=0.0 h=0.0 visible=false
Button("Гнать") x=8.0 y=8.0 w=384.0 h=36.0 visible=true`
	if got != want {
		t.Errorf("hiding a row did not reflow the stack:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Clicking must reach the user's handler through the backend event channel and
// QueueUpdate, landing on the UI thread.
func TestClickReachesHandler(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	btn, _ := win.AddButton("Гнать")

	clicks := 0
	btn.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { clicks++ })
	pumpUntilIdle(t, app)

	if !headless.ClickByText("Гнать") {
		t.Fatal("no widget with that caption")
	}
	pumpUntilIdle(t, app)

	if clicks != 1 {
		t.Fatalf("clicks = %d, want 1", clicks)
	}
}

// NFR "Утечки подписок": closing a scope must drop every subscription.
func TestNoLeakedSubscriptions(t *testing.T) {
	scope := vreactive.NewScope()
	p := vreactive.NewProperty("a")
	e := vreactive.NewEvent[int]()

	for i := 0; i < 10; i++ {
		p.OnChange(scope, func(string, string) {})
		e.On(scope, func(int) {})
	}
	if scope.Len() != 20 {
		t.Fatalf("scope holds %d subscriptions, want 20", scope.Len())
	}

	scope.Close()
	if scope.Len() != 0 {
		t.Fatalf("scope still holds %d subscriptions after Close", scope.Len())
	}

	fired := 0
	p.OnChange(scope, func(string, string) { fired++ }) // into a closed scope
	p.Set("b")
	if fired != 0 {
		t.Fatalf("subscription into a closed scope still fired %d times", fired)
	}
}

// Batch must be glitch-free: subscribers observe the committed state once.
func TestBatchIsGlitchFree(t *testing.T) {
	scope := vreactive.NewScope()
	defer scope.Close()

	a := vreactive.NewProperty(1)
	b := vreactive.NewProperty(1)

	var seen []int
	a.OnChange(scope, func(int, int) { seen = append(seen, a.Get()+b.Get()) })

	vreactive.Batch(func() {
		a.Set(10)
		b.Set(20)
	})

	if len(seen) != 1 || seen[0] != 30 {
		t.Fatalf("observed %v, want exactly [30] — a half-applied batch leaked", seen)
	}
}

// pumpUntilIdle runs queued updates and frames the way a real main loop would,
// without entering a blocking native loop.
func pumpUntilIdle(t *testing.T, app *goWidgets.App) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	idle := 0
	for time.Now().Before(deadline) {
		if app.PumpOnce() {
			idle = 0
			continue
		}
		// Backend events reach the queue through a helper goroutine, so a
		// single empty pump does not prove quiescence.
		idle++
		if idle >= 3 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("pipeline never went idle")
}

// Two-way binding is the risky part of any property-based API: the platform
// owns the check box's state, so a toggle must travel backend → property, while
// a property write must travel property → backend, and neither direction may
// echo back into the other.
func TestCheckBoxBindsBothWays(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	cb, err := win.AddCheckBox("Цель: починить CI", false)
	if err != nil {
		t.Fatal(err)
	}
	toggles := 0
	cb.Toggled.On(app.Scope(), func(bool) { toggles++ })
	pumpUntilIdle(t, app)

	// property → platform
	cb.Checked.Set(true)
	pumpUntilIdle(t, app)
	if got, found := headless.CheckedByText("Цель: починить CI"); !found || !got {
		t.Fatalf("property write did not reach the platform (found=%v checked=%v)", found, got)
	}

	// platform → property
	if !headless.ToggleByText("Цель: починить CI") {
		t.Fatal("no such check box")
	}
	pumpUntilIdle(t, app)
	if cb.Checked.Get() {
		t.Fatal("user toggle did not reach the property")
	}
	if toggles != 1 {
		t.Fatalf("Toggled fired %d times, want 1", toggles)
	}

	// and the echo did not loop back and flip the platform again
	if got, _ := headless.CheckedByText("Цель: починить CI"); got {
		t.Fatal("state echoed back to the platform — binding loop")
	}
}
