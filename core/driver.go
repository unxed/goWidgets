// Package core holds the platform-independent engine: geometry types, the node
// registry, the scheduler and the backend contract.
//
// Layering rule (§7): backends may import core; core must never import a
// backend. Drivers register themselves through init() + build tags.
package core

import (
	"context"
	"errors"
)

// ------------------------------------------------------------- §4.1 geometry

// All coordinates in core are DIP (device-independent pixels, float64).
// Rounding to physical pixels happens in the backend and nowhere else.
type (
	// Point is a position in DIP.
	Point struct{ X, Y float64 }
	// Size is an extent in DIP.
	Size struct{ W, H float64 }
	// Rect is a rectangle in DIP.
	Rect struct{ X, Y, W, H float64 }
	// Insets are per-edge paddings in DIP.
	Insets struct{ L, T, R, B float64 }
)

// ScaleInfo describes a window's scaling. It changes when the window moves
// between monitors with different DPI.
type ScaleInfo struct {
	Scale     float64 // 1.0, 1.5, 2.0 …
	FontScale float64 // system font enlargement
}

// ------------------------------------------------------------- §4.4 backend

// Handle is an opaque node identifier — the shared vocabulary between core and
// a backend. Zero is never a valid handle.
type Handle uint64

// WidgetKind enumerates the widget types a backend must be able to create.
type WidgetKind uint8

// Widget kinds. Iteration 1 covers only what the crescent showcase needs.
const (
	KindLabel WidgetKind = iota + 1
	KindButton
	KindCheckBox
	// KindTextView is a multi-line, read-only, scrolling text area — a native
	// GtkTextView on GTK and a multiline EDIT on Win32.
	KindTextView
	// KindEdit is a single-line text field — GtkEntry on GTK, a single-line
	// EDIT on Win32.
	KindEdit
	// KindComboBox is a drop-down list, with or without a text field —
	// GtkComboBoxText on GTK, COMBOBOX on Win32.
	KindComboBox
)

func (k WidgetKind) String() string {
	switch k {
	case KindLabel:
		return "Label"
	case KindButton:
		return "Button"
	case KindCheckBox:
		return "CheckBox"
	case KindTextView:
		return "TextView"
	case KindEdit:
		return "Edit"
	case KindComboBox:
		return "ComboBox"
	}
	return "Unknown"
}

// PropKey names a settable backend property. Deliberately a narrow, typed
// channel — no reflect (§4.4).
type PropKey uint8

// Property keys.
const (
	PropText PropKey = iota + 1
	PropEnabled
	PropVisible
	PropChecked
	// PropSelected is a list's selected index (int), -1 for none.
	PropSelected
	// PropItems is a list's items.
	PropItems
	// PropDropdownOnly makes a combo box a pure list, no text entry. Set
	// once, before the widget is shown.
	PropDropdownOnly
)

func (p PropKey) String() string {
	switch p {
	case PropText:
		return "Text"
	case PropEnabled:
		return "Enabled"
	case PropVisible:
		return "Visible"
	case PropSelected:
		return "selected"
	case PropItems:
		return "items"
	case PropDropdownOnly:
		return "dropdownOnly"
	case PropChecked:
		return "Checked"
	}
	return "Unknown"
}

// DialogKind selects a message box's button set. The button texts are the
// platform's own and come out in the user's language.
type DialogKind int

const (
	// DialogInfo has a single OK.
	DialogInfo DialogKind = iota
	// DialogConfirm has OK and Cancel.
	DialogConfirm
	// DialogYesNo has Yes and No.
	DialogYesNo
)

// DialogResult is what the user chose. Closing the box any other way
// (Escape, the title-bar cross) reads as the cautious answer: Cancel or No.
type DialogResult int

const (
	DialogOK DialogResult = iota
	DialogCancel
	DialogYes
	DialogNo
)

// FileFilter is one entry of a file dialog's type list: a name the user sees
// and the glob patterns it admits ("*.txt").
type FileFilter struct {
	Name     string
	Patterns []string
}

// BoundsChange is one entry of a layout batch.
type BoundsChange struct {
	H       Handle
	R       Rect
	Visible bool
}

// Caps declares what a driver can do. Anything not declared here may be
// answered with "unsupported" — see §3.3 and the vcontract suite.
type Caps struct {
	NativeControls  bool
	TreeView        bool
	GridView        bool
	FileDialog      bool
	Menus           bool
	Clipboard       bool
	IME             bool
	A11y            bool
	SmoothAnimation bool
	TrayIcon        bool // added for the crescent showcase, see ADR-0003
	MaxCallbacks    int
}

// EventKind classifies a backend event.
type EventKind uint8

// Backend event kinds.
const (
	EventClicked EventKind = iota + 1
	EventToggled
	EventCloseRequested
	EventScaleChanged
	EventResized

	// EventKey is a key pressed or released while a window has focus.
	EventKey
	// EventTrayActivated is a left click on the tray icon.
	EventTrayActivated
	// EventMenuItem carries the id of the chosen menu entry in H.
	EventMenuItem
	// EventTextChanged reports that the user edited a text field; Text is the
	// field's whole new contents.
	EventTextChanged
	// EventActivated is Enter pressed in a text field; Text is its contents.
	EventActivated
	// EventSelected is a list item chosen; Int is its index, Text its text.
	EventSelected
)

