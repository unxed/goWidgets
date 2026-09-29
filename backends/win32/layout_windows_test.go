//go:build windows

package win32

import (
	"testing"
	"unsafe"

	"github.com/unxed/goWidgets/core"
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

// The public window size is the client area. CreateWindowEx receives the
// outer frame size, so it must be larger or the first auto-layout pass places
// the last controls under the non-client area until a later repaint/resize.
func TestOuterWindowSizeContainsRequestedClient(t *testing.T) {
	w, h := outerWindowSize(core.Size{W: 660, H: 700})
	if w < 660 || h < 700 {
		t.Fatalf("outer size = %dx%d, smaller than requested client area", w, h)
	}
}
