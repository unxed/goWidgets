# imgy

`imgy` is an image-viewer showcase for goWidgets. The first increment provides a
constraint-laid-out window, a folder listing, file/folder selection, and a
centered “fit to window” preview. It currently decodes GIF, JPEG, and PNG using
the Go standard library. BMP support is deferred until an image-decoder
dependency is explicitly approved.

All interface placement uses goWidgets Auto Layout. Canvas is used only for
image pixels; on backends that do not yet support Canvas, the preview falls
back to the native ImageView widget.

Run it from this module with:

```sh
go run ./showcase/imgy [image-file-or-folder]
```

Use **Open folder** to browse a directory, select an image in the list, or use
**Open image…** / **Ctrl+O** to choose a file. **Esc** exits.

We like FastStone and use it as an example of a good image viewer.
