// Package goWidgets is the public API. Users see only this package: no build
// tags, no platform types, no Handles.
//
// Contract: §4.3 of docs/disdoc.md.
//
//	app, _ := goWidgets.NewApp()
//	win, _ := app.NewWindow("Title", 420, 240)
//	b := win.AddButton("Run")
//	b.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { … })
//	app.Run(win)
package goWidgets

import (
	"time"

	"github.com/unxed/goWidgets/core"
	"github.com/unxed/goWidgets/vreactive"
)

// Geometry types are re-exported so users never import core.
type (
	// Size is an extent in device-independent pixels.
	Size = core.Size
	// Rect is a rectangle in device-independent pixels.
	Rect = core.Rect
	// ScaleInfo describes a window's DPI scaling.
	ScaleInfo = core.ScaleInfo
	// DriverInfo answers "why am I not seeing native controls?".
	DriverInfo = core.DriverInfo
	// Scope owns subscriptions.
	Scope = vreactive.Scope
)

// ClickInfo carries the details of a click. Empty in iteration 1; it exists so
// that adding modifier keys later is not a breaking API change.
type ClickInfo struct{}

// CloseRequest is passed to Window.Closing. Set Cancel to keep the window open
// — the idiom for minimising to tray.
type CloseRequest struct{ Cancel bool }

// Cancel vetoes the close.
func (c *CloseRequest) CancelClose() { c.Cancel = true }

// App owns the driver, the window tree and the update queue.
type App struct {
	eng   *core.App
	scope *vreactive.Scope
}

// NewApp selects and initialises a backend (§3.3) and pins the UI thread.
func NewApp() (*App, error) {
	eng, err := core.NewApp()
	if err != nil {
		return nil, err
	}
	return &App{eng: eng, scope: eng.Scope()}, nil
}

// Scope is the application-lifetime subscription owner.
func (a *App) Scope() *Scope { return a.scope }

// Diagnostics reports the chosen driver and everything that was tried.
func (a *App) Diagnostics() DriverInfo { return a.eng.Diagnostics() }

// QueueUpdate runs f on the UI thread. Safe from any goroutine (§4.5.3).
func (a *App) QueueUpdate(f func()) { a.eng.QueueUpdate(f) }

// Cancel stops a pending Post.
type Cancel func()

// Post runs f on the UI thread after d.
func (a *App) Post(d time.Duration, f func()) Cancel {
	t := time.AfterFunc(d, func() { a.eng.QueueUpdate(f) })
	return func() { t.Stop() }
}

// Run shows the window and enters the native event loop. It returns when the
// application quits.
func (a *App) Run(main *Window) error {
	// Windows are created hidden so that the first frame is measured and laid
	// out before anything appears on screen; Run is what puts the main window
	// up. The condition reads "still hidden", not "already visible".
	if main != nil && main.hidden {
		main.Show()
	}
	return a.eng.Run()
}

// Quit leaves the event loop.
func (a *App) Quit() { a.eng.Quit() }

// PumpOnce advances the pipeline by one step without entering the native event
// loop, reporting whether anything happened. Intended for tests and for hosts
// that drive their own loop; a normal application calls Run instead.
func (a *App) PumpOnce() bool { return a.eng.PumpOnce() }

// Window is a native top-level window holding a vertical stack of widgets.
type Window struct {
	app     *App
	Title   *vreactive.Property[string]
	Closing *vreactive.Event[*CloseRequest]
	keys    *vreactive.Event[Key]
	hidden  bool
}

