//go:build linux

package qttest

import (
	"testing"
	"time"

	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
)

// TextView: the text reaches the view whole, the view is read-only, and it
// takes the height the program asked for while the width follows the window.
func TestTextView(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	win, _ := app.NewWindow("log", 400, 300)
	tv, err := win.AddTextView(120)
	if err != nil {
		t.Fatal(err)
	}
	const log = "12:00 codex   старт\n12:01 codex   лимит, ждём до 17:00"
	tv.SetText(log)

	var got string
	var readOnly bool
	var g rect
	drive(app, 300*time.Millisecond, func() {
		h := qt.WidgetHandle(core.KindTextView)
		// toPlainText() is inline; the document's is exported.
		doc := qt.Call(qt.Sym("_ZNK14QPlainTextEdit8documentEv"), h)
		got = qt.CallString(qt.Sym("_ZNK13QTextDocument11toPlainTextEv"), doc)
		readOnly = qt.Call(qt.Sym("_ZNK14QPlainTextEdit10isReadOnlyEv"), h)&0xff != 0
		g = geometry(h)
	})
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if got != log {
		t.Errorf("text = %q", got)
	}
	if !readOnly {
		t.Error("the view is editable")
	}
	if g.H != 120 || g.W != 400-16 {
		t.Errorf("geometry %+v, want 384×120", g)
	}
}
