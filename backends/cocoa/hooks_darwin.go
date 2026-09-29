//go:build darwin

package cocoa

import (
	"os"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
)

// Test hooks. A test plays the user by sending the same messages AppKit's
// own event handling would (performClick:, insertText:, sendEvent:), which
// needs the objects and the messaging the driver already has. An
// application has no use for any of it.

// Msg sends a message whose result is an object or an integer.
func Msg(id objc.ID, name string, args ...any) objc.ID { return msg(id, name, args...) }

// MsgBool sends a message with a BOOL result.
func MsgBool(id objc.ID, name string, args ...any) bool { return msgT[bool](id, name, args...) }

// Class returns a class as a receiver.
func Class(name string) objc.ID { return cls(name) }

// NSString and GoString convert strings.
func NSString(s string) objc.ID  { return nsString(s) }
func GoString(ns objc.ID) string { return goString(ns) }
func Size(w, h float64) any      { return nsSize{W: w, H: h} }
func Point(x, y float64) any     { return nsPoint{X: x, Y: y} }

// Rect is a view's alignment rect in its superview — what the layout set.
func Rect(view objc.ID) (x, y, w, h float64) {
	f := msgT[nsRect](view, "frame")
	r := msgT[nsRect](view, "alignmentRectForFrame:", f)
	return r.X, r.Y, r.W, r.H
}

// WindowHandle returns the current NSWindow, or 0.
func WindowHandle() objc.ID {
	if current == nil || current.win == nil {
		return 0
	}
	return current.win.handle
}

// WidgetHandle returns the view of the first widget of the kind, or 0.
func WidgetHandle(kind core.WidgetKind) objc.ID { return NthWidgetHandle(kind, 0) }

// NthWidgetHandle is WidgetHandle for the i-th widget of the kind.
func NthWidgetHandle(kind core.WidgetKind, i int) objc.ID {
	if n := nthNode(kind, i); n != nil {
		return n.view
	}
	return 0
}

// InnerHandle returns the table or text view inside the i-th scrolling
// widget of the kind, or 0.
func InnerHandle(kind core.WidgetKind, i int) objc.ID {
	if n := nthNode(kind, i); n != nil {
		return n.inner
	}
	return 0
}

func nthNode(kind core.WidgetKind, i int) *node {
	if current == nil || current.win == nil {
		return nil
	}
	for h := core.Handle(1); h <= current.win.nextH; h++ {
		if n := current.win.nodes[h]; n != nil && n.kind == kind {
			if i == 0 {
				return n
			}
			i--
		}
	}
	return nil
}

// DialogHandle returns the NSAlert or NSSavePanel running modally, or 0.
func DialogHandle() objc.ID {
	dialogMu.Lock()
	defer dialogMu.Unlock()
	return dialogCurrent
}

// TrayButton returns the status item's button, or 0.
func TrayButton() objc.ID {
	trayMu.Lock()
	defer trayMu.Unlock()
	if trayCurrent == nil || trayCurrent.item == 0 {
		return 0
	}
	return msg(trayCurrent.item, "button")
}

// TrayMenuItem returns the i-th item of the tray menu, or 0.
func TrayMenuItem(i int) objc.ID {
	trayMu.Lock()
	defer trayMu.Unlock()
	if trayCurrent == nil || i < 0 || i >= len(trayCurrent.order) {
		return 0
	}
	return trayCurrent.order[i]
}

// Snapshot renders the window's content into a PNG file: what the controls
// look like, without needing screen-recording permission for a screenshot.
func Snapshot(path string) error {
	w := WindowHandle()
	if w == 0 {
		return os.ErrNotExist
	}
	view := msg(w, "contentView")
	bounds := msgT[nsRect](view, "bounds")
	rep := msg(view, "bitmapImageRepForCachingDisplayInRect:", bounds)
	msg(view, "cacheDisplayInRect:toBitmapImageRep:", bounds, rep)
	const png = 4 // NSBitmapImageFileTypePNG
	data := msg(rep, "representationUsingType:properties:", uint64(png), msg(cls("NSDictionary"), "dictionary"))
	if !msgT[bool](data, "writeToFile:atomically:", nsString(path), true) {
		return os.ErrInvalid
	}
	return nil
}

// EndModal ends the modal session on screen with a response code, as a
// panel's own buttons do — NSSavePanel runs out of process on current macOS
// and its ok:/cancel: are not implemented on this side.
func EndModal(code int64) {
	msg(nsApp, "stopModalWithCode:", code)
	postWakeEvent()
}
