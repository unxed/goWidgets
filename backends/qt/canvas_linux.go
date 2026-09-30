//go:build linux

package qt

import (
	"bytes"
	"image/png"
	"runtime"
	"unsafe"

	"github.com/unxed/goWidgets/core"
)

var canvasAvailable bool

func (r *resolver) bindCanvas() bool {
	available := true
	for _, binding := range []struct {
		fn     any
		symbol string
	}{
		{&qWidgetMouse, "_ZN7QWidget16setMouseTrackingEb"},
		{&qWidgetMapFromGlobal, "_ZNK7QWidget13mapFromGlobalERK6QPoint"},
		{&qCursorPos, "_ZN7QCursor3posEv"},
		{&qMouseButtons, "_ZN15QGuiApplication11mouseButtonsEv"},
		{&qKeyboardMods, "_ZN15QGuiApplication17keyboardModifiersEv"},
		{&qPixmapLoadData, "_ZN7QPixmap12loadFromDataEPKhjPKc6QFlagsIN2Qt19ImageConversionFlagEE"},
		{&qPixmapDefault, "_ZN7QPixmapC1Ev"},
	} {
		available = r.optionalFn(binding.fn, binding.symbol) && available
	}
	// Do not advertise the widget until the Qt-major-specific wheel offset is
	// measured and recorded in abi_linux.go.
	return available && lay.wheelDelta != 0
}

// PresentCanvas encodes the reusable frame to PNG and lets Qt's QPixmap
// decoder take an owned copy before the temporary Go byte slice is released.
// QLabel scales that pixmap to the core-allocated canvas bounds.
func (w *window) PresentCanvas(h core.Handle, frame *core.CanvasFrame) {
	n := w.nodes[h]
	if n == nil || n.kind != core.KindCanvas || frame == nil || frame.Image == nil {
		return
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame.Image); err != nil || encoded.Len() == 0 {
		return
	}
	data := encoded.Bytes()
	pix := cxxNew(64)
	qPixmapDefault(pix)
	format := []byte{'P', 'N', 'G', 0}
	if qPixmapLoadData(uintptr(pix), &data[0], uint32(len(data)), &format[0], 0) {
		qLabelPixmap(n.handle, pix)
	}
	qPixmapDtor(pix)
	runtime.KeepAlive(data)
}

func (d *driver) canvasEvent(watched uintptr, ev unsafe.Pointer) {
	w := d.win
	if w == nil {
		return
	}
	var n *node
	var h core.Handle
	for id, candidate := range w.nodes {
		if candidate.kind == core.KindCanvas && candidate.handle == watched {
			h, n = id, candidate
			break
		}
	}
	if n == nil {
		return
	}
	kind := evType(ev)
	point := qCursorPos()
	global := qPoint{X: int32(uint32(point)), Y: int32(uint32(point >> 32))}
	local := qWidgetMapFromGlobal(n.handle, &global)
	m := core.MouseInfo{
		X: float64(int32(uint32(local))),
		Y: float64(int32(uint32(local >> 32))),
	}
	mods := uint32(qKeyboardMods())
	if mods&0x02000000 != 0 {
		m.Mods |= core.ModShift
	}
	if mods&0x04000000 != 0 {
		m.Mods |= core.ModControl
	}
	if mods&0x08000000 != 0 {
		m.Mods |= core.ModAlt
	}

	var event core.EventKind
	switch kind {
	case evMouseButtonPress, evMouseDoubleClick:
		event = core.EventMouseDown
		buttons := qMouseButtons()
		changed := buttons &^ n.canvasButtons
		if changed == 0 {
			changed = buttons
		}
		m.Button = qtMouseButton(changed)
		n.canvasButtons = buttons
	case evMouseButtonRelease:
		event = core.EventMouseUp
		buttons := qMouseButtons()
		changed := n.canvasButtons &^ buttons
		if changed == 0 {
			changed = n.canvasButtons
		}
		m.Button = qtMouseButton(changed)
		n.canvasButtons = buttons
	case evMouseMove:
		event = core.EventMouseMove
		n.canvasButtons = qMouseButtons()
	case evWheel:
		event = core.EventMouseWheel
		delta := (*qPoint)(unsafe.Add(ev, lay.wheelDelta)).Y
		m.Delta = float64(delta) / 120
	}
	emit(core.BackendEvent{Kind: event, H: h, Mouse: m})
}

func qtMouseButton(buttons int32) core.MouseButton {
	switch {
	case buttons&1 != 0:
		return core.MouseLeft
	case buttons&4 != 0:
		return core.MouseMiddle
	case buttons&2 != 0:
		return core.MouseRight
	default:
		return core.MouseNone
	}
}