// BackendEvent travels from the platform to core. It carries no pointers, so a
// driver may queue it from a native callback without touching Go memory rules.
// KeyEvent describes one keystroke.
//
// The shape is winkeys.InputEvent — the Win32 INPUT_RECORD layout that
// unxed/vtinput already uses in production. Reusing it costs the Win32 backend
// no conversion at all, lets terminal and GUI code share one vocabulary, and
// borrows a tested table of key codes instead of inventing another. GTK maps
// its keyvals onto the same codes.
type KeyEvent struct {
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	Char            rune
	KeyDown         bool
	ControlKeyState uint32
	RepeatCount     uint16
}

// Ctrl reports whether either control key was held.
func (k KeyEvent) Ctrl() bool { return k.ControlKeyState&(0x0004|0x0008) != 0 }

// Alt reports whether either alt key was held.
func (k KeyEvent) Alt() bool { return k.ControlKeyState&(0x0001|0x0002) != 0 }

// Shift reports whether shift was held.
func (k KeyEvent) Shift() bool { return k.ControlKeyState&0x0010 != 0 }

type BackendEvent struct {
	Kind  EventKind
	H     Handle
	Size  Size
	Scale ScaleInfo
	Bool  bool     // EventToggled: the control's new state
	Key   KeyEvent // EventKey: the keystroke
	Text  string   // EventTextChanged, EventActivated, EventSelected: text
	Int   int      // EventSelected: the index
}

// MenuItem is one entry of the tray menu. A separator ignores Label.
type MenuItem struct {
	ID        Handle
	Label     string
	Separator bool
}

// TraySpec describes the tray icon to create.
type TraySpec struct {
	Tooltip string
	// IconName is a themed icon name on platforms that have an icon theme.
	// Empty means the backend picks something neutral.
	IconName string
	Menu     []MenuItem
}

// BackendTray is a status-area icon. It is owned by the driver rather than by a
// window: an application that has hidden its window must keep its tray icon.
type BackendTray interface {
	SetTooltip(string)
	SetMenu([]MenuItem)
	Destroy()
	Events() <-chan BackendEvent

	// Embedded reports whether a panel has actually accepted the icon.
	// Creating one succeeds even when no status area is running, so this is
	// what separates "no panel here" from "our code is broken" — and it is
	// what makes the tray assertable in a test instead of by eye.
	Embedded() bool
}

// WindowSpec describes a window to create.
type WindowSpec struct {
	Title  string
	Size   Size
	Hidden bool
}

// DriverInfo answers "why do I not see native controls?" (§3.3.4).
type DriverInfo struct {
	Name     string
	Caps     Caps
	Attempts []string // every driver tried, in order, with the outcome

	// LayoutConflicts lists every constraint the solver rejected as
	// unsatisfiable and core dropped (§5.3), most recent last. Empty is the
	// normal state; anything here is a bug in the caller's constraints.
	LayoutConflicts []string
}

// PlatformDriver is the contract every backend implements.
//
// Deviation from §4.4, recorded in ADR-0002: Wake is added and RunMainLoop
// takes a pump callback. A native loop blocks inside GetMessage/gtk_main, so
// QueueUpdate (§4.5.3) needs a platform-specific nudge to interrupt it, and the
// woken loop needs a way to run the queued work on the UI thread it owns.
type PlatformDriver interface {
	Name() string
	Init() error
	Capabilities() Caps

	// RunMainLoop owns the UI thread until ctx is cancelled. It must call pump
	// once before blocking and again after every Wake.
	RunMainLoop(ctx context.Context, pump func()) error

	// Wake is safe to call from any goroutine.
	Wake()

	CreateWindow(spec WindowSpec) (BackendWindow, error)

	// CreateTray returns ErrNoTray on drivers that have no status area.
	CreateTray(spec TraySpec) (BackendTray, error)

	Shutdown()
}

// ErrNoTray is returned by CreateTray on a driver without a status area.
var ErrNoTray = errors.New("core: this backend has no tray")

// BackendWindow is one native top-level window plus the widgets inside it.
type BackendWindow interface {
	SetTitle(string)
	Show()
	Close()
	Scale() ScaleInfo

	CreateWidget(kind WidgetKind, parent Handle) (Handle, error)
	DestroyWidget(h Handle)
	SetParent(child, parent Handle, index int)

	SetString(h Handle, p PropKey, v string)
	SetBool(h Handle, p PropKey, v bool)
	SetFloat(h Handle, p PropKey, v float64)
	SetInt(h Handle, p PropKey, v int)
	SetList(h Handle, p PropKey, items []string)

	// MeasureIntrinsic is the only source of truth about a widget's natural
	// size: only the platform knows its font metrics and theme padding (§5.1).
	MeasureIntrinsic(h Handle, avail Size) (min, natural Size)

	// ApplyLayout receives absolute rectangles for changed nodes only.
	ApplyLayout(changes []BoundsChange)

	// Focus moves keyboard focus to a widget. Before the window is shown the
	// backend remembers the request and applies it when it can.
	Focus(h Handle)

	// Dialog shows a modal message box over the window and blocks until the
	// user answers. Modal here is the platform's own nested loop, so the
	// application keeps processing its queue while the dialog is up.
	Dialog(kind DialogKind, title, text string) DialogResult

	// FileDialog shows the platform's open or save dialog, modal over the
	// window, and returns the chosen path or ok=false when the user backed
	// out. suggested is the initial file name for a save dialog.
	FileDialog(save bool, title, suggested string, filters []FileFilter) (path string, ok bool)

	// RootHandle identifies the window's content area as a layout parent.
	RootHandle() Handle

	Events() <-chan BackendEvent
}
