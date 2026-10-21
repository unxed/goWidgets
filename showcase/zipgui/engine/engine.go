// Package engine is zipgui's only door to github.com/unxed/zipper/archive.
//
// The window code never imports zipper itself (ADR-0019): whatever the
// window needs from an archive is asked for here, so a missing piece of the
// zipper API shows up in this package and gets fixed in zipper, not worked
// around in the UI.
package engine

import (
	"io/fs"

	"github.com/unxed/zipper/archive"
)

// Summary is what the archive holds, counted over every entry.
type Summary struct {
	Files int   // regular files and other non-directory entries
	Dirs  int   // directories, including ones the format only implies
	Size  int64 // unpacked size of all files, bytes
}

// Summarize walks fsys from its root and counts what it finds.
func Summarize(fsys fs.FS) (Summary, error) {
	var s Summary
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if d.IsDir() {
			s.Dirs++
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		s.Files++
		s.Size += info.Size()
		return nil
	})
	return s, err
}

// Open reads the archive at path with zipper and summarizes it.
func Open(path string) (Summary, error) {
	fsys, err := archive.OpenFS(path, archive.Options{})
	if err != nil {
		return Summary{}, err
	}
	s, err := Summarize(fsys)
	if cerr := fsys.Close(); err == nil {
		err = cerr
	}
	return s, err
}