// NewWindow creates the application window, w×h in device-independent pixels.
func (a *App) NewWindow(title string, w, h float64) (*Window, error) {
	if err := a.eng.OpenWindow(core.WindowSpec{
		Title:  title,
		Size:   Size{W: w, H: h},
		Hidden: true,
	}); err != nil {
		return nil, err
	}
	win := &Window{
		app:     a,
		keys:    vreactive.NewEvent[Key](),
		Title:   vreactive.NewProperty(title),
		Closing: vreactive.NewEvent[*CloseRequest](),
		hidden:  true,
	}
	win.Title.OnChange(a.scope, func(v, _ string) { a.eng.Window().SetTitle(v) })
	a.eng.SetKeyHandler(func(k core.KeyEvent) { win.keys.Emit(k) })
	a.eng.SetCloseHandler(func() bool {
		req := &CloseRequest{}
		win.Closing.Emit(req)
		return !req.Cancel
	})
	return win, nil
}

// Show makes the window visible.
func (w *Window) Show() {
	w.hidden = false
	w.app.eng.Window().Show()
	w.app.eng.Invalidate()
}

// Hide takes the window off screen without destroying it.
func (w *Window) Hide() {
	w.hidden = true
	w.app.eng.Window().Close()
}

// Message shows a modal box with the text and an OK button, and returns
// when it is closed. Like every window call, from the UI goroutine only.
func (w *Window) Message(title, text string) {
	w.app.eng.Window().Dialog(core.DialogInfo, title, text)
}

// Confirm shows a modal OK/Cancel box and reports whether OK was chosen.
// Any other way out — Cancel, Escape, the close button — is false.
func (w *Window) Confirm(title, text string) bool {
	return w.app.eng.Window().Dialog(core.DialogConfirm, title, text) == core.DialogOK
}

// Ask shows a modal Yes/No box and reports whether Yes was chosen.
func (w *Window) Ask(title, text string) bool {
	return w.app.eng.Window().Dialog(core.DialogYesNo, title, text) == core.DialogYes
}

// FileFilter is a file-type entry for OpenFile and SaveFile.
type FileFilter = core.FileFilter

// OpenFile shows the platform's open-file dialog and returns the chosen
// path; ok is false when the user backs out. Filters, if any, are offered
// in order and the first is selected.
func (w *Window) OpenFile(title string, filters ...FileFilter) (path string, ok bool) {
	return w.app.eng.Window().FileDialog(false, title, "", filters)
}

// SaveFile shows the save-file dialog with suggested as the initial name.
// Overwriting an existing file is the platform's question to ask, and it does.
func (w *Window) SaveFile(title, suggested string, filters ...FileFilter) (path string, ok bool) {
	return w.app.eng.Window().FileDialog(true, title, suggested, filters)
}

// Scale reports the window's current DPI scaling.
func (w *Window) Scale() ScaleInfo { return w.app.eng.Window().Scale() }

// widget is the shared implementation of every control.
type widget struct {
	app  *App
	node *core.Node

	Text    *vreactive.Property[string]
	Enabled *vreactive.Property[bool]
	Visible *vreactive.Property[bool]

	// platformText is the text the platform is known to hold: what was last
	// written to it, or what it last reported. A property change that lands
	// on this value is the platform's own edit coming round, not something
	// to write back — writing it back resets the caret, and a fast typist's
	// next character then lands at the start of the field (seen under Wine:
	// "novaya" arrived as "ayavon").
	//
	// A flag set around Property.Set does not work for this: QueueUpdate
	// runs under vreactive.Batch, so OnChange fires after the batch commits,
	// long after any such flag was cleared. Comparing values has no window
	// to get wrong.
	platformText string
}

func (w *Window) newWidget(kind core.WidgetKind, text string) (*widget, error) {
	n, err := w.app.eng.NewNode(kind)
	if err != nil {
		return nil, err
	}
	wd := &widget{
		app:     w.app,
		node:    n,
		Text:    vreactive.NewProperty(text),
		Enabled: vreactive.NewProperty(true),
		Visible: vreactive.NewProperty(true),
	}
	bw := w.app.eng.Window()
	bw.SetString(n.H, core.PropText, text)
	wd.platformText = text

	s := w.app.scope
	wd.Text.OnChange(s, func(v, _ string) {
		if v == wd.platformText {
			return
		}
		wd.platformText = v
		bw.SetString(n.H, core.PropText, v)
		// Text changes the intrinsic size of a label or a button. A text
		// field's size is a choice, not a function of its contents, and a
		// re-measure of the whole window per keystroke would be wasted.
		if kind != core.KindEntry {
			w.app.eng.Invalidate()
		}
	})
	wd.Enabled.OnChange(s, func(v, _ bool) { bw.SetBool(n.H, core.PropEnabled, v) })
	wd.Visible.OnChange(s, func(v, _ bool) {
		n.Visible = v
		bw.SetBool(n.H, core.PropVisible, v)
		w.app.eng.Invalidate()
	})
	return wd, nil
}

