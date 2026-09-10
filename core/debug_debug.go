//go:build goWidgets_debug

package core

// DebugBuild turns recoverable diagnostics into panics (§5.3): an
// unsatisfiable constraint is a programming error, and a debug build should
// stop on it rather than lay out without it.
const DebugBuild = true
