//go:build windows

package win32

import (
	"testing"
	"unsafe"
)

// The OPENFILENAMEW mirror must be laid out exactly as the C struct, or
// comdlg32 reads garbage; 152 bytes is the documented 64-bit size.
func TestOpenFileNameLayout(t *testing.T) {
	if sz := unsafe.Sizeof(openFileNameW{}); sz != 152 {
		t.Fatalf("sizeof(openFileNameW) = %d, want 152", sz)
	}
	var o openFileNameW
	if off := unsafe.Offsetof(o.lpstrFile); off != 48 {
		t.Fatalf("lpstrFile at %d, want 48", off)
	}
	if off := unsafe.Offsetof(o.flags); off != 96 {
		t.Fatalf("flags at %d, want 96", off)
	}
}
