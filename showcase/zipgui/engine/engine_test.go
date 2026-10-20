package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/unxed/zipper/archive"
)

func TestSummarizeCountsFilesDirsAndSize(t *testing.T) {
	fsys := fstest.MapFS{
		"a.txt":         {Data: []byte("hello")},
		"sub/b.txt":     {Data: []byte("world!")},
		"sub/deep/c.md": {Data: []byte("")},
	}
	got, err := Summarize(fsys)
	if err != nil {
		t.Fatal(err)
	}
	// MapFS implies sub and sub/deep from the file names.
	want := Summary{Files: 3, Dirs: 2, Size: 11}
	if got != want {
		t.Fatalf("Summarize = %+v, want %+v", got, want)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	got, err := Summarize(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if got != (Summary{}) {
		t.Fatalf("Summarize(empty) = %+v, want zero", got)
	}
}

// TestOpenReadsArchivesWrittenByZipper packs a small tree with zipper itself
// and reads it back through Open: the engine and zipper agree end to end,
// in this process, for the formats zipgui opens first.
func TestOpenReadsArchivesWrittenByZipper(t *testing.T) {
	// OpenFS keeps a tar index under the user cache; keep it in the test.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	for _, name := range []string{"sample.zip", "sample.tar.zst"} {
		t.Run(name, func(t *testing.T) {
			src := t.TempDir()
			write(t, filepath.Join(src, "a.txt"), "hello")
			write(t, filepath.Join(src, "sub", "b.txt"), "world!")

			files := map[string]os.FileInfo{}
			for _, rel := range []string{"a.txt", "sub", filepath.Join("sub", "b.txt")} {
				p := filepath.Join(src, rel)
				info, err := os.Lstat(p)
				if err != nil {
					t.Fatal(err)
				}
				files[p] = info
			}

			out := filepath.Join(t.TempDir(), name)
			a, err := archive.NewArchiver(out, src, archive.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Archive(context.Background(), files); err != nil {
				_ = a.Close()
				t.Fatal(err)
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}

			got, err := Open(out)
			if err != nil {
				t.Fatal(err)
			}
			want := Summary{Files: 2, Dirs: 1, Size: 11}
			if got != want {
				t.Fatalf("Open(%s) = %+v, want %+v", name, got, want)
			}
		})
	}
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "absent.zip")); err == nil {
		t.Fatal("Open of a missing archive returned no error")
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
