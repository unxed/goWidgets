//go:build linux

package gtk

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets/core"
)

// The status area on Linux has two possible implementations and neither is
// comfortable.
//
// GtkStatusIcon is deprecated since GTK 3.14. It is nevertheless still present
// and still functional in GTK 3.24, and the panels that matter — Cinnamon,
// XFCE, MATE, KDE — all accept it, either natively or through their XEmbed
// compatibility. StatusNotifierItem is the modern answer, but it is a D-Bus
// protocol that has to be implemented by hand, requires a running
// StatusNotifierWatcher, and on GNOME does not appear at all without an
// extension.
//
// So: GtkStatusIcon, because a deprecated API that works beats a correct one
// that cannot be verified. If a panel ignores it, the failure is visible
// immediately and swapping the implementation touches only this file.
var (
	gtkStatusIconNew      func() uintptr
	gtkStatusIconIconName func(icon uintptr, name string)
	gtkStatusIconTooltip  func(icon uintptr, text string)
	gtkStatusIconVisible  func(icon uintptr, visible int32)
	gtkStatusIconEmbedded func(icon uintptr) int32
	gtkMenuNew            func() uintptr
	gtkMenuItemNewLabel   func(label string) uintptr
	gtkSeparatorItemNew   func() uintptr
	gtkMenuShellAppend    func(shell, child uintptr)
	gtkMenuPopupAtPointer func(menu, event uintptr)
	gtkWidgetShowAll      func(w uintptr)
	gObjectUnref          func(obj uintptr)
)

func (d *driver) registerTray(gtk, gobj uintptr) {
	purego.RegisterLibFunc(&gtkStatusIconNew, gtk, "gtk_status_icon_new")
	purego.RegisterLibFunc(&gtkStatusIconIconName, gtk, "gtk_status_icon_set_from_icon_name")
	purego.RegisterLibFunc(&gtkStatusIconTooltip, gtk, "gtk_status_icon_set_tooltip_text")
	purego.RegisterLibFunc(&gtkStatusIconVisible, gtk, "gtk_status_icon_set_visible")
	purego.RegisterLibFunc(&gtkStatusIconEmbedded, gtk, "gtk_status_icon_is_embedded")
	purego.RegisterLibFunc(&gtkMenuNew, gtk, "gtk_menu_new")
	purego.RegisterLibFunc(&gtkMenuItemNewLabel, gtk, "gtk_menu_item_new_with_label")
	purego.RegisterLibFunc(&gtkSeparatorItemNew, gtk, "gtk_separator_menu_item_new")
	purego.RegisterLibFunc(&gtkMenuShellAppend, gtk, "gtk_menu_shell_append")
	purego.RegisterLibFunc(&gtkMenuPopupAtPointer, gtk, "gtk_menu_popup_at_pointer")
	purego.RegisterLibFunc(&gtkWidgetShowAll, gtk, "gtk_widget_show_all")
	purego.RegisterLibFunc(&gObjectUnref, gobj, "g_object_unref")
}

type tray struct {
	drv    *driver
	icon   uintptr
	menu   uintptr
	events chan core.BackendEvent

	mu    sync.Mutex
	items map[uintptr]core.Handle // menu widget → item id
}

// trayTarget mirrors evTarget: GTK callbacks carry no Go context, and there is
// one tray per process.
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
	if gtkStatusIconNew == nil {
		return nil, core.ErrNoTray
	}
	icon := gtkStatusIconNew()
	if icon == 0 {
		return nil, fmt.Errorf("gtk_status_icon_new вернул NULL")
	}
	name := spec.IconName
	if name == "" {
		name = "applications-system"
	}
	gtkStatusIconIconName(icon, name)
	if spec.Tooltip != "" {
		gtkStatusIconTooltip(icon, spec.Tooltip)
	}
	gtkStatusIconVisible(icon, 1)

	t := &tray{
		drv:    d,
		icon:   icon,
		events: make(chan core.BackendEvent, 32),
		items:  map[uintptr]core.Handle{},
	}
	trayMu.Lock()
	trayCurrent = t
	trayMu.Unlock()

	gSignalConnect(icon, "activate", d.cbTrayActivate, 0, 0, 0)
	gSignalConnect(icon, "popup-menu", d.cbTrayPopup, 0, 0, 0)

	t.SetMenu(spec.Menu)
	return t, nil
}

func (t *tray) SetTooltip(s string) {
	if t.icon != 0 {
		gtkStatusIconTooltip(t.icon, s)
	}
}

func (t *tray) SetMenu(items []core.MenuItem) {
	menu := gtkMenuNew()
	if menu == 0 {
		return
	}
	t.mu.Lock()
	t.items = map[uintptr]core.Handle{}
	for _, it := range items {
		var w uintptr
		if it.Separator {
			w = gtkSeparatorItemNew()
		} else {
			w = gtkMenuItemNewLabel(it.Label)
			t.items[w] = it.ID
			gSignalConnect(w, "activate", t.drv.cbMenuItem, uintptr(it.ID), 0, 0)
		}
		if w != 0 {
			gtkMenuShellAppend(menu, w)
		}
	}
	old := t.menu
	t.menu = menu
	t.mu.Unlock()

	gtkWidgetShowAll(menu)
	if old != 0 {
		gObjectUnref(old)
	}
}

func (t *tray) Destroy() {
	if t.icon != 0 {
		gtkStatusIconVisible(t.icon, 0)
		gObjectUnref(t.icon)
		t.icon = 0
	}
	trayMu.Lock()
	if trayCurrent == t {
		trayCurrent = nil
	}
	trayMu.Unlock()
}

func (t *tray) Events() <-chan core.BackendEvent { return t.events }

// Embedded reports whether a panel actually accepted the icon. Useful when
// nothing appears: it distinguishes "no tray manager running" from a bug.
func (t *tray) Embedded() bool {
	return t.icon != 0 && gtkStatusIconEmbedded != nil && gtkStatusIconEmbedded(t.icon) != 0
}

func (t *tray) popup() {
	t.mu.Lock()
	menu := t.menu
	t.mu.Unlock()
	if menu != 0 {
		gtkMenuPopupAtPointer(menu, 0)
	}
}
