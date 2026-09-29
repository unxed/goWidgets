//go:build linux

package qt

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets/core"
)

// The tray is QSystemTrayIcon. Qt picks the protocol itself: a
// StatusNotifierItem over D-Bus where a watcher runs (Plasma), the XEmbed
// system tray otherwise (XFCE, MATE, stalonetray in CI) — the choice the GTK
// driver had to make by hand and could not make for SNI.
var (
	qTrayCtor      func(this unsafe.Pointer, parent uintptr)
	qTrayIcon      func(t uintptr, icon *qvalue)
	qTrayTooltip   func(t uintptr, text *qstring)
	qTrayVisible   func(t uintptr, on bool)
	qTrayIsVisible func(t uintptr) bool
	qTrayMenu      func(t uintptr, menu uintptr)
	qTrayAvailable func() bool
	qIconFromTheme func(name *qstring) qvalue
	qIconIsNull    func(i *qvalue) bool
	qAppStyle      func() uintptr
	qMenuCtor      func(this unsafe.Pointer, parent uintptr)
	qMenuAddAction func(m uintptr, text *qstring) uintptr
	qMenuAddSep    func(m uintptr) uintptr

	sigTrayActivated signal // QSystemTrayIcon::activated(ActivationReason)
	sigTriggered     signal // QAction::triggered(bool)

	slotStandardIcon int // QCommonStyle::standardIcon, for the fallback icon
)

func (r *resolver) bindTray() {
	r.fn(&qTrayCtor, "_ZN15QSystemTrayIconC1EP7QObject")
	r.fn(&qTrayIcon, "_ZN15QSystemTrayIcon7setIconERK5QIcon")
	r.fn(&qTrayTooltip, "_ZN15QSystemTrayIcon10setToolTipERK7QString")
	r.fn(&qTrayVisible, "_ZN15QSystemTrayIcon10setVisibleEb")
	r.fn(&qTrayIsVisible, "_ZNK15QSystemTrayIcon9isVisibleEv")
	r.fn(&qTrayMenu, "_ZN15QSystemTrayIcon14setContextMenuEP5QMenu")
	r.fn(&qTrayAvailable, "_ZN15QSystemTrayIcon21isSystemTrayAvailableEv")
	r.fn(&qIconFromTheme, "_ZN5QIcon9fromThemeERK7QString")
	r.fn(&qIconIsNull, "_ZNK5QIcon6isNullEv")
	r.fn(&qAppStyle, "_ZN12QApplication5styleEv")
	r.fn(&qMenuCtor, "_ZN5QMenuC1EP7QWidget")
	// Qt 6.3 moved addAction(QString) up to QWidget; older Qt 6 and Qt 5
	// have it on QMenu.
	r.fn(&qMenuAddAction, v("_ZN5QMenu9addActionERK7QString", "_ZN7QWidget9addActionERK7QString"), "_ZN5QMenu9addActionERK7QString")
	r.fn(&qMenuAddSep, "_ZN5QMenu12addSeparatorEv")
	sigTrayActivated = r.signal("15QSystemTrayIcon", "_ZN15QSystemTrayIcon9activatedENS_16ActivationReasonE")
	sigTriggered = r.signal("7QAction", "_ZN7QAction9triggeredEb")

	// QStyle::standardIcon is pure virtual in Qt 5; QCommonStyle's slot is
	// the same slot, and every style derives from QCommonStyle.
	slotStandardIcon = vslot("_ZTV12QCommonStyle", "_ZNK12QCommonStyle12standardIconEN6QStyle14StandardPixmapEPK12QStyleOptionPK7QWidget", 160)
	if slotStandardIcon < 0 {
		r.missing = append(r.missing, "QCommonStyle::standardIcon slot")
	}
}

type tray struct {
	icon    uintptr   // QSystemTrayIcon*
	menu    uintptr   // QMenu*
	actions []uintptr // QAction* per menu entry, separators included
	events  chan core.BackendEvent
}

