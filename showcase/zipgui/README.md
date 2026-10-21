# zipgui

`zipgui` is the archiver showcase of goWidgets: a native-controls window over
the [zipper](https://github.com/unxed/zipper) engine (`zipper/archive`, in the
same process). Scope and plan: [ADR-0019](../../docs/adr/0019-showcase-zipgui.md),
[docs/zipgui/PLAN.md](../../docs/zipgui/PLAN.md).

It is a separate Go module so that zipper's dependencies stay out of the
library module. Build and run it from this directory:

```sh
go run . [archive]
```

Right now it opens an archive (zip, tar.*, 7z, rar) from the command line or
with **Открыть…** and shows how many files and folders it holds and their
unpacked size.
