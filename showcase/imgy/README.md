# imgy

`imgy` is the goWidgets image viewer showcase, developed as a RUP-style
sequence of small, visible increments.

We like FastStone and use it as an example of a good image viewer.

## Stage 2 — image preview

The first increment implemented the shortest useful browse loop:

1. start in a folder;
2. show the supported image files in an Explorer-like list;
3. select an image;
4. show the selected image and its file metadata.

Run it with:

```text
go run ./showcase/imgy [folder]
```

All geometry is declared through `Window.Constrain`, `Row`, or intrinsic-size
preferences. There are no pixel-position layout calls in the showcase.

`ImageView` is implemented on Win32, GTK, Qt, Cocoa, and headless. It loads a
platform-supported image file, fits it to the constraint-allocated preview
area, and updates when selection changes. PNG, JPEG, and GIF dimensions are
shown alongside the file size.

## Acceptance gate

The next increment adds full-screen navigation, zoom, and panning. The headless
backend exercises the layout pipeline in CI; Windows, Linux, and macOS CI build
and exercise their native drivers.
