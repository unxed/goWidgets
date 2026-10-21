package engine

// Format is what zipgui can do with one kind of archive, told by the end of
// its file name. The table follows zipper/archive: NewArchiver for Write,
// NewUpdater for Update; docs/zipgui/REFERENCE.md shows the same table.
type Format struct {
	Ext      string // ".zip", ".tar.gz", ...
	Read     bool   // open, list and extract
	Write    bool   // create a new archive
	Update   bool   // add to and remove from an existing archive in place
	Password bool   // a password protects what is written, or opens what is read
}

var formats = []Format{
	{Ext: ".zip", Read: true, Write: true, Update: true, Password: true},
	{Ext: ".tar", Read: true, Write: true, Update: true, Password: true},
	{Ext: ".tar.gz", Read: true, Write: true, Update: true, Password: true},
	{Ext: ".tar.xz", Read: true, Write: true, Update: true, Password: true},
	{Ext: ".tar.zst", Read: true, Write: true, Update: true, Password: true},
	// The tar package reads bzip2 but has no bzip2 compressor.
	{Ext: ".tar.bz2", Read: true, Password: true},
	// zipper's updater refuses the formats it hands to the archives package.
	{Ext: ".7z", Read: true, Write: true, Password: true},
	// zipper does not write RAR.
	{Ext: ".rar", Read: true, Password: true},
}

// Formats returns the archive formats zipgui knows, in the order a format
// list shows them. The slice is a copy.
func Formats() []Format {
	return append([]Format(nil), formats...)
}
