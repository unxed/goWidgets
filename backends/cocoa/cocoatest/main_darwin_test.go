//go:build darwin

// Package cocoatest exercises the Cocoa driver against a real AppKit.
//
// AppKit runs on the main thread only, and testing.M runs each test on a
// goroutine of its own. So TestMain keeps the main goroutine — locked to
// the main thread by the cocoa package's init — and runs there whatever the
// tests hand it: every scenario below goes through onMain, from NewApp to
// the end of Run. Scenarios report with t.Error, never t.Fatal: Goexit on
// the main goroutine would end TestMain itself.
package cocoatest

import (
	"os"
	"testing"

	"github.com/unxed/goWidgets"
	"github.com/unxed/goWidgets/backends/cocoa"
)

var mainQueue = make(chan func())

func TestMain(m *testing.M) {
	done := make(chan int)
	go func() { done <- m.Run() }()
	for {
		select {
		case f := <-mainQueue:
			f()
		case code := <-done:
			os.Exit(code)
		}
	}
}

// onMain runs f on the main thread and waits for it.
func onMain(f func()) {
	finished := make(chan struct{})
	mainQueue <- func() {
		defer close(finished)
		f()
	}
	<-finished
}

// newApp is NewApp on the Cocoa driver, or a skip.
func newApp(t *testing.T) *goWidgets.App {
	t.Helper()
	os.Setenv("goWidgets_BACKEND", "cocoa")
	app, err := goWidgets.NewApp()
	if err != nil {
		t.Errorf("NewApp: %v", err)
		return nil
	}
	return app
}

var _ = cocoa.WindowHandle
