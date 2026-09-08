//go:build goWidgets_debug

package vreactive

import (
	"bytes"
	"fmt"
	"runtime"
	"strconv"
)

var mainG uint64

// BindMainThread records the current goroutine as the UI goroutine. Core calls
// it right after runtime.LockOSThread (§4.5.1); since that goroutine is pinned
// to its OS thread for the process lifetime, goroutine identity and thread
// identity coincide, and the goroutine id is portable across backends.
func BindMainThread() { mainG = goID() }

func assertMainThread(op string) {
	if mainG != 0 && goID() != mainG {
		panic(fmt.Sprintf(
			"vreactive: %s called off the UI goroutine (§4.5.2) — use App.QueueUpdate", op))
	}
}

// goID parses the goroutine id out of a stack header. Debug-only: this is slow
// and relies on runtime formatting, which is exactly why it is behind a tag.
func goID() uint64 {
	var buf [64]byte
	s := buf[:runtime.Stack(buf[:], false)]
	s = bytes.TrimPrefix(s, []byte("goroutine "))
	if i := bytes.IndexByte(s, ' '); i >= 0 {
		n, _ := strconv.ParseUint(string(s[:i]), 10, 64)
		return n
	}
	return 0
}
