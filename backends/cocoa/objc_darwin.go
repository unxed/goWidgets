//go:build darwin

package cocoa

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Objective-C through purego's objc package: objc_msgSend with the argument
// list classified by the actual Go types, so a CGFloat travels in a float
// register and an NSRect as the four-double aggregate the ABI says it is
// (on arm64 in d0–d3; objc.Send picks objc_msgSend_stret on amd64 where a
// large struct comes back through memory).

type (
	nsPoint struct{ X, Y float64 }
	nsSize  struct{ W, H float64 }
	nsRect  struct {
		X, Y, W, H float64
	}
)

var (
	selMu    sync.Mutex
	selCache = map[string]objc.SEL{}
)

// sel returns the selector for name, registering it once.
func sel(name string) objc.SEL {
	selMu.Lock()
	defer selMu.Unlock()
	s, ok := selCache[name]
	if !ok {
		s = objc.RegisterName(name)
		selCache[name] = s
	}
	return s
}

// cls returns a class as a receiver for class messages.
func cls(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

// msg sends a message whose result is an object or an integer.
func msg(id objc.ID, name string, args ...any) objc.ID { return id.Send(sel(name), args...) }

// msgT sends a message with a typed result: bool, a float, a struct.
func msgT[T any](id objc.ID, name string, args ...any) T { return objc.Send[T](id, sel(name), args...) }

// nsString returns an autoreleased NSString holding s.
func nsString(s string) objc.ID {
	return msg(cls("NSString"), "stringWithUTF8String:", s)
}

// goString copies an NSString into Go.
func goString(ns objc.ID) string {
	if ns == 0 {
		return ""
	}
	p := msgT[*byte](ns, "UTF8String")
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// nsArray returns an autoreleased NSArray of NSStrings.
func nsArray(items []string) objc.ID {
	a := msg(cls("NSMutableArray"), "array")
	for _, it := range items {
		msg(a, "addObject:", nsString(it))
	}
	return a
}

// alloc returns [[name alloc] init], owned by the caller.
func alloc(name string) objc.ID { return msg(msg(cls(name), "alloc"), "init") }

// cptr turns an address that came from C (dlsym) into an unsafe.Pointer.
// Reinterpreting the variable's storage instead of converting the integer
// is deliberate: the address is not Go memory, and vet's unsafeptr check is
// about Go memory.
func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

// Process-level C entry points.
var (
	pthreadMainNP     func() int32
	cfRunLoopGetMain  func() uintptr
	cfSourceCreate    func(allocator uintptr, order int64, ctx *[10]uintptr) uintptr
	cfRunLoopAddSrc   func(loop, source, mode uintptr)
	cfSourceSignal    func(source uintptr)
	cfRunLoopWakeUp   func(loop uintptr)
	runLoopModes      []uintptr // the modes our source is served in
	wakeSourceContext [10]uintptr
)

// globalString reads an exported CFStringRef/NSString * constant.
func globalString(lib uintptr, name string) uintptr {
	a, err := purego.Dlsym(lib, name)
	if err != nil || a == 0 {
		return 0
	}
	return *(*uintptr)(cptr(a))
}

func loadFrameworks() error {
	open := func(path string) (uintptr, error) {
		h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		return h, nil
	}
	cf, err := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	if err != nil {
		return err
	}
	if _, err := open("/System/Library/Frameworks/Foundation.framework/Foundation"); err != nil {
		return err
	}
	appKit, err := open("/System/Library/Frameworks/AppKit.framework/AppKit")
	if err != nil {
		return err
	}
	lib, err := open("/usr/lib/libobjc.A.dylib")
	if err != nil {
		return err
	}
	sys, err := open("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&msgSendSuper2, lib, "objc_msgSendSuper2")
	purego.RegisterLibFunc(&pthreadMainNP, sys, "pthread_main_np")
	purego.RegisterLibFunc(&cfRunLoopGetMain, cf, "CFRunLoopGetMain")
	purego.RegisterLibFunc(&cfSourceCreate, cf, "CFRunLoopSourceCreate")
	purego.RegisterLibFunc(&cfRunLoopAddSrc, cf, "CFRunLoopAddSource")
	purego.RegisterLibFunc(&cfSourceSignal, cf, "CFRunLoopSourceSignal")
	purego.RegisterLibFunc(&cfRunLoopWakeUp, cf, "CFRunLoopWakeUp")
	// The common modes, and the two AppKit modes named outright: the modal
	// loop of an alert or a panel is where a wake-up must get through too,
	// and the main dispatch queue — tried first — does not (seen in CI: a
	// QueueUpdate posted under NSAlert's runModal never ran).
	for _, m := range []uintptr{
		globalString(cf, "kCFRunLoopCommonModes"),
		globalString(appKit, "NSModalPanelRunLoopMode"),
		globalString(appKit, "NSEventTrackingRunLoopMode"),
	} {
		if m != 0 {
			runLoopModes = append(runLoopModes, m)
		}
	}
	if len(runLoopModes) == 0 {
		return fmt.Errorf("CoreFoundation: no run loop modes")
	}
	return nil
}
