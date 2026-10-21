package engine

import (
	"context"
	"hash/crc32"
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
	// MapFS implies sub and sub/deep from the file names; an fs.FS tells no
	// packed sizes.
	want := Summary{Files: 3, Dirs: 2, Size: 11, Packed: -1}
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
// in this process, for the formats zipgui opens first. A zip tells the
// packed size of every file, a tar stream does not.
func TestOpenReadsArchivesWrittenByZipper(t *testing.T) {
	// OpenFS keeps a tar index under the user cache; keep it in the test.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	for _, name := range []string{"sample.zip", "sample.tar.zst"} {
		t.Run(name, func(t *testing.T) {
			out := packSample(t, name)
			got, err := Open(out)
			if err != nil {
				t.Fatal(err)
			}
			if got.Files != 2 || got.Dirs != 1 || got.Size != 11 {
				t.Fatalf("Open(%s) = %+v, want 2 files, 1 directory, 11 bytes", name, got)
			}
			if zip := name == "sample.zip"; zip && got.Packed <= 0 || !zip && got.Packed != -1 {
				t.Fatalf("Open(%s).Packed = %d, want > 0 for zip and -1 for tar", name, got.Packed)
			}
		})
	}
}

// TestListReportsZipperMetadata checks the columns of the file list on what
// zipper writes: a zip names its method and keeps sizes and CRC-32 of each
// file; a tar.zst only knows the method of the whole stream.
func TestListReportsZipperMetadata(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	for _, tc := range []struct {
		name     string
		packed   bool // the format tells packed sizes and CRC-32
		method   string
		dirCount int
	}{
		{"sample.zip", true, "", 1},
		{"sample.tar.zst", false, "zstd", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := List(context.Background(), packSample(t, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]Entry{}
			dirs := 0
			for _, e := range entries {
				byName[e.Name] = e
				if e.IsDir {
					dirs++
				}
			}
			if dirs != tc.dirCount || !byName["sub"].IsDir {
				t.Fatalf("directories = %d (sub: %+v), want %d with sub", dirs, byName["sub"], tc.dirCount)
			}
			for file, data := range map[string]string{"a.txt": "hello", "sub/b.txt": "world!"} {
				e, ok := byName[file]
				if !ok {
					t.Fatalf("no entry %q in %+v", file, entries)
				}
				if e.IsDir || e.Size != int64(len(data)) || e.Modified.IsZero() || e.Encrypted {
					t.Fatalf("%s: %+v, want a plain file of %d bytes with a time", file, e, len(data))
				}
				if tc.packed {
					if e.Packed <= 0 || e.CRC32 != crc32.ChecksumIEEE([]byte(data)) || e.Method == "" {
						t.Fatalf("%s: %+v, want packed size, CRC-32 %08x and a method", file, e,
							crc32.ChecksumIEEE([]byte(data)))
					}
				} else if e.Packed != -1 || e.CRC32 != 0 || e.Method != tc.method {
					t.Fatalf("%s: %+v, want Packed -1, no CRC-32, method %q", file, e, tc.method)
				}
			}
		})
	}
}

func TestListMissingFile(t *testing.T) {
	if _, err := List(context.Background(), filepath.Join(t.TempDir(), "absent.zip")); err == nil {
		t.Fatal("List of a missing archive returned no error")
	}
}

func TestSummaryOf(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []Entry
		want    Summary
	}{
		{"empty", nil, Summary{}},
		{"zip", []Entry{
			{Name: "d", IsDir: true},
			{Name: "d/a", Size: 100, Packed: 40},
			{Name: "b", Size: 10, Packed: 10},
		}, Summary{Files: 2, Dirs: 1, Size: 110, Packed: 50}},
		{"one unknown makes the total unknown", []Entry{
			{Name: "a", Size: 100, Packed: 40},
			{Name: "b", Size: 10, Packed: -1},
		}, Summary{Files: 2, Size: 110, Packed: -1}},
		{"directories only", []Entry{{Name: "d", IsDir: true, Packed: -1}}, Summary{Dirs: 1}},
	} {
		if got := SummaryOf(tc.entries); got != tc.want {
			t.Errorf("%s: SummaryOf = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// packSample writes a.txt ("hello") and sub/b.txt ("world!") with zipper into
// a new archive named name and returns its path.
func packSample(t *testing.T, name string) string {
	t.Helper()
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
	return out
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
