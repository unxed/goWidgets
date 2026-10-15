//go:build js && wasm

package web

import (
	"context"
	"testing"
	"time"

	"github.com/unxed/goWidgets/core"
)

func TestDriverContractInBrowser(t *testing.T) {
	d := &driver{}
	if got := d.Name(); got != "web" {
		t.Fatalf("Name() = %q, want web", got)
	}
	if err := d.Init(); err != nil {
		t.Fatalf("Init() in browser: %v", err)
	}

	want := core.Caps{
		NativeControls:  true,
		TreeView:        true,
		GridView:        true,
		FileDialog:      true,
		FolderDialog:    false,
		Menus:           true,
		Clipboard:       true,
		IME:             true,
		A11y:            true,
		SmoothAnimation: true,
		TrayIcon:        false,
		Canvas:          true,
		MaxCallbacks:    10000,
	}
	if got := d.Capabilities(); got != want {
		t.Fatalf("Capabilities() = %#v, want %#v", got, want)
	}
}

func TestWakeBeforeAndDuringMainLoop(t *testing.T) {
	d := &driver{}
	if err := d.Init(); err != nil {
		t.Fatalf("Init() in browser: %v", err)
	}

	// A wake requested before the loop starts must be delivered after its
	// initial pump. The second wake comes from a different goroutine while
	// the loop is active.
	d.Wake()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pumps := make(chan struct{}, 4)
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- d.RunMainLoop(ctx, func() { pumps <- struct{}{} })
	}()

	waitForPump(t, pumps) // initial pump
	waitForPump(t, pumps) // queued before RunMainLoop

	wakeReturned := make(chan struct{})
	go func() {
		d.Wake()
		close(wakeReturned)
	}()
	select {
	case <-wakeReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Wake() blocked when called from another goroutine")
	}
	waitForPump(t, pumps)

	cancel()
	select {
	case err := <-loopDone:
		if err != nil {
			t.Fatalf("RunMainLoop() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunMainLoop() did not return after context cancellation")
	}
}

func waitForPump(t *testing.T, pumps <-chan struct{}) {
	t.Helper()
	select {
	case <-pumps:
	case <-time.After(5 * time.Second):
		t.Fatal("RunMainLoop() did not pump after Wake()")
	}
}
