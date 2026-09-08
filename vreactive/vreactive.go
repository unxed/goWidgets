// Package vreactive is the reactive layer shared by goWidgets and vtui.
//
// It imports nothing from goWidgets or core, per the layering rule in §7 of the
// design document.
//
// Contract: §4.2. Invariants implemented here:
//   - notifications are delivered after the enclosing Batch commits, so no
//     subscriber ever observes a half-applied group of changes ("glitch-free");
//   - Set with an equal value produces no notification;
//   - cascade depth is capped by MaxPropagationDepth and reports an error
//     instead of hanging;
//   - subscriptions are owned by a Scope and die with it.
//
// Thread affinity (§4.5) is enforced by the caller, not here: everything in
// this package expects to run on the main thread. Debug builds check it — see
// affinity_debug.go.
package vreactive

import (
	"fmt"
	"sync"
)

// MaxPropagationDepth bounds a notification cascade. Exceeding it is a bug in
// the user's dependency graph; we surface it rather than spin forever.
const MaxPropagationDepth = 64

// Scope owns subscriptions. Close unsubscribes everything at once — the single
// countermeasure against the leak class tracked by TestNoLeakedSubscriptions.
type Scope struct {
	mu     sync.Mutex
	closed bool
	undo   []func()
}

// NewScope returns an empty scope.
func NewScope() *Scope { return &Scope{} }

func (s *Scope) add(undo func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		undo()
		return
	}
	s.undo = append(s.undo, undo)
}

// Close drops every subscription registered through this scope.
func (s *Scope) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	undo := s.undo
	s.undo = nil
	s.mu.Unlock()
	for i := len(undo) - 1; i >= 0; i-- {
		undo[i]()
	}
}

// Len reports how many live subscriptions the scope owns. Used by leak tests.
func (s *Scope) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.undo)
}

// ---------------------------------------------------------------- batching

var (
	batchDepth int
	pending    []func()
	depth      int
)

// Batch coalesces notifications: subscribers are woken once, after f returns.
func Batch(f func()) {
	batchDepth++
	defer func() {
		batchDepth--
		if batchDepth == 0 {
			flush()
		}
	}()
	f()
}

func flush() {
	depth++
	defer func() { depth-- }()
	if depth > MaxPropagationDepth {
		panic(fmt.Sprintf("vreactive: propagation depth exceeded %d — cyclic binding?", MaxPropagationDepth))
	}
	for len(pending) > 0 {
		batch := pending
		pending = nil
		for _, fn := range batch {
			fn()
		}
	}
}

func schedule(fn func()) {
	pending = append(pending, fn)
	if batchDepth == 0 {
		flush()
	}
}

// ---------------------------------------------------------------- Property

// Property is a value with change notifications. Contract: §4.2.
type Property[T comparable] struct {
	v    T
	subs map[int]func(newV, oldV T)
	next int
}

// NewProperty seeds a property with an initial value.
func NewProperty[T comparable](v T) *Property[T] {
	return &Property[T]{v: v}
}

// Get returns the current value.
func (p *Property[T]) Get() T { return p.v }

// Set replaces the value and notifies subscribers. Setting an equal value is a
// no-op, which is what keeps idle UIs at 0% CPU (NFR "Простой UI").
func (p *Property[T]) Set(v T) {
	assertMainThread("Property.Set")
	if p.v == v {
		return
	}
	old := p.v
	p.v = v
	for _, h := range p.subs {
		h := h
		schedule(func() { h(v, old) })
	}
}

// Update applies f to the current value atomically with respect to notification
// ordering.
func (p *Property[T]) Update(f func(T) T) { p.Set(f(p.v)) }

// OnChange subscribes h for the lifetime of s.
func (p *Property[T]) OnChange(s *Scope, h func(newV, oldV T)) {
	assertMainThread("Property.OnChange")
	if p.subs == nil {
		p.subs = map[int]func(T, T){}
	}
	p.next++
	id := p.next
	p.subs[id] = h
	s.add(func() { delete(p.subs, id) })
}

// Bind keeps dst equal to src. Dependencies are declared explicitly; automatic
// dependency tracking is deliberately out of scope for v1 (§4.2).
func Bind[T comparable](s *Scope, dst, src *Property[T]) {
	dst.Set(src.Get())
	src.OnChange(s, func(newV, _ T) { dst.Set(newV) })
}

// ------------------------------------------------------------------- Event

// Event is a multicast signal. Contract: §4.2.
type Event[T any] struct {
	subs map[int]func(T)
	next int
}

// NewEvent returns an event with no subscribers.
func NewEvent[T any]() *Event[T] { return &Event[T]{} }

// Emit delivers v to every subscriber.
func (e *Event[T]) Emit(v T) {
	assertMainThread("Event.Emit")
	for _, h := range e.subs {
		h := h
		schedule(func() { h(v) })
	}
}

// On subscribes h for the lifetime of s.
func (e *Event[T]) On(s *Scope, h func(T)) {
	assertMainThread("Event.On")
	if e.subs == nil {
		e.subs = map[int]func(T){}
	}
	e.next++
	id := e.next
	e.subs[id] = h
	s.add(func() { delete(e.subs, id) })
}
