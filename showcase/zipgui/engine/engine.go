// Package engine is zipgui's only door to github.com/unxed/zipper/archive.
//
// The window code never imports zipper itself (ADR-0019): whatever the
// window needs from an archive is asked for here, so a missing piece of the
// zipper API shows up in this package and gets fixed in zipper, not worked
// around in the UI.
package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// TestError identifies an archive member that failed integrity testing.
type TestError struct {
	Name string
	Err  error
}

// Test reads every file in an archive without extracting it. Progress reports
// cumulative uncompressed bytes and files tested.
func Test(ctx context.Context, filename string, progress func(bytes, entries int64)) ([]TestError, error) {
	failures, err := archive.Test(ctx, filename, archive.Options{}, progress)
	if err != nil {
		return nil, err
	}
	result := make([]TestError, len(failures))
	for i, failure := range failures {
		result[i] = TestError{Name: failure.Name, Err: failure.Err}
	}
	return result, nil
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

// Create writes the selected source files to a new archive. Files are stored
// by their base names; duplicate names are rejected instead of overwriting an
// entry silently. A zero level leaves zipper's format default in effect.
func Create(ctx context.Context, destination string, sources []string, level int) error {
	format, ok := WritableFormat(destination)
	if !ok {
		return fmt.Errorf("unsupported archive extension: %s", filepath.Ext(destination))
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if level != 0 && (!format.Level || level < 1 || level > 9) {
		return fmt.Errorf("compression levels are not supported for %s", format.Ext)
	}
	if len(sources) == 0 {
		return fmt.Errorf("select at least one file to add")
	}

	files := make(map[string]os.FileInfo, len(sources))
	pathMapping := make(map[string]string, len(sources))
	names := make(map[string]bool, len(sources))
	for _, source := range sources {
		path, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		if path == destination {
			return fmt.Errorf("archive destination is also a selected source file: %s", source)
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", source)
		}
		name := filepath.Base(path)
		if names[name] {
			return fmt.Errorf("two selected files have the same name: %s", name)
		}
		names[name] = true
		files[path] = info
		pathMapping[path] = name
	}

	archiver, err := archive.NewArchiver(destination, filepath.Dir(destination), archive.Options{
		Level:       level,
		PathMapping: pathMapping,
	})
	if err != nil {
		return err
	}
	archiveErr := archiver.Archive(ctx, files)
	closeErr := archiver.Close()
	if archiveErr != nil {
		_ = os.Remove(destination)
		return archiveErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	return nil
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
