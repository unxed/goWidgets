//go:build linux

// Package qttest exercises the Qt driver against a real Qt, 6 and 5 both.
//
// A process can host one QApplication of one Qt major, so every test re-runs
// itself in a child process per major: the parent is a thin dispatcher with
// a subtest per version, the child (goWidgets_BACKEND set) is the test. A
// major that is not installed, or no display, skips its subtest rather than
// failing it.
package qttest

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/qt"
)

const childEnv = "GOWIDGETS_QTTEST_CHILD"

// perMajor runs the calling test once per Qt major, each in its own process.
// It returns an application in the child, nil in the parent (which is done).
func perMajor(t *testing.T) *goWidgets.App {
	t.Helper()
	if os.Getenv(childEnv) == "" {
		for _, major := range []string{"qt6", "qt5"} {
			t.Run(major, func(t *testing.T) {
				cmd := exec.Command(os.Args[0], "-test.run=^"+strings.Split(t.Name(), "/")[0]+"$", "-test.v", "-test.count=1")
				cmd.Env = append(os.Environ(), childEnv+"=1", "goWidgets_BACKEND="+major)
				out, err := cmd.CombinedOutput()
				text := string(out)
				switch {
				case strings.Contains(text, "--- SKIP"):
					t.Skipf("%s", lastLines(text, 3))
				case err != nil:
					t.Fatalf("%s: %v\n%s", major, err, text)
				default:
					t.Logf("%s", lastLines(text, 6))
				}
			})
		}
		return nil
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("нет дисплея: запускать под Xvfb")
	}
	app, err := goWidgets.NewApp()
	// NewApp locks this goroutine to its thread. A locked goroutine that
	// ends takes its thread with it, and fakecgo's thread entry does not
	// survive that exit: the process crashed after the test had passed.
	// Unlocking first hands the thread back to the scheduler instead.
	t.Cleanup(runtime.UnlockOSThread)
	if err != nil {
		// A Qt that is not installed, or cannot reach a display, is a
		// skip. Anything else — a missing symbol above all — is a bug.
		for _, why := range []string{"not available", "no display", "not reachable", "platform plugin"} {
			if strings.Contains(err.Error(), why) {
				t.Skipf("Qt недоступен: %v", err)
			}
		}
		t.Fatal(err)
	}
	if qt.Version() == 0 {
		t.Skipf("драйвер %s, а проверяется qt", app.Diagnostics().Name)
	}
	return app
}

func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}

// drive runs steps on the UI thread one turn apart, waits, and quits. Each
// step is its own QueueUpdate, so a property write in one has reached the
// widget before the next looks.
func drive(app *goWidgets.App, settle time.Duration, steps ...func()) {
	go func() {
		time.Sleep(settle)
		for _, s := range steps {
			done := make(chan struct{})
			app.QueueUpdate(func() { s(); close(done) })
			<-done
			time.Sleep(50 * time.Millisecond)
		}
		app.QueueUpdate(func() {})
		time.Sleep(100 * time.Millisecond)
		app.Quit()
	}()
}

type rect struct{ X, Y, W, H int }

func geometry(w uintptr) rect {
	x, y, wd, ht := qt.Geometry(w)
	return rect{x, y, wd, ht}
}
