//go:build darwin

package cocoa

import (
	"sync"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
)

// The tray is an NSStatusItem in the menu bar. A Mac status item usually
// opens its menu on any click; the contract here is the GTK and Win32 one —
// a left click is Activated, the menu is for the right click (or a
// Control-click, the Mac's right click) — so the item gets an action rather
// than a fixed menu, and pops the menu itself when asked to.

type tray struct {
	item   objc.ID // NSStatusItem
	menu   objc.ID // NSMenu
	ids    map[objc.ID]core.Handle
	order  []objc.ID // menu items in order, separators included
	events chan core.BackendEvent
}

var (
	trayMu      sync.Mutex
	trayCurrent *tray
)

func trayEmit(ev core.BackendEvent) {
	trayMu.Lock()
	t := trayCurrent
	trayMu.Unlock()
	if t == nil {
		return
	}
	select {
	case t.events <- ev:
	default:
	}
}

func (d *driver) CreateTray(spec core.TraySpec) (core.BackendTray, error) {
	bar := msg(cls("NSStatusBar"), "systemStatusBar")
	item := msg(bar, "statusItemWithLength:", float64(variableStatusItemLen))
	if item == 0 {
		return nil, core.ErrNoTray
	}
	msg(item, "retain")
	t := &tray{item: item, events: make(chan core.BackendEvent, 32)}

	button := msg(item, "button")
	msg(button, "setImage:", trayImage(spec.IconName))
	msg(button, "setTarget:", target)
	msg(button, "setAction:", sel("trayFire:"))
	const leftUp, rightUp = 1 << 2, 1 << 4 // NSEventMaskLeftMouseUp, RightMouseUp
	msg(button, "sendActionOn:", uint64(leftUp|rightUp))
	t.SetTooltip(spec.Tooltip)

	trayMu.Lock()
	trayCurrent = t
	trayMu.Unlock()
	t.SetMenu(spec.Menu)
	return t, nil
}

// trayImage is a template image — drawn in the menu bar's own colours, as
// status items are. A freedesktop icon name means nothing here; an SF
// Symbol of the same spirit stands in, with AppKit's action image where
// symbols are missing (before macOS 11).
func trayImage(name string) objc.ID {
	symbol := "gearshape"
	img := objc.ID(0)
	if msgT[bool](cls("NSImage"), "respondsToSelector:", sel("imageWithSystemSymbolName:accessibilityDescription:")) {
		img = msg(cls("NSImage"), "imageWithSystemSymbolName:accessibilityDescription:", nsString(symbol), objc.ID(0))
	}
	if img == 0 {
		img = msg(cls("NSImage"), "imageNamed:", nsString("NSActionTemplate"))
	}
	msg(img, "setTemplate:", true)
	return img
}

func (t *tray) SetTooltip(s string) {
	msg(msg(t.item, "button"), "setToolTip:", nsString(s))
}

func (t *tray) SetMenu(items []core.MenuItem) {
	menu := alloc("NSMenu")
	ids := map[objc.ID]core.Handle{}
	var order []objc.ID
	for _, it := range items {
		if it.Separator {
			sep := msg(cls("NSMenuItem"), "separatorItem")
			msg(menu, "addItem:", sep)
			order = append(order, sep)
			continue
		}
		mi := msg(menu, "addItemWithTitle:action:keyEquivalent:", nsString(it.Label), sel("menuFire:"), nsString(""))
		msg(mi, "setTarget:", target)
		ids[mi] = it.ID
		order = append(order, mi)
	}
	trayMu.Lock()
	old := t.menu
	t.menu, t.ids, t.order = menu, ids, order
	trayMu.Unlock()
	if old != 0 {
		msg(old, "release")
	}
}

func (t *tray) Destroy() {
	if t.item != 0 {
		msg(msg(cls("NSStatusBar"), "systemStatusBar"), "removeStatusItem:", t.item)
		msg(t.item, "release")
		t.item = 0
	}
	trayMu.Lock()
	if trayCurrent == t {
		trayCurrent = nil
	}
	trayMu.Unlock()
}

func (t *tray) Events() <-chan core.BackendEvent { return t.events }

// Embedded reports whether the item is actually in the menu bar: it is
// visible and its button sits in a window there.
func (t *tray) Embedded() bool {
	if t.item == 0 {
		return false
	}
	button := msg(t.item, "button")
	return msgT[bool](t.item, "isVisible") && button != 0 && msg(button, "window") != 0
}

// onTray is a click on the status item.
func onTray(sender objc.ID) {
	trayMu.Lock()
	t := trayCurrent
	trayMu.Unlock()
	if t == nil {
		return
	}
	ev := msg(nsApp, "currentEvent")
	right := ev != 0 && (uint64(msg(ev, "type")) == eventRightMouseUp ||
		uint64(msg(ev, "modifierFlags"))&modifierControl != 0)
	if right && t.menu != 0 {
		// Deprecated since 10.14 for items that hold a menu, which this
		// one does not; it remains the way to open one on demand.
		msg(t.item, "popUpStatusItemMenu:", t.menu)
		return
	}
	trayEmit(core.BackendEvent{Kind: core.EventTrayActivated})
}

// onMenuItem is a pick from the tray menu.
func onMenuItem(sender objc.ID) {
	trayMu.Lock()
	t := trayCurrent
	var id core.Handle
	if t != nil {
		id = t.ids[sender]
	}
	trayMu.Unlock()
	if id != 0 {
		trayEmit(core.BackendEvent{Kind: core.EventMenuItem, H: id})
	}
}
