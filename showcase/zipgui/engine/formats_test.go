package engine

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/zipper/archive"
)

func TestFormatsTableIsConsistent(t *testing.T) {
	list := Formats()
	if len(list) == 0 {
		t.Fatal("Formats() is empty")
	}
	seen := map[string]bool{}
	for _, f := range list {
		if seen[f.Ext] {
			t.Errorf("%s is listed twice", f.Ext)
		}
		seen[f.Ext] = true
		kind := archive.DetectFormat("x" + f.Ext)
		if kind == "" {
			t.Errorf("archive.DetectFormat(%q) is empty", "x"+f.Ext)
		}
		if f.Update && kind != "zip" && kind != "tar" {
			t.Errorf("%s: Update, but zipper detects %q (its updater writes only zip and tar)", f.Ext, kind)
		}
		if !f.Read {
			t.Errorf("%s: not readable", f.Ext)
		}
		if f.Update && !f.Write {
			t.Errorf("%s: Update without Write", f.Ext)
		}
	}
	list[0].Ext = "changed"
	if Formats()[0].Ext == "changed" {
		t.Error("Formats() returns the table itself, not a copy")
	}
}

func TestWritableFormatUsesLongestExtension(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
		ok   bool
	}{
		{"backup.tar.gz", ".tar.gz", true},
		{"backup.TAR.XZ", ".tar.xz", true},
		{"backup.tar.zst", ".tar.zst", true},
		{"backup.zip", ".zip", true},
		{"backup.7z", ".7z", true},
		{"backup.tar.bz2", "", false},
		{"backup.rar", "", false},
		{"backup.txt", "", false},
	} {
		got, ok := WritableFormat(tc.name)
		if ok != tc.ok || got.Ext != tc.want {
			t.Errorf("WritableFormat(%q) = (%+v, %t), want extension %q, ok %t", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestFormatsMatchReference keeps the table of docs/zipgui/REFERENCE.md and
// Formats() the same.
func TestFormatsMatchReference(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "..", "docs", "zipgui", "REFERENCE.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var doc []Format
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "| `.") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 6 {
			t.Fatalf("REFERENCE.md: a format row needs 6 cells: %q", line)
		}
		yes := func(i int) bool {
			switch v := strings.TrimSpace(cells[i]); v {
			case "да":
				return true
			case "нет":
				return false
			default:
				t.Fatalf("REFERENCE.md: %q is neither да nor нет in %q", v, line)
				return false
			}
		}
		doc = append(doc, Format{
			Ext:      strings.Trim(strings.TrimSpace(cells[0]), "`"),
			Read:     yes(1),
			Write:    yes(2),
			Update:   yes(3),
			Password: yes(4),
			Level:    yes(5),
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	code := Formats()
	if len(doc) != len(code) {
		t.Fatalf("REFERENCE.md lists %d formats, Formats() %d", len(doc), len(code))
	}
	for i := range code {
		if doc[i] != code[i] {
			t.Errorf("row %d: REFERENCE.md %+v, Formats() %+v", i+1, doc[i], code[i])
		}
	}
}

// TestFormatsMatchZipper does with zipper what the table promises: a format
// with Write is created and read back, one with Update gets a file appended,
// and zipper refuses the ones without. Read-only formats need an archive
// zipper cannot make, so only their refusal to be written is checked here.
func TestFormatsMatchZipper(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	for _, f := range Formats() {
		t.Run(strings.TrimPrefix(f.Ext, "."), func(t *testing.T) {
			src := t.TempDir()
			write(t, filepath.Join(src, "a.txt"), "hello")
			write(t, filepath.Join(src, "b.txt"), "world!")
			out := filepath.Join(t.TempDir(), "sample"+f.Ext)

			err := pack(out, src, "a.txt", "b.txt")
			if !f.Write {
				if err == nil {
					t.Fatalf("zipper wrote %s, but the table says it cannot", f.Ext)
				}
				return
			}
			if err != nil {
				t.Fatalf("writing %s: %v", f.Ext, err)
			}
			if got, err := Open(out); err != nil || got.Files != 2 {
				t.Fatalf("Open(new %s) = %+v, %v; want 2 files", f.Ext, got, err)
			}

			u, err := archive.NewUpdater(out, archive.Options{})
			if err != nil {
				if f.Update {
					t.Fatalf("NewUpdater(%s): %v", f.Ext, err)
				}
				return
			}
			err = u.Append("c.txt", 3, strings.NewReader("abc"))
			if cerr := u.Close(); err == nil {
				err = cerr
			}
			if !f.Update {
				if err == nil {
					t.Fatalf("zipper appended to %s, but the table says it cannot", f.Ext)
				}
				return
			}
			if err != nil {
				t.Fatalf("appending to %s: %v", f.Ext, err)
			}
			// Read back through the default tar index in the cache, which the
			// Open above built for the old contents: since zipper v0.1.179 its
			// name carries a fingerprint of the archive, so the changed
			// archive gets a fresh index (unxed/zipper#35, Z7).
			got, err := Open(out)
			if err != nil || got.Files != 3 {
				t.Fatalf("updated %s holds %+v, %v; want 3 files", f.Ext, got, err)
			}
		})
	}
}

// pack writes the named files of dir into a new archive at out.
func pack(out, dir string, names ...string) error {
	files := map[string]os.FileInfo{}
	for _, name := range names {
		p := filepath.Join(dir, name)
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		files[p] = info
	}
	a, err := archive.NewArchiver(out, dir, archive.Options{})
	if err != nil {
		return err
	}
	err = a.Archive(context.Background(), files)
	if cerr := a.Close(); err == nil {
		err = cerr
	}
	return err
}
