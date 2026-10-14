//go:build js && wasm

// Package web is the WebAssembly (browser) backend for goWidgets.
// It targets GOOS=js GOARCH=wasm and uses syscall/js to interact with the DOM.
package web

import (
	"context"
	"errors"
	"fmt"
	"syscall/js"

	"github.com/unxed/goWidgets/core"
)

func init() {
	// Register with build tag "web" so it's only included when GOOS=js GOARCH=wasm
	core.RegisterDriver("web", func() core.PlatformDriver { return &driver{} })
}

var (
	// ErrNotInBrowser is returned by Init when not running in a browser environment.
	ErrNotInBrowser = errors.New("web: not running in a browser (js.Global() is undefined)")
)

type driver struct {
	wake chan struct{}
}

func (d *driver) Name() string { return "web" }

func (d *driver) Init() error {
	// Check if we're running in a browser (js.Global() must be defined)
	global := js.Global()
	if global.IsUndefined() || global.IsNull() {
		return ErrNotInBrowser
	}
	d.wake = make(chan struct{}, 1)
	return nil
}

func (d *driver) Capabilities() core.Caps {
	return core.Caps{
		NativeControls:  true,  // DOM elements are native for the web
		TreeView:        true,  // via <div role="tree">
		GridView:        true,  // virtualized list
		FileDialog:      true,  // <input type=file>
		FolderDialog:    false, // not in browser
		Menus:           true,  // custom <menu> / contextmenu
		Clipboard:       true,  // navigator.clipboard
		IME:             true,  // composition events
		A11y:            true,  // aria-*, role, tabindex
		SmoothAnimation: true,  // CSS transitions / Web Animations API
		TrayIcon:        false, // only in Electron
		Canvas:          true,  // <canvas> 2D context
		MaxCallbacks:    10000,
	}
}

// RunMainLoop owns the UI thread until ctx is cancelled.
// In WASM, the event loop is single-threaded and event-driven.
// We call pump once before blocking, then wait for Wake signals via queueMicrotask.
func (d *driver) RunMainLoop(ctx context.Context, pump func()) error {
	// Initial pump call
	pump()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-d.wake:
			pump()
		}
	}
}

// Wake is safe to call from any goroutine.
// In WASM, we use queueMicrotask to schedule a microtask that will send on the wake channel.
func (d *driver) Wake() {
	// Use queueMicrotask to schedule a wakeup on the JS event loop
	js.Global().Call("queueMicrotask", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		select {
		case d.wake <- struct{}{}:
		default:
		}
		return nil
	}))
}

func (d *driver) CreateWindow(spec core.WindowSpec) (core.BackendWindow, error) {
	// W1 spike: window creation not implemented yet
	return nil, fmt.Errorf("web: CreateWindow not implemented in W1 spike")
}

func (d *driver) CreateTray(spec core.TraySpec) (core.BackendTray, error) {
	return nil, core.ErrNoTray
}

func (d *driver) Shutdown() {
	// No-op for W1
}
