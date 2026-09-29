//go:build linux

package qttest

import "unsafe"

// uintptrOf passes a Go value to a Qt call as a const reference. The value
// must stay reachable for the call, which the caller's frame guarantees.
func uintptrOf[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }
