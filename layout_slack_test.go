package goWidgets_test

import (
	"strings"
	"testing"

	"github.com/unxed/goWidgets/backends/headless"
)

// The imgy browser, as a regression: in a column with slack, it goes to the
// list and preview, not to a one-line field nor to a label that hugs its height.
// Before one-line controls held
// their natural height at medium, the text field came out 444 pixels tall.
func TestColumnSlackGoesToTheList(t *testing.T) {
	app := newHeadlessApp(t)
	win, _ := app.NewWindow("imgy", 1040, 680)
	title, _ := win.AddLabel("imgy — image browser")
	pathEdit, _ := win.AddEdit("/tmp")
	open, _ := win.AddButton("Открыть папку")
	files, _ := win.AddListBox(nil)
	preview, _ := win.AddImageView()
	preview.SetPath("selected.png")
	info, _ := win.AddLabel("Файлов: 0")
	fullscreen, _ := win.AddButton("Полный экран")
	status, _ := win.AddLabel("Этап 1")
	const pad = 12
	err := win.Constrain(
		title.Left().Eq(win.Left().Plus(pad)),
		title.Top().Eq(win.Top().Plus(pad)),
		title.Right().Eq(win.Right().Minus(pad)),
		pathEdit.Left().Eq(title.Left()),
		pathEdit.Top().Eq(title.Bottom().Plus(pad)),
		open.Right().Eq(title.Right()),
		open.Top().Eq(pathEdit.Top()),
		pathEdit.Right().Eq(open.Left().Minus(pad)),
		files.Left().Eq(title.Left()),
		files.Top().Eq(pathEdit.Bottom().Plus(pad)),
		files.Width().Is(280),
		files.Bottom().Eq(status.Top().Minus(pad)),
		preview.Left().Eq(files.Right().Plus(pad)),
		preview.Top().Eq(files.Top()),
		preview.Right().Eq(title.Right()),
		preview.Bottom().Eq(status.Top().Minus(pad)),
		info.Left().Eq(preview.Left()),
		info.Bottom().Eq(preview.Bottom().Minus(pad)),
		fullscreen.Right().Eq(preview.Right()),
		fullscreen.Bottom().Eq(preview.Bottom().Minus(pad)),
		status.Left().Eq(title.Left()),
		status.Right().Eq(title.Right()),
		status.Bottom().Eq(win.Bottom().Minus(pad)),
	)
	if err != nil {
		t.Fatal(err)
	}
	info.HugHeight()
	fullscreen.HugWidth()
	open.HugWidth()
	title.HugHeight()
	status.HugHeight()
	pumpUntilIdle(t, app)

	got := headless.Golden()
	for _, want := range []string{
		`Edit("/tmp") x=12.0 y=48.0 w=881.0 h=24.0`,
		`ListBox("") x=12.0 y=84.0 w=280.0 h=548.0`,
		`Label("Этап 1") x=12.0 y=644.0 w=1016.0 h=24.0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if c := app.Diagnostics().LayoutConflicts; len(c) != 0 {
		t.Errorf("conflicts: %v", c)
	}
}
