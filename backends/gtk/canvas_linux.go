//go:build linux

package gtk

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets/core"
)

const canvasEventMask = int32(1<<2 | 1<<8 | 1<<9 | 1<<21) // motion, button, release, scroll

var (
	gtkDrawingAreaNew  func() uintptr
	gtkWidgetAddEvents func(widget uintptr, events int32)
	gtkWidgetQueueDraw func(widget uintptr)
	gdkEventGetCoords  func(event uintptr, x, y *float64) int32
	gdkEventGetButton  func(event uintptr, button *uint32) int32
	gdkEventGetState   func(event uintptr, state *uint32) int32
	gdkScrollDeltas    func(event uintptr, dx, dy *float64) int32
	gdkScrollDirection func(event uintptr, direction *int32) int32

	cairoSurfaceCreate  func(format, width, height int32) uintptr
	cairoSurfaceData    func(surface uintptr) *byte
	cairoSurfaceStride  func(surface uintptr) int32
	cairoSurfaceDirty   func(surface uintptr)
	cairoSurfaceDestroy func(surface uintptr)
	cairoSurfaceStatus  func(surface uintptr) int32
	cairoSave           func(cr uintptr)
	cairoRestore        func(cr uintptr)
	cairoScale          func(cr uintptr, x, y float64)
	cairoSourceSurface  func(cr, surface uintptr, x, y float64)
	cairoPaint          func(cr uintptr)
)

func (d *driver) initCanvas(gtkLib uintptr) error {
	gdk, err := dlopenAny("libgdk-3.so.0", "libgdk-3.so")
	if err != nil {
		return fmt.Errorf("GTK GDK unavailable: %w", err)
	}
	cairo, err := dlopenAny("libcairo.so.2", "libcairo.so")
	if err != nil {
		return fmt.Errorf("Cairo unavailable: %w", err)
	}
	purego.RegisterLibFunc(&gtkDrawingAreaNew, gtkLib, "gtk_drawing_area_new")
	purego.RegisterLibFunc(&gtkWidgetAddEvents, gtkLib, "gtk_widget_add_events")
	purego.RegisterLibFunc(&gtkWidgetQueueDraw, gtkLib, "gtk_widget_queue_draw")
	purego.RegisterLibFunc(&gdkEventGetCoords, gdk, "gdk_event_get_coords")
	purego.RegisterLibFunc(&gdkEventGetButton, gdk, "gdk_event_get_button")
	purego.RegisterLibFunc(&gdkEventGetState, gdk, "gdk_event_get_state")
	purego.RegisterLibFunc(&gdkScrollDeltas, gdk, "gdk_event_get_scroll_deltas")
	purego.RegisterLibFunc(&gdkScrollDirection, gdk, "gdk_event_get_scroll_direction")
	purego.RegisterLibFunc(&cairoSurfaceCreate, cairo, "cairo_image_surface_create")
	purego.RegisterLibFunc(&cairoSurfaceData, cairo, "cairo_image_surface_get_data")
	purego.RegisterLibFunc(&cairoSurfaceStride, cairo, "cairo_image_surface_get_stride")
	purego.RegisterLibFunc(&cairoSurfaceDirty, cairo, "cairo_surface_mark_dirty")
	purego.RegisterLibFunc(&cairoSurfaceDestroy, cairo, "cairo_surface_destroy")
	purego.RegisterLibFunc(&cairoSurfaceStatus, cairo, "cairo_surface_status")
	purego.RegisterLibFunc(&cairoSave, cairo, "cairo_save")
	purego.RegisterLibFunc(&cairoRestore, cairo, "cairo_restore")
	purego.RegisterLibFunc(&cairoScale, cairo, "cairo_scale")
	purego.RegisterLibFunc(&cairoSourceSurface, cairo, "cairo_set_source_surface")
	purego.RegisterLibFunc(&cairoPaint, cairo, "cairo_paint")

	d.cbCanvasDraw = purego.NewCallback(func(widget, cr, data uintptr) uintptr {
		if d.win != nil {
			if n := d.win.nodes[core.Handle(data)]; n != nil && n.canvasSurface != 0 {
				scale := n.canvasFrame.Scale
				if scale <= 0 {
					scale = 1
				}
				cairoSave(cr)
				cairoScale(cr, 1/scale, 1/scale)
				cairoSourceSurface(cr, n.canvasSurface, 0, 0)
				cairoPaint(cr)
				cairoRestore(cr)
			}
		}
		return 0
	})
	for i, kind := range []core.EventKind{core.EventMouseDown, core.EventMouseUp, core.EventMouseMove, core.EventMouseWheel} {
		kind := kind
		d.cbCanvasInput[i] = purego.NewCallback(func(widget, event, data uintptr) uintptr {
			d.canvasInput(kind, event, core.Handle(data))
			return 0
		})
	}
	d.cbCanvasScale = purego.NewCallback(func(object, pspec, data uintptr) uintptr {
		if d.win != nil {
			f := float64(gtkScaleFactor(d.win.handle))
			if f <= 0 {
				f = 1
			}
			emit(core.BackendEvent{Kind: core.EventScaleChanged, Scale: core.ScaleInfo{Scale: f, FontScale: 1}})
		}
		return 0
	})
	return nil
}