// Label is static text.
type Label struct{ *widget }

// TextView is a multi-line, scrolling, read-only text area — a native control
// on every backend. It is for showing text the user reads but does not edit: a
// log, an output pane.
type TextView struct{ *widget }

// SetText replaces the whole contents.
func (t *TextView) SetText(s string) { t.Text.Set(s) }

// Focus moves keyboard focus to the widget. Tab and Shift+Tab move it on
// from there in creation order, on every backend.
func (w *widget) Focus() { w.app.eng.Focus(w.node) }

// Entry is a single-line native text field. Text is two-way: setting it
// changes the field, and typing updates it. Changed fires per edit with the
// whole contents; Activated fires on Enter.
type Entry struct {
	*widget
	Changed   *vreactive.Event[string]
	Activated *vreactive.Event[string]
}

// AddEntry appends a text field holding text.
func (w *Window) AddEntry(text string) (*Entry, error) {
	wd, err := w.newWidget(core.KindEntry, text)
	if err != nil {
		return nil, err
	}
	e := &Entry{
		widget:    wd,
		Changed:   vreactive.NewEvent[string](),
		Activated: vreactive.NewEvent[string](),
	}
	wd.node.OnTextChanged = func(v string) {
		if v == wd.platformText {
			// GTK and Win32 both report a text the program just wrote as a
			// change; that is our own write coming back, not an edit.
			return
		}
		wd.platformText = v
		e.Text.Set(v)
		e.Changed.Emit(v)
	}
	wd.node.OnActivated = func(v string) { e.Activated.Emit(v) }
	return e, nil
}

// CheckBox is a native check box. Checked is two-way: setting it moves the
// control, and the user moving the control updates it.
type CheckBox struct {
	*widget
	Checked *vreactive.Property[bool]
	Toggled *vreactive.Event[bool]

	// platformChecked is the state the platform is known to hold; see
	// widget.platformText for why it is a value and not a flag.
	platformChecked bool
}

// Button is a native push button.
type Button struct {
	*widget
	Clicked *vreactive.Event[ClickInfo]
}

// AddLabel appends a text row.
func (w *Window) AddLabel(text string) (*Label, error) {
	wd, err := w.newWidget(core.KindLabel, text)
	if err != nil {
		return nil, err
	}
	return &Label{widget: wd}, nil
}

// AddCheckBox appends a check-box row.
func (w *Window) AddCheckBox(text string, checked bool) (*CheckBox, error) {
	wd, err := w.newWidget(core.KindCheckBox, text)
	if err != nil {
		return nil, err
	}
	cb := &CheckBox{
		widget:  wd,
		Checked: vreactive.NewProperty(checked),
		Toggled: vreactive.NewEvent[bool](),
	}
	bw := w.app.eng.Window()
	bw.SetBool(wd.node.H, core.PropChecked, checked)
	cb.platformChecked = checked

	cb.Checked.OnChange(w.app.scope, func(v, _ bool) {
		if v == cb.platformChecked {
			return
		}
		cb.platformChecked = v
		bw.SetBool(wd.node.H, core.PropChecked, v)
	})
	wd.node.OnToggled = func(v bool) {
		cb.platformChecked = v
		cb.Checked.Set(v)
		cb.Toggled.Emit(v)
	}
	return cb, nil
}

// Key is a keystroke, in the winkeys vocabulary shared with unxed/vtinput.
type Key = core.KeyEvent

