# goWidgets Architectural Considerations & TODO List

## vreactive Layer
- [ ] Implement loop/cycle detection in `Property[T]` notify chains (using evaluation context or depth counter).
- [ ] Provide thread-safe mutation primitives for `Property[T]` when updated from background goroutines.
- [ ] Implement `DiscreteAnimator` (for vtui frame-based updates) and `SmoothAnimator` (for vgui tick-based updates) sharing a unified `Animator` interface.

## Core & Backend Integration
- [ ] Ensure `UpdateLayout` batching during window resizes to avoid redundant Cassowary solver recalculations.
- [ ] Web backend: benchmark DOM tree updates using CSS `position: absolute` with `requestAnimationFrame` batching.
- [ ] Verify `puregotk` dynamic library loading error handling on Linux distributions without GTK3 preinstalled.
