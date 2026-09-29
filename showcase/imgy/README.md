# imgy

`imgy` is the goWidgets showcase for a FastStone Image Viewer rewrite. It is
developed as a RUP-style sequence of small, visible increments.

## Stage 1 — browser walking skeleton

The first increment implements the shortest useful loop described by the
FastStone help (`FSViewerHelp.chm`, topics **Overview**, **Features**, and
**Mouse & Keyboard Use**):

1. start in a folder;
2. show the supported image files in an Explorer-like list;
3. select an image;
4. keep a large preview area and file metadata visible;
5. expose the future full-screen entry point.

Run it with:

```text
go run ./showcase/imgy [folder]
```

All geometry is declared through `Window.Constrain`, `Row`, or intrinsic-size
preferences. There are no pixel-position layout calls in the showcase.

The preview is intentionally a backend-neutral placeholder in stage 1. The
next increment adds the `ImageView` widget and image decoding, then zoom/pan
and the top/bottom/left/right fly-out panels described in the help.

## Acceptance gate

The stage is complete when a user can open a folder, select an image, and see
the selected filename/size change without restarting the app. The headless
backend can exercise the same layout and selection pipeline in CI; Win32 and
GTK provide the native visual shell.
