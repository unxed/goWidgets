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
		Title:   vreactive.NewProperty(title),
		Closing: vreactive.NewEvent[*CloseRequest](),
		hidden:  true,
	}
	win.Title.OnChange(a.scope, func(v, _ string) { a.eng.Window().SetTitle(v) })
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

// Scale reports the window's current DPI scaling.
func (w *Window) Scale() ScaleInfo { return w.app.eng.Window().Scale() }

// widget is the shared implementation of every control.
type widget struct {
	app  *App
	node *core.Node

	Text    *vreactive.Property[string]
	Enabled *vreactive.Property[bool]
	Visible *vreactive.Property[bool]
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

	s := w.app.scope
	wd.Text.OnChange(s, func(v, _ string) {
		bw.SetString(n.H, core.PropText, v)
		w.app.eng.Invalidate() // text changes the intrinsic size
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

// CheckBox is a native check box. Checked is two-way: setting it moves the
// control, and the user moving the control updates it.
type CheckBox struct {
	*widget
	Checked *vreactive.Property[bool]
	Toggled *vreactive.Event[bool]

	// syncing suppresses the echo when a property change originates from the
	// platform. Without it, backend → property → backend is an infinite loop on
	// any toolkit that reports the state it was just told about.
	syncing bool
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

	cb.Checked.OnChange(w.app.scope, func(v, _ bool) {
		if cb.syncing {
			return
		}
		bw.SetBool(wd.node.H, core.PropChecked, v)
	})
	wd.node.OnToggled = func(v bool) {
		cb.syncing = true
		cb.Checked.Set(v)
		cb.syncing = false
		cb.Toggled.Emit(v)
	}
	return cb, nil
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
