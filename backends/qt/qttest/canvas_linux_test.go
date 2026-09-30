//go:build linux

package qttest

import (
	"image"
	"image/color"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
	"github.com/unxed/goWidgets/core"
)

func TestCanvasPaintAndPointer(t *testing.T) {
	app := perMajor(t)
	if app == nil {
		return
	}
	win, err := app.NewWindow("Qt Canvas test", 400, 300)
	if err != nil {
		t.Fatal(err)
	}
	canvas, err := win.AddCanvas()
	if err != nil {
		for _, symbol := range []string{
			"_ZN7QWidget12setAttributeENS_15WidgetAttributeEb",
			"_ZNK7QWidget13mapFromGlobalERK6QPoint",
			"_ZN7QCursor3posEv",
			"_ZN15QGuiApplication17keyboardModifiersEv",
			"_ZN7QPixmap12loadFromDataEPKhjPKc6QFlagsIN2Qt19ImageConversionFlagEE",
			"_ZN7QPixmapC1Ev",
		} {
			t.Logf("Qt %d symbol %s = %#x", qt.Version(), symbol, qt.Sym(symbol))
		}
		t.Fatal(err)
	}
	want := color.RGBA{R: 51, G: 102, B: 153, A: 255}
	canvas.Paint.On(app.Scope(), func(frame *goWidgets.CanvasFrame) {
		for i := 0; i < len(frame.Image.Pix); i += 4 {
			copy(frame.Image.Pix[i:i+4], []byte{want.R, want.G, want.B, want.A})
		}
	})
	var down, wheel bool
	var wheelDelta float64
	var mouse goWidgets.MouseInfo
	var button core.MouseButton
	canvas.MouseDown.On(app.Scope(), func(m goWidgets.MouseInfo) { down, mouse, button = true, m, m.Button })
	canvas.MouseWheel.On(app.Scope(), func(m goWidgets.MouseInfo) { wheel, wheelDelta, mouse = true, m.Delta, m })

	screenshot := ""
	if dir := os.Getenv("GOWIDGETS_SNAPSHOT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		screenshot = filepath.Join(dir, "qt-canvas.png")
	}
	var inputErr error
	go func() {
		time.Sleep(350 * time.Millisecond)
		// The only child is the stretchy Canvas. Use the client-window center,
		// which stays inside it regardless of window-manager decorations.
		id, err := exec.Command("xdotool", "search", "--onlyvisible", "--name", "Qt Canvas test").Output()
		if err != nil {
			inputErr = err
		} else {
			ids := strings.Fields(string(id))
			if len(ids) == 0 {
				inputErr = os.ErrNotExist
			} else {
				windowID := ids[0]
				geometry, err := exec.Command("xdotool", "getwindowgeometry", "--shell", windowID).Output()
				if err != nil {
					inputErr = err
				} else {
					values := map[string]int{}
					for _, line := range strings.Split(string(geometry), "\n") {
						p := strings.SplitN(line, "=", 2)
						if len(p) == 2 {
							values[p[0]], _ = strconv.Atoi(p[1])
						}
					}
					x := values["X"] + values["WIDTH"]/2
					y := values["Y"] + values["HEIGHT"]/2
					inputErr = exec.Command("xdotool", "windowraise", windowID, "mousemove", "--sync", strconv.Itoa(x), strconv.Itoa(y), "click", "1").Run()
					if inputErr == nil {
						time.Sleep(80 * time.Millisecond)
						inputErr = exec.Command("xdotool", "windowraise", windowID, "mousemove", "--sync", strconv.Itoa(x), strconv.Itoa(y), "click", "4").Run()
					}
				}
			}
		}
		time.Sleep(180 * time.Millisecond)
		if screenshot != "" {
			if err := exec.Command("scrot", screenshot).Run(); err != nil {
				inputErr = err
			}
		}
		app.QueueUpdate(func() { app.Quit() })
	}()
	if err := app.Run(win); err != nil {
		t.Fatal(err)
	}
	if inputErr != nil {
		t.Fatalf("drive Qt Canvas pointer: %v", inputErr)
	}
	if !down || button != core.MouseLeft || !wheel || wheelDelta != 1 || mouse.X <= 0 || mouse.Y <= 0 {
		t.Errorf("Canvas input down=%v button=%v wheel=%v delta=%v position=(%v,%v); want left down, wheel +1 and local coordinates", down, button, wheel, wheelDelta, mouse.X, mouse.Y)
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
		found := false
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y && !found; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				if got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA); got == want {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("Qt Canvas screenshot contains no painted pixel %#v", want)
		}
	}
}
