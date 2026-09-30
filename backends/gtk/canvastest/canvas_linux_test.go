//go:build linux

package canvastest

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/gtk"
	"github.com/unxed/goWidgets/core"
)

func TestCanvasDrawPointerAndScreenshot(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("run under Xvfb")
	}
	app, err := goWidgets.NewApp()
	defer runtime.UnlockOSThread()
	if err != nil {
		t.Skipf("GTK unavailable: %v", err)
	}
	if app.Diagnostics().Name != "gtk" {
		t.Skipf("selected %s, not GTK", app.Diagnostics().Name)
	}
	win, err := app.NewWindow("Canvas test", 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := win.AddCanvas()
	if err != nil {
		t.Fatal(err)
	}
	want := color.RGBA{R: 51, G: 102, B: 153, A: 255}
	canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
		for i := 0; i < len(frame.Image.Pix); i += 4 {
			frame.Image.Pix[i+0] = want.R
			frame.Image.Pix[i+1] = want.G
			frame.Image.Pix[i+2] = want.B
			frame.Image.Pix[i+3] = want.A
		}
	})
	var down, wheel bool
	canvas.MouseDown.On(app.Scope(), func(goWidgets.MouseInfo) { down = true })
	canvas.MouseWheel.On(app.Scope(), func(goWidgets.MouseInfo) { wheel = true })

	gtkLib, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	cairoLib, err := purego.Dlopen("libcairo.so.2", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		gtkWindowPresent    func(uintptr)
		gtkWidgetGetEvents  func(uintptr) int32
		gtkWidgetGetWindow  func(uintptr) uintptr
		gtkWidgetTranslate  func(uintptr, uintptr, int32, int32, *int32, *int32) int32
		gtkWidgetDraw       func(uintptr, uintptr)
		gtkAllocW           func(uintptr) int32
		gtkAllocH           func(uintptr) int32
		cairoImageSurface   func(int32, int32, int32) uintptr
		cairoCreate         func(uintptr) uintptr
		cairoDestroy        func(uintptr)
		cairoSurfaceDestroy func(uintptr)
		cairoSurfaceFlush   func(uintptr)
		cairoWritePNG       func(uintptr, string) int32
	)
	purego.RegisterLibFunc(&gtkWindowPresent, gtkLib, "gtk_window_present")
	purego.RegisterLibFunc(&gtkWidgetGetEvents, gtkLib, "gtk_widget_get_events")
	purego.RegisterLibFunc(&gtkWidgetGetWindow, gtkLib, "gtk_widget_get_window")
	purego.RegisterLibFunc(&gtkWidgetTranslate, gtkLib, "gtk_widget_translate_coordinates")
	gdkLib, err := purego.Dlopen("libgdk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var gdkX11WindowGetXID func(uintptr) uint64
	purego.RegisterLibFunc(&gdkX11WindowGetXID, gdkLib, "gdk_x11_window_get_xid")
	purego.RegisterLibFunc(&gtkWidgetDraw, gtkLib, "gtk_widget_draw")
	purego.RegisterLibFunc(&gtkAllocW, gtkLib, "gtk_widget_get_allocated_width")
	purego.RegisterLibFunc(&gtkAllocH, gtkLib, "gtk_widget_get_allocated_height")
	purego.RegisterLibFunc(&cairoImageSurface, cairoLib, "cairo_image_surface_create")
	purego.RegisterLibFunc(&cairoCreate, cairoLib, "cairo_create")
	purego.RegisterLibFunc(&cairoDestroy, cairoLib, "cairo_destroy")
	purego.RegisterLibFunc(&cairoSurfaceDestroy, cairoLib, "cairo_surface_destroy")
	purego.RegisterLibFunc(&cairoSurfaceFlush, cairoLib, "cairo_surface_flush")
	purego.RegisterLibFunc(&cairoWritePNG, cairoLib, "cairo_surface_write_to_png")
	windowWidget := gtk.WindowHandle()
	canvasWidget := gtk.WidgetHandle(core.KindCanvas)

	var screenshot string
	if dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		screenshot = filepath.Join(dir, "gtk-canvas.png")
	}
	var inputErr error
	var eventMask int32
	go func() {
		time.Sleep(350 * time.Millisecond)
		pointerAt := make(chan [3]int, 1)
		app.QueueUpdate(func() {
			gtkWindowPresent(windowWidget)
			eventMask = gtkWidgetGetEvents(canvasWidget)
			var canvasX, canvasY int32
			xid := gdkX11WindowGetXID(gtkWidgetGetWindow(windowWidget))
			if xid == 0 || gtkWidgetTranslate(canvasWidget, windowWidget, gtkAllocW(canvasWidget)/2, gtkAllocH(canvasWidget)/2, &canvasX, &canvasY) == 0 {
				pointerAt <- [3]int{-1, -1, -1}
				return
			}
			pointerAt <- [3]int{int(xid), int(canvasX), int(canvasY)}
		})
		coords := <-pointerAt
		if coords[0] < 0 {
			inputErr = fmt.Errorf("could not resolve GTK Canvas screen coordinates")
		} else {
			inputErr = exec.Command("xdotool", "windowraise", fmt.Sprint(coords[0]), "mousemove", "--window", fmt.Sprint(coords[0]), fmt.Sprint(coords[1]), fmt.Sprint(coords[2]), "click", "1", "click", "4").Run()
		}
		time.Sleep(180 * time.Millisecond)
		app.QueueUpdate(func() {
			if screenshot != "" {
				if err := captureCanvas(gtk.WidgetHandle(core.KindCanvas), screenshot, gtkAllocW, gtkAllocH, gtkWidgetDraw, cairoImageSurface, cairoCreate, cairoDestroy, cairoSurfaceDestroy, cairoSurfaceFlush, cairoWritePNG); err != nil {
					t.Errorf("save GTK Canvas screenshot: %v", err)
				}
			}
			app.Quit()
		})
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if screenshot != "" {
		f, err := os.Open(screenshot)
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := color.RGBAModel.Convert(img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2)).(color.RGBA)
		if got != want {
			t.Fatalf("Canvas screenshot center = %#v, want %#v", got, want)
		}
	}
	if !down || !wheel {
		t.Fatalf("GTK pointer events down=%v wheel=%v, mask=%#x, xdotool err=%v; expected Canvas-center input", down, wheel, eventMask, inputErr)
	}
}

func captureCanvas(widget uintptr, path string, width, height func(uintptr) int32, drawWidget func(uintptr, uintptr),
	createSurface func(int32, int32, int32) uintptr, createContext func(uintptr) uintptr, destroyContext, destroySurface func(uintptr),
	flush func(uintptr), writePNG func(uintptr, string) int32) error {
	w, h := width(widget), height(widget)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("canvas has no allocation: %dx%d", w, h)
	}
	surface := createSurface(0, w, h)
	if surface == 0 {
		return fmt.Errorf("cairo could not allocate %dx%d surface", w, h)
	}
	defer destroySurface(surface)
	cr := createContext(surface)
	if cr == 0 {
		return fmt.Errorf("cairo could not create context")
	}
	drawWidget(widget, cr)
	destroyContext(cr)
	flush(surface)
	if status := writePNG(surface, path); status != 0 {
		return fmt.Errorf("cairo PNG writer status %d", status)
	}
	return nil
}
