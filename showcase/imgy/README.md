# imgy

`imgy` is an image-viewer showcase for goWidgets. It provides a constraint-laid-
out window, a folder listing, file/folder selection, and an image preview with
fit, actual-size, fill, cursor-centered zoom, and drag-to-pan. It decodes GIF,
JPEG, and PNG using the Go standard library, plus BMP, TIFF, and WebP with
`golang.org/x/image` v0.46.0. WebP support is decode-only for now.

All interface placement uses goWidgets Auto Layout. Canvas is used only for
image pixels; on backends that do not yet support Canvas, the preview falls
back to the native ImageView widget.

Run it from this module with:

```sh
go run ./showcase/imgy [image-file-or-folder]
```

Use **Open folder** to browse a directory, select an image in the list, or use
**Open image…** / **Ctrl+O** to choose a file. The mouse wheel goes to the
previous or next naturally sorted image; **Ctrl+wheel** zooms around the
pointer. Drag the image to pan. **F** fits the whole image, **1** shows actual
size, and **2** fills the preview area. **← / Backspace / Page Up** go to the
previous image; **→ / Space / Page Down** go to the next. **Home / End** go to
the first or last image. **Esc** exits.

We like FastStone and use it as an example of a good image viewer.