// KeyPressed fires for every key pressed or released while the window has
// focus. Handlers run on the UI thread like every other event.
//
// The event is delivered rather than consumed: a handler that ignores a key
// leaves it to the focused control, so listening never breaks typing.
func (w *Window) KeyPressed() *vreactive.Event[Key] { return w.keys }

// AddTextView appends a scrolling text area of the given height in DIP.
func (w *Window) AddTextView(height float64) (*TextView, error) {
	wd, err := w.newWidget(core.KindTextView, "")
	if err != nil {
		return nil, err
	}
	if height > 0 {
		w.app.eng.SetPrefHeight(wd.node, height)
	}
	return &TextView{widget: wd}, nil
}

// AddButton appends a push-button row.
func (w *Window) AddButton(text string) (*Button, error) {
	wd, err := w.newWidget(core.KindButton, text)
	if err != nil {
		return nil, err
	}
	b := &Button{widget: wd, Clicked: vreactive.NewEvent[ClickInfo]()}
	wd.node.OnClicked = func() { b.Clicked.Emit(ClickInfo{}) }
	return b, nil
}

// MenuItem is one entry of a tray menu.
type MenuItem struct {
	Label     string
	Separator bool
	Clicked   *vreactive.Event[struct{}]
}

// NewMenuItem returns a clickable entry.
func NewMenuItem(label string) *MenuItem {
	return &MenuItem{Label: label, Clicked: vreactive.NewEvent[struct{}]()}
}

// Separator returns a divider.
func Separator() *MenuItem { return &MenuItem{Separator: true} }

// TrayIcon is an icon in the system status area.
//
// It belongs to the application rather than to a window, so hiding the window
// does not take the icon away — which is the entire point of having one.
type TrayIcon struct {
	app *App

	// Tooltip is what the status area shows on hover.
	Tooltip *vreactive.Property[string]
	// Activated fires on a left click.
	Activated *vreactive.Event[struct{}]

	items []*MenuItem
}

// NewTrayIcon puts an icon in the status area. It returns an error wrapping
// core.ErrNoTray on backends that have none, which callers should treat as
// "run without an icon" rather than as a fatal condition.
func (a *App) NewTrayIcon(tooltip string, items ...*MenuItem) (*TrayIcon, error) {
	t := &TrayIcon{
		app:       a,
		Tooltip:   vreactive.NewProperty(tooltip),
		Activated: vreactive.NewEvent[struct{}](),
		items:     items,
	}
	if err := a.eng.OpenTray(tooltip, t.spec(), t.dispatch); err != nil {
		return nil, err
	}
	t.Tooltip.OnChange(a.scope, func(v, _ string) { a.eng.SetTrayTooltip(v) })
	return t, nil
}

func (t *TrayIcon) spec() []core.MenuItem {
	out := make([]core.MenuItem, 0, len(t.items))
	for i, it := range t.items {
		out = append(out, core.MenuItem{
			ID:        core.Handle(i + 1),
			Label:     it.Label,
			Separator: it.Separator,
		})
	}
	return out
}

func (t *TrayIcon) dispatch(kind core.EventKind, h core.Handle) {
	switch kind {
	case core.EventTrayActivated:
		t.Activated.Emit(struct{}{})
	case core.EventMenuItem:
		i := int(h) - 1
		if i >= 0 && i < len(t.items) && t.items[i].Clicked != nil {
			t.items[i].Clicked.Emit(struct{}{})
		}
	}
}

// SetMenu replaces the menu.
func (t *TrayIcon) SetMenu(items ...*MenuItem) {
	t.items = items
	t.app.eng.SetTrayMenu(t.spec())
}

// Embedded reports whether a panel actually accepted the icon. Creating a tray
// icon succeeds even where nothing displays it, so an application that wants to
// warn "this desktop has no status area" asks rather than assumes.
func (t *TrayIcon) Embedded() bool { return t.app.eng.TrayEmbedded() }
