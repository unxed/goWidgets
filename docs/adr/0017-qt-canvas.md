# ADR-0017: Canvas on Qt 5 and Qt 6

- Status: accepted
- Date: 2026-09-30
- Plan: [`imgy/PLAN.md` §B5](../imgy/PLAN.md)
- Builds on: [ADR-0016](0016-canvas-surface.md)

## Context

The Qt 5 and Qt 6 drivers need to present the shared Go-owned `image.RGBA`
Canvas and forward pointer input without adding cgo or making the driver draw
through Qt APIs. Qt has no C API, so the backend already uses purego bindings
to its C++ ABI.

## Decision

- Use a child `QLabel` with scaled contents for the Canvas widget.
- Encode each completed RGBA frame as PNG in Go, load it into a temporary
  `QPixmap`, and set that pixmap on the label. The frame remains the canonical
  drawable surface; Qt only presents it.
- Keep input routing on the existing application event filter. Convert the
  global cursor position into Canvas-local coordinates and map Qt button and
  modifier values to the shared event model.
- Read Qt-major-specific inline event fields only at offsets measured by the
  CI C++ probe. CI asserts Qt 5/6 `QWheelEvent` and `QMouseEvent` offsets and
  runs real Xvfb click/wheel plus pixel/screenshot checks for both majors.
- Advertise Canvas only when the presenter and required ABI entry points are
  available. Unsupported Qt installations must continue to fall back cleanly.
- Keep all view sizing in core Auto Layout; the Qt backend only applies solved
  bounds.

## Consequences

- Qt 5 and Qt 6 Canvas presentation and pointer input are covered separately
  in Linux CI; no cgo or new Go module dependency is required.
- Inline C++ ABI offsets remain an explicit maintenance risk. The probe and
  runtime test must be updated together if the supported Qt ABI changes.
- Cocoa remains the only backend without Canvas and is the next parity step.
- The decision preserves ADR-0016: Go owns drawing; backends only present the
  buffer and forward input.
