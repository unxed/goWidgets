//go:build !goWidgets_debug

package vreactive

// In release builds the affinity check compiles to nothing. Debug builds
// (-tags goWidgets_debug) panic with a readable message instead, per §4.5.5.

// BindMainThread is a no-op outside debug builds.
func BindMainThread() {}

func assertMainThread(string) {}
