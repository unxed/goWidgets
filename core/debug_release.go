//go:build !goWidgets_debug

package core

// DebugBuild is false outside -tags goWidgets_debug: conflicts are dropped and
// reported through Diagnostics() instead of panicking.
const DebugBuild = false
