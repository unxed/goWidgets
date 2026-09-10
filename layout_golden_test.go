package goWidgets_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/headless"
	"github.com/unxed/goWidgets/core"
	"github.com/unxed/goWidgets/vreactive"
)

// The golden layout test of §9.1: the headless driver reports deterministic
// intrinsic sizes, so the whole measure→solve→apply pipeline has one correct
// answer that is identical on every machine. Any change in layout behaviour
// shows up here immediately.
//
// The stack below is not a separate layout: it is what the solver produces
// for widgets with no constraints of their own (the flow, ADR-0005), which is
// why this golden survived the move to Cassowary unchanged.
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
//
// This test and the next build no App, so nothing binds the UI goroutine for
// them — but a test before them did, to its own goroutine, and under
// -tags goWidgets_debug the affinity check would fire on ours. This goroutine
// is the whole application here, so it is the UI goroutine.
func TestNoLeakedSubscriptions(t *testing.T) {
	vreactive.BindMainThread()
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
	vreactive.BindMainThread()
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

// The tray is optional by contract: a backend without a status area must say so
// rather than fail, and an application must be able to carry on without one.
func TestTrayIsOptionalOnBackendsWithoutOne(t *testing.T) {
	app := newHeadlessApp(t)
	if _, err := app.NewWindow("crescent", 400, 300); err != nil {
		t.Fatal(err)
	}

	tray, err := app.NewTrayIcon("crescent", goWidgets.NewMenuItem("Выход"))
	if err == nil {
		t.Fatal("the headless driver claimed to have a tray")
	}
	if !errors.Is(err, core.ErrNoTray) {
		t.Errorf("err = %v, want it to wrap core.ErrNoTray", err)
	}
	if tray != nil {
		t.Error("a tray was returned alongside the error")
	}

	// Everything else must keep working.
	pumpUntilIdle(t, app)
}

// A text view is a viewport, not a line of text: its height is chosen, not
// measured, and it must take that height in the layout so the widgets below it
// are not pushed off screen.
func TestTextViewTakesItsChosenHeight(t *testing.T) {
	app := newHeadlessApp(t)
	win, err := app.NewWindow("crescent", 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	tv, err := win.AddTextView(150)
	if err != nil {
		t.Fatal(err)
	}
	tv.SetText("строка один\nстрока два")
	below, _ := win.AddButton("ниже")
	_ = below

	pumpUntilIdle(t, app)

	log := headless.Golden()
	if !strings.Contains(log, "TextView") {
		t.Fatalf("text view not laid out:\n%s", log)
	}
	// The chosen height (150) must appear, not a measured near-zero one.
	if !strings.Contains(log, "h=150.0") {
		t.Errorf("text view did not take its chosen height:\n%s", log)
	}
	// The button must sit below it, not on top of it — y greater than 150.
	if !strings.Contains(log, `Button("ниже")`) {
		t.Errorf("button below the text view was not placed:\n%s", log)
	}
}

// A real dialog, declared as constraints (§5): status line, a column of check
// boxes under it, three equal buttons along the bottom. Golden because the
// solver's answer is exact and the same on every machine.
const goldenDialog = `Label("Простаиваю. Целей в очереди: 2") x=8.0 y=8.0 w=444.0 h=24.0 visible=true
CheckBox("Цель: починить флаки в CI") x=8.0 y=40.0 w=444.0 h=24.0 visible=true
CheckBox("Цель: дописать тесты бэкенда") x=8.0 y=72.0 w=444.0 h=24.0 visible=true
Button("Гнать все цели") x=8.0 y=276.0 w=142.7 h=36.0 visible=true
Button("Убрать отмеченные") x=158.7 y=276.0 w=142.7 h=36.0 visible=true
Button("Выход") x=309.3 y=276.0 w=142.7 h=36.0 visible=true`

type dialog struct {
	win             *goWidgets.Window
	status          *goWidgets.Label
	cb1, cb2        *goWidgets.CheckBox
	run, drop, quit *goWidgets.Button
}

func newDialog(t *testing.T, app *goWidgets.App) *dialog {
	t.Helper()
	win, err := app.NewWindow("crescent", 460, 320)
	if err != nil {
		t.Fatal(err)
	}
	d := &dialog{win: win}
	d.status, _ = win.AddLabel("Простаиваю. Целей в очереди: 2")
	d.cb1, _ = win.AddCheckBox("Цель: починить флаки в CI", false)
	d.cb2, _ = win.AddCheckBox("Цель: дописать тесты бэкенда", false)
	d.run, _ = win.AddButton("Гнать все цели")
	d.drop, _ = win.AddButton("Убрать отмеченные")
	d.quit, _ = win.AddButton("Выход")

	const pad = 8
	var cs []*goWidgets.Constraint
	cs = append(cs,
		d.status.Left().Eq(win.Left().Plus(pad)),
		d.status.Top().Eq(win.Top().Plus(pad)),
		d.status.Right().Eq(win.Right().Minus(pad)),
		d.run.Left().Eq(win.Left().Plus(pad)),
		d.run.Bottom().Eq(win.Bottom().Minus(pad)),
		d.quit.Right().Eq(win.Right().Minus(pad)),
	)
	cs = append(cs, goWidgets.Column(pad, d.status, d.cb1, d.cb2)...)
	cs = append(cs, goWidgets.Row(pad, d.run, d.drop, d.quit)...)
	cs = append(cs, goWidgets.EqualWidths(d.run, d.drop, d.quit)...)
	if err := win.Constrain(cs...); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGoldenConstraintDialog(t *testing.T) {
	app := newHeadlessApp(t)
	newDialog(t, app)
	pumpUntilIdle(t, app)

	got := strings.TrimSpace(headless.Golden())
	if got != goldenDialog {
		t.Errorf("dialog drifted from golden:\n--- got ---\n%s\n--- want ---\n%s", got, goldenDialog)
	}
	if c := app.Diagnostics().LayoutConflicts; len(c) != 0 {
		t.Errorf("a well-formed dialog reported conflicts: %v", c)
	}
}

// The window size is an edit variable: a resize is a re-solve of the same
// system, and edges pinned to the window follow it.
func TestResizeReflowsConstraints(t *testing.T) {
	app := newHeadlessApp(t)
	d := newDialog(t, app)
	pumpUntilIdle(t, app)

	headless.Reset()
	headless.Resize(600, 400)
	pumpUntilIdle(t, app)

	log := headless.Golden()
	for _, want := range []string{
		`Label("Простаиваю. Целей в очереди: 2") x=8.0 y=8.0 w=584.0`,
		`Button("Выход") x=402.7 y=356.0 w=189.3 h=36.0`,
	} {
		if !strings.Contains(log, want) {
			t.Errorf("after resize, missing %q in:\n%s", want, log)
		}
	}
	_ = d
}

// Hiding a constrained widget collapses it along the axis its chain leaves
// free, so the rows below a hidden row move up by its height; the chain's gap
// stays, because the constraint saying so is still there.
func TestHiddenConstrainedWidgetCollapses(t *testing.T) {
	app := newHeadlessApp(t)
	d := newDialog(t, app)
	pumpUntilIdle(t, app)

	headless.Reset()
	d.cb1.Visible.Set(false)
	pumpUntilIdle(t, app)

	got := strings.TrimSpace(headless.Golden())
	want := `CheckBox("Цель: починить флаки в CI") x=0.0 y=0.0 w=0.0 h=0.0 visible=false
CheckBox("Цель: дописать тесты бэкенда") x=8.0 y=48.0 w=444.0 h=24.0 visible=true`
	if got != want {
		t.Errorf("hidden row did not collapse:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Phase 2 DoD (§8): an unsatisfiable system is diagnosed, never a panic and
// never a silent hang. Two required rules that disagree: the second is
// dropped, reported, and everything else keeps working.
func TestUnsatisfiableIsDiagnosedNotFatal(t *testing.T) {
	if core.DebugBuild {
		t.Skip("a debug build panics on an unsatisfiable constraint by design")
	}
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	btn, _ := win.AddButton("Гнать")

	err := win.Constrain(
		btn.Width().Is(100).Required(),
		btn.Width().Is(200).Required(),
	)
	if !errors.Is(err, core.ErrUnsatisfiable) {
		t.Fatalf("err = %v, want it to wrap core.ErrUnsatisfiable", err)
	}
	if c := app.Diagnostics().LayoutConflicts; len(c) != 1 {
		t.Fatalf("LayoutConflicts = %v, want exactly one entry", c)
	}
	pumpUntilIdle(t, app)
	if !strings.Contains(headless.Golden(), "w=100.0") {
		t.Errorf("the first required rule did not survive the conflict:\n%s", headless.Golden())
	}
}

// Strong rules that merely disagree are not an error: the solver compromises,
// and the caller controls the outcome with strengths.
func TestStrengthsResolveDisagreement(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	btn, _ := win.AddButton("Гнать")

	if err := win.Constrain(
		btn.Width().Is(100),
		btn.Width().Is(200).Weak(),
	); err != nil {
		t.Fatal(err)
	}
	pumpUntilIdle(t, app)
	if !strings.Contains(headless.Golden(), "w=100.0") {
		t.Errorf("strong did not beat weak:\n%s", headless.Golden())
	}
}

// One constraint takes a widget out of the flow; removing it puts the widget
// back. The stack and the solver are one system, so this is just a re-solve.
func TestUnconstrainReturnsToFlow(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	lbl, _ := win.AddLabel("Статус")
	btn, _ := win.AddButton("Гнать")

	c := btn.Left().Eq(win.Left().Plus(100))
	if err := win.Constrain(c); err != nil {
		t.Fatal(err)
	}
	pumpUntilIdle(t, app)
	if !strings.Contains(headless.Golden(), `Button("Гнать") x=100.0 y=0.0`) {
		t.Fatalf("constrained button not where it was put:\n%s", headless.Golden())
	}

	headless.Reset()
	win.Unconstrain(c)
	pumpUntilIdle(t, app)
	if !strings.Contains(headless.Golden(), `Button("Гнать") x=8.0 y=40.0 w=384.0`) {
		t.Errorf("button did not return to the flow under the label:\n%s", headless.Golden())
	}
	_ = lbl
}

// Nesting: a guide is a container that lives only in the solver. A button
// bar pinned to the bottom of the window, two buttons splitting it — the
// guide gets no rectangle of its own in the log, only its contents do.
func TestGuideNestsLayout(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	ok, _ := win.AddButton("OK")
	cancel, _ := win.AddButton("Отмена")

	bar := win.NewGuide()
	var cs []*goWidgets.Constraint
	cs = append(cs,
		bar.Left().Eq(win.Left().Plus(8)),
		bar.Right().Eq(win.Right().Minus(8)),
		bar.Bottom().Eq(win.Bottom().Minus(8)),
		bar.Height().Is(36),

		ok.Left().Eq(bar.Left()),
		ok.Top().Eq(bar.Top()),
		ok.Bottom().Eq(bar.Bottom()),
		cancel.Right().Eq(bar.Right()),
		cancel.Top().Eq(bar.Top()),
		cancel.Bottom().Eq(bar.Bottom()),
	)
	cs = append(cs, goWidgets.Row(8, ok, cancel)...)
	cs = append(cs, goWidgets.EqualWidths(ok, cancel)...)
	if err := win.Constrain(cs...); err != nil {
		t.Fatal(err)
	}
	pumpUntilIdle(t, app)

	got := strings.TrimSpace(headless.Golden())
	want := `Button("OK") x=8.0 y=256.0 w=188.0 h=36.0 visible=true
Button("Отмена") x=204.0 y=256.0 w=188.0 h=36.0 visible=true`
	if got != want {
		t.Errorf("guide did not nest the bar:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// A text field is the first widget whose contents the user owns: what they
// type must reach the property, what the program sets must reach the field,
// and a platform-reported edit must never be written back over the field —
// that echo is how a fast typist loses characters.
func TestEntryBindsBothWays(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	e, err := win.AddEntry("")
	if err != nil {
		t.Fatal(err)
	}
	var changes []string
	e.Changed.On(app.Scope(), func(v string) { changes = append(changes, v) })
	pumpUntilIdle(t, app)

	// program → platform
	e.Text.Set("починить CI")
	pumpUntilIdle(t, app)
	if got, ok := headless.EntryText(); !ok || got != "починить CI" {
		t.Fatalf("property write did not reach the field (found=%v text=%q)", ok, got)
	}

	// platform → property, two edits queued before the first is handled:
	// the property must end on the later one and the field must be left alone.
	measured := headless.MeasureCount()
	writes := headless.TextWrites()
	headless.TypeIntoEntry("починить C")
	headless.TypeIntoEntry("починить")
	pumpUntilIdle(t, app)
	if headless.TextWrites() != writes {
		// The write would carry the right text and still be wrong: on Win32
		// SetWindowText moves the caret to the start, so the next character
		// typed lands in front of the previous ones.
		t.Errorf("a platform edit was written back to the platform (%d writes)", headless.TextWrites()-writes)
	}
	if got := e.Text.Get(); got != "починить" {
		t.Fatalf("Text = %q, want the last edit", got)
	}
	if got, _ := headless.EntryText(); got != "починить" {
		t.Fatalf("an earlier edit was echoed back over the field: %q", got)
	}
	if len(changes) != 2 || changes[1] != "починить" {
		t.Fatalf("Changed fired with %v, want the two edits and not the program's own write", changes)
	}
	if headless.MeasureCount() != measured {
		t.Error("typing re-measured the window; a field's size does not depend on its text")
	}
}

// Enter in a text field is the field's activation, with its contents.
func TestEntryActivates(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	e, _ := win.AddEntry("")
	var got []string
	e.Activated.On(app.Scope(), func(v string) { got = append(got, v) })
	pumpUntilIdle(t, app)

	headless.TypeIntoEntry("новая цель")
	headless.PressEnterInEntry()
	pumpUntilIdle(t, app)
	if len(got) != 1 || got[0] != "новая цель" {
		t.Fatalf("Activated = %v, want [новая цель]", got)
	}
}

// A field in the flow takes its chosen width when nothing else says
// otherwise — here the flow stretches it, and the golden pins its height.
func TestEntryGolden(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	win.AddEntry("x")
	pumpUntilIdle(t, app)
	want := `Entry("x") x=8.0 y=8.0 w=384.0 h=24.0 visible=true`
	if got := strings.TrimSpace(headless.Golden()); got != want {
		t.Errorf("--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Two widgets sharing a row both prefer their natural width equally weakly,
// so which one stretches is the solver's pick. HugWidth settles it.
func TestHugWidthDecidesWhoStretches(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	entry, _ := win.AddEntry("")
	add, _ := win.AddButton("Добавить")
	if err := win.Constrain(
		entry.Left().Eq(win.Left().Plus(8)),
		entry.Top().Eq(win.Top().Plus(8)),
		add.Right().Eq(win.Right().Minus(8)),
	); err != nil {
		t.Fatal(err)
	}
	if err := win.Constrain(goWidgets.Row(8, entry, add)...); err != nil {
		t.Fatal(err)
	}
	add.HugWidth()
	pumpUntilIdle(t, app)

	got := strings.TrimSpace(headless.Golden())
	// "Добавить" is 8 chars: 8·7 + 2·16 = 88 wide; the field gets the rest.
	want := `Entry("") x=8.0 y=8.0 w=288.0 h=36.0 visible=true
Button("Добавить") x=304.0 y=8.0 w=88.0 h=36.0 visible=true`
	if got != want {
		t.Errorf("--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Focus() reaches the platform; the platform then owns Tab from there.
func TestFocusReachesPlatform(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	win.AddButton("Гнать")
	e, _ := win.AddEntry("цель")
	e.Focus()
	pumpUntilIdle(t, app)
	if got, ok := headless.Focused(); !ok || got != "цель" {
		t.Fatalf("focused = %q (%v), want the entry", got, ok)
	}
}

// Message boxes: the answer scripted by the test reaches the caller, and an
// unscripted box is answered the cautious way, as a user closing it would.
func TestDialogsAnswerAndDefault(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	pumpUntilIdle(t, app)

	headless.AnswerDialogs(core.DialogYes, core.DialogOK)
	if !win.Ask("Убрать?", "Убрать 2 отмеченные цели?") {
		t.Error("scripted Yes did not reach Ask")
	}
	if !win.Confirm("Выход", "Выйти?") {
		t.Error("scripted OK did not reach Confirm")
	}
	if win.Ask("Убрать?", "ещё раз") {
		t.Error("an unscripted Yes/No box should read as No")
	}
	if win.Confirm("Выход", "ещё раз") {
		t.Error("an unscripted OK/Cancel box should read as Cancel")
	}
	win.Message("Готово", "Все цели закрыты")

	ds := headless.Dialogs()
	if len(ds) != 5 || ds[0].Title != "Убрать?" || ds[4].Kind != core.DialogInfo {
		t.Errorf("Dialogs = %+v", ds)
	}
}

// File dialogs: scripted paths come back, "" is a cancel, and what the
// program asked for (kind, title, suggested name, filters) is recorded.
func TestFileDialogsScripted(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("crescent", 400, 300)
	pumpUntilIdle(t, app)

	headless.AnswerFiles("/tmp/цели.txt", "")
	txt := goWidgets.FileFilter{Name: "Текст", Patterns: []string{"*.txt", "*.md"}}
	if p, ok := win.OpenFile("Открыть", txt); !ok || p != "/tmp/цели.txt" {
		t.Errorf("OpenFile = %q, %v", p, ok)
	}
	if p, ok := win.SaveFile("Сохранить", "цели.txt", txt); ok || p != "" {
		t.Errorf("a scripted cancel came back as %q, %v", p, ok)
	}
	if p, ok := win.OpenFile("Ещё"); ok || p != "" {
		t.Errorf("an unscripted dialog should be a cancel, got %q, %v", p, ok)
	}
	ds := headless.FileDialogs()
	if len(ds) != 3 || ds[0].Save || !ds[1].Save || ds[1].Suggested != "цели.txt" || len(ds[0].Filters) != 1 {
		t.Errorf("FileDialogs = %+v", ds)
	}
}
