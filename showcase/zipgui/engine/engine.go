// Package engine is zipgui's only door to github.com/unxed/zipper/archive.
//
// The window code never imports zipper itself (ADR-0019): whatever the
// window needs from an archive is asked for here, so a missing piece of the
// zipper API shows up in this package and gets fixed in zipper, not worked
// around in the UI.
package engine

import (
	"context"
	"io/fs"
	"time"

	"github.com/unxed/zipper/archive"
)

// Summary is what the archive holds, counted over every entry.
type Summary struct {
	Files  int   // regular files and other non-directory entries
	Dirs   int   // directories, including ones the format only implies
	Size   int64 // unpacked size of all files, bytes
	Packed int64 // bytes the files take in the archive; -1 when the format does not tell it
}

// Entry is one row of the archive listing: the columns of WinRAR's file list
// (Name, Size, Packed, Modified, CRC32) plus what the window needs to sort and
// mark rows. It mirrors archive.Entry so that the window never imports zipper.
type Entry struct {
	Name      string    // slash-separated path inside the archive
	Size      int64     // unpacked size; 0 for a directory
	Packed    int64     // size inside the archive; -1 where the format does not tell it
	Modified  time.Time // modification time
	CRC32     uint32    // CRC-32 of the data; 0 where the format keeps none
	Method    string    // compression method ("deflate", "zstd", ...); "" where unknown
	Encrypted bool      // the data is encrypted
	IsDir     bool      // a directory, stored or implied
}

// Extractor exposes an in-progress extraction without leaking zipper into the
// window package.
type Extractor interface {
	Extract(context.Context) error
	Close() error
	Written() (bytes, entries int64)
	CurrentFile() string
}

type extractor struct {
	archive.Extractor
}

func (e extractor) CurrentFile() string {
	if current, ok := e.Extractor.(archive.CurrentFiler); ok {
		return current.CurrentFile()
	}
	return ""
}

// NewExtractor starts extracting archive paths into destination. An empty
// include list means all entries; keepOld skips existing destination files.
func NewExtractor(filename, destination string, include []string, keepOld bool) (Extractor, error) {
	inner, err := archive.NewExtractor(filename, destination, archive.Options{
		Include:      include,
		KeepOldFiles: keepOld,
	})
	if err != nil {
		return nil, err
	}
	return extractor{Extractor: inner}, nil
}

// List returns every entry of the archive at path, directories included, in
// lexical order within each directory (zipper archive.List, unxed/zipper#35 Z1).
func List(ctx context.Context, path string) ([]Entry, error) {
	got, err := archive.List(ctx, path, archive.Options{})
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(got))
	for i, e := range got {
		entries[i] = Entry(e)
	}
	return entries, nil
}

// SummaryOf counts the entries of a listing. Packed is the sum over the files
// only when every file tells its packed size, otherwise -1: a partial sum
// would show a compression ratio the archive does not have.
func SummaryOf(entries []Entry) Summary {
	s := Summary{}
	known := true
	for _, e := range entries {
		if e.IsDir {
			s.Dirs++
			continue
		}
		s.Files++
		s.Size += e.Size
		if e.Packed < 0 {
			known = false
		} else {
			s.Packed += e.Packed
		}
	}
	if !known {
		s.Packed = -1
	}
	return s
}

// Summarize walks fsys from its root and counts what it finds. An fs.FS does
// not tell packed sizes, so Packed is -1 when there is at least one file.
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
		s.Packed = -1
		return nil
	})
	return s, err
}

// Open reads the listing of the archive at path with zipper and summarizes it.
func Open(path string) (Summary, error) {
	entries, err := List(context.Background(), path)
	if err != nil {
		return Summary{}, err
	}
	return SummaryOf(entries), nil
}