func (w *window) PresentCanvas(h core.Handle, frame *core.CanvasFrame) {
	n := w.nodes[h]
	if n == nil || frame == nil || frame.Image == nil {
		return
	}
	width, height := int32(frame.Image.Rect.Dx()), int32(frame.Image.Rect.Dy())
	if width <= 0 || height <= 0 {
		return
	}
	if n.canvasSurface == 0 || n.canvasWidth != width || n.canvasHeight != height {
		if n.canvasSurface != 0 {
			cairoSurfaceDestroy(n.canvasSurface)
		}
		// CAIRO_FORMAT_ARGB32 is native-endian BGRA bytes on little endian.
		n.canvasSurface = cairoSurfaceCreate(0, width, height)
		if n.canvasSurface == 0 || cairoSurfaceStatus(n.canvasSurface) != 0 {
			if n.canvasSurface != 0 {
				cairoSurfaceDestroy(n.canvasSurface)
				n.canvasSurface = 0
			}
			return
		}
		n.canvasWidth, n.canvasHeight = width, height
	}
	stride := cairoSurfaceStride(n.canvasSurface)
	data := cairoSurfaceData(n.canvasSurface)
	if data == nil || stride < width*4 {
		return
	}
	packed := make([]byte, width*height*4)
	if err := core.ConvertRGBA(packed, frame.Image, core.PixelBGRA); err != nil {
		return
	}
	dst := unsafe.Slice(data, int(stride*height))
	for row := int32(0); row < height; row++ {
		copy(dst[row*stride:row*stride+width*4], packed[row*width*4:(row+1)*width*4])
	}
	runtime.KeepAlive(packed)
	cairoSurfaceDirty(n.canvasSurface)
	n.canvasFrame = frame
	gtkWidgetQueueDraw(n.handle)
}

func (d *driver) canvasInput(kind core.EventKind, event uintptr, h core.Handle) {
	var x, y float64
	gdkEventGetCoords(event, &x, &y)
	m := core.MouseInfo{X: x, Y: y}
	var state uint32
	gdkEventGetState(event, &state)
	if state&1 != 0 {
		m.Mods |= core.ModShift
	}
	if state&4 != 0 {
		m.Mods |= core.ModControl
	}
	if state&8 != 0 {
		m.Mods |= core.ModAlt
	}
	if kind == core.EventMouseDown || kind == core.EventMouseUp {
		var button uint32
		gdkEventGetButton(event, &button)
		switch button {
		case 1:
			m.Button = core.MouseLeft
		case 2:
			m.Button = core.MouseMiddle
		case 3:
			m.Button = core.MouseRight
		}
	}
	if kind == core.EventMouseWheel {
		var dx, dy float64
		if gdkScrollDeltas(event, &dx, &dy) != 0 {
			m.Delta = -dy
		} else {
			var direction int32
			gdkScrollDirection(event, &direction)
			switch direction {
			case 0:
				m.Delta = 1
			case 1:
				m.Delta = -1
			}
		}
	}
	emit(core.BackendEvent{Kind: kind, H: h, Mouse: m})
}
