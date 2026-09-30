//go:build darwin

package cocoa

import (
	"bytes"
	"image/png"
	"runtime"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
)

// PresentCanvas copies the completed RGBA frame into AppKit's image view.
// NSImageView takes care of drawing the image into the frame assigned by the
// shared Auto Layout engine.
func (w *window) PresentCanvas(h core.Handle, frame *core.CanvasFrame) {
	n := w.nodes[h]
	if n == nil || n.kind != core.KindCanvas || frame == nil || frame.Image == nil {
		return
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, frame.Image) != nil || encoded.Len() == 0 {
		return
	}
	dataBytes := encoded.Bytes()
	data := msg(cls("NSData"), "dataWithBytes:length:", &dataBytes[0], uintptr(len(dataBytes)))
	image := msg(msg(cls("NSImage"), "alloc"), "initWithData:", data)
	if image != 0 {
		msg(n.view, "setImage:", image)
		msg(image, "release")
	}
	runtime.KeepAlive(dataBytes)
}

const (
	eventLeftMouseDown      = 1
	eventLeftMouseUp        = 2
	eventRightMouseDown     = 3
	eventCanvasRightMouseUp = 4
	eventMouseMoved         = 5
	eventLeftMouseDragged   = 6
	eventRightMouseDragged  = 7
	eventScrollWheel        = 22
	eventOtherMouseDown     = 25
	eventOtherMouseUp       = 26
	eventOtherMouseDragged  = 27
	modifierShift           = 1 << 17
	modifierOption          = 1 << 19
)

func handleCanvasEvent(w *window, ev objc.ID, typ uint64) bool {
	if w == nil {
		return false
	}
	move := typ == eventMouseMoved || typ == eventLeftMouseDragged || typ == eventRightMouseDragged || typ == eventOtherMouseDragged
	scroll := typ == eventScrollWheel
	down := typ == eventLeftMouseDown || typ == eventRightMouseDown || typ == eventOtherMouseDown
	up := typ == eventLeftMouseUp || typ == eventCanvasRightMouseUp || typ == eventOtherMouseUp
	if !move && !scroll && !down && !up {
		return false
	}

	location := msgT[nsPoint](ev, "locationInWindow")
	h := w.canvasCapture
	var n *node
	if h != 0 && (move || up) {
		n = w.nodes[h]
	}
	if n == nil {
		contentPoint := msgT[nsPoint](w.content, "convertPoint:fromView:", location, objc.ID(0))
		view := msg(w.content, "hitTest:", contentPoint)
		h, n = lookup(view)
	}
	if n == nil || n.kind != core.KindCanvas {
		return false
	}
	point := msgT[nsPoint](n.view, "convertPoint:fromView:", location, objc.ID(0))
	bounds := msgT[nsRect](n.view, "bounds")
	if !msgT[bool](n.view, "isFlipped") {
		point.Y = bounds.H - point.Y
	}
	flags := uint64(msg(ev, "modifierFlags"))
	mods := core.Modifiers(0)
	if flags&modifierShift != 0 {
		mods |= core.ModShift
	}
	if flags&modifierControl != 0 {
		mods |= core.ModControl
	}
	if flags&modifierOption != 0 {
		mods |= core.ModAlt
	}
	button := core.MouseNone
	if down || up {
		switch int64(msg(ev, "buttonNumber")) {
		case 0:
			button = core.MouseLeft
		case 1:
			button = core.MouseRight
		case 2:
			button = core.MouseMiddle
		}
		if down && button != core.MouseNone {
			n.canvasButtons |= 1 << (button - 1)
			w.canvasCapture = h
		} else if up && button != core.MouseNone {
			n.canvasButtons &^= 1 << (button - 1)
		}
	}
	info := core.MouseInfo{X: point.X, Y: point.Y, Button: button, Mods: mods}
	kind := core.EventMouseMove
	switch {
	case down:
		kind = core.EventMouseDown
	case up:
		kind = core.EventMouseUp
	case scroll:
		kind = core.EventMouseWheel
		info.Delta = msgT[float64](ev, "scrollingDeltaY")
	}
	emit(core.BackendEvent{Kind: kind, H: h, Mouse: info})
	if up && n.canvasButtons == 0 && w.canvasCapture == h {
		w.canvasCapture = 0
	}
	return false // Canvas is passive; preserve AppKit's normal event dispatch.
}
