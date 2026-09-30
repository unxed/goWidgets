# ADR-0016. Canvas drawing surface

* Status: accepted
* Date: 2026-09-30
* Affects: public widget API, backend contract, DPI behavior

## Context

The `imgy` showcase needs one drawing surface for decoded images, selection
overlays, and later thumbnail browsing. Existing vtui vocabulary names cover
terminal pixel storage (`ImageSurface`) and terminal image placement
(`GraphicsLayer`), not an interactive GUI drawing widget. The user approved
`Canvas` as the goWidgets/vtui widget name.

## Decision

- Expose `Window.AddCanvas`, with a reusable top-down `*image.RGBA` frame,
  `Paint`, `Invalidate`, `Scale`, and mouse down/up/move/wheel events.
- Layout bounds and pointer coordinates are DIP. The image buffer dimensions
  are `ceil(bounds × scale)` physical pixels; the `Scale` property follows the
  window's monitor scale.
- Multiple invalidations coalesce until the next frame. A layout change only
  repaints a Canvas when its solved bounds change; DPI changes always repaint.
- A backend must declare `Caps.Canvas` and implement `CanvasPresenter`; until
  then `AddCanvas` returns `ErrCanvasUnsupported`. Headless is the first
  implementation and the contract gate. Native support is added per backend
  before claiming parity.
- `core.ConvertRGBA` is the shared RGBA/BGRA/ARGB byte-order conversion point.

## Consequences

Image decoding and drawing remain application-owned and backend-independent.
The first phase does not include partial invalidation, text, or native backend
implementations. B2/B3/B5 remain required before `imgy` runs on those drivers.