// trayCurrent mirrors evTarget: one tray per process, and Qt callbacks carry
// no Go context.
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
	obj := cxxNew(64) // sizeof(QSystemTrayIcon) is 16
	qTrayCtor(obj, 0)
	t := &tray{icon: uintptr(obj), events: make(chan core.BackendEvent, 32)}

	name := spec.IconName
	if name == "" {
		name = "applications-system"
	}
	icon := themedIcon(name)
	qTrayIcon(t.icon, &icon)
	qIconDtor(&icon)
	t.SetTooltip(spec.Tooltip)

	trayMu.Lock()
	trayCurrent = t
	trayMu.Unlock()

	// A left click is Trigger; the right click opens the context menu on
	// its own and is not reported.
	const trigger = 3
	connect(t.icon, sigTrayActivated, func(a *[4]unsafe.Pointer) {
		if *(*int32)(a[1]) == trigger {
			trayEmit(core.BackendEvent{Kind: core.EventTrayActivated})
		}
	})
	t.SetMenu(spec.Menu)
	qTrayVisible(t.icon, true)
	return t, nil
}

// themedIcon is QIcon::fromTheme, or the style's computer icon when the
// theme has no such name (or there is no icon theme at all, as under Xvfb):
// a tray icon without a picture is not shown by every panel.
func themedIcon(name string) qvalue {
	var icon qvalue
	qstr(name, func(s *qstring) { icon = qIconFromTheme(s) })
	if !qIconIsNull(&icon) {
		return icon
	}
	qIconDtor(&icon)
	var standardIcon func(style uintptr, which int32, option, widget uintptr) qvalue
	purego.RegisterFunc(&standardIcon, vfunc(qAppStyle(), slotStandardIcon))
	const spComputerIcon = 15
	return standardIcon(qAppStyle(), spComputerIcon, 0, 0)
}

func (t *tray) SetTooltip(s string) {
	qstr(s, func(q *qstring) { qTrayTooltip(t.icon, q) })
}

// SetMenu builds a fresh QMenu and swaps it in. Actions report their item's
// id; a separator is QMenu's own.
func (t *tray) SetMenu(items []core.MenuItem) {
	obj := cxxNew(128)
	qMenuCtor(obj, 0)
	menu := uintptr(obj)
	t.actions = t.actions[:0]
	for _, it := range items {
		if it.Separator {
			t.actions = append(t.actions, qMenuAddSep(menu))
			continue
		}
		id := it.ID
		var act uintptr
		qstr(it.Label, func(s *qstring) { act = qMenuAddAction(menu, s) })
		t.actions = append(t.actions, act)
		connect(act, sigTriggered, func(*[4]unsafe.Pointer) {
			trayEmit(core.BackendEvent{Kind: core.EventMenuItem, H: id})
		})
	}
	qTrayMenu(t.icon, menu)
	if t.menu != 0 {
		qDeleteLater(t.menu)
	}
	t.menu = menu
}

func (t *tray) Destroy() {
	if t.icon != 0 {
		qTrayVisible(t.icon, false)
		qDeleteLater(t.icon)
		t.icon = 0
	}
	trayMu.Lock()
	if trayCurrent == t {
		trayCurrent = nil
	}
	trayMu.Unlock()
}

func (t *tray) Events() <-chan core.BackendEvent { return t.events }

// Embedded reports whether a status area has taken the icon: the icon is up
// and a system tray exists on this display. Qt does not say more than that.
func (t *tray) Embedded() bool {
	return t.icon != 0 && qTrayAvailable() && qTrayIsVisible(t.icon)
}

// TrayHandle returns the QSystemTrayIcon* of the current tray, or 0. A test
// hook.
func TrayHandle() uintptr {
	trayMu.Lock()
	defer trayMu.Unlock()
	if trayCurrent == nil {
		return 0
	}
	return trayCurrent.icon
}

// TrayAction returns the QAction* of the i-th menu entry of the current
// tray, or 0. A test hook: a test triggers it as a click would.
func TrayAction(i int) uintptr {
	trayMu.Lock()
	defer trayMu.Unlock()
	if trayCurrent == nil || i < 0 || i >= len(trayCurrent.actions) {
		return 0
	}
	return trayCurrent.actions[i]
}
