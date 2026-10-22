// Package listing contains archive-listing and navigation models independent
// of the UI.
package listing

import (
	"path"
	"sort"
	"strings"

	"github.com/unxed/goWidgets/showcase/zipgui/engine"
)

// Tree is a directory tree built from an archive listing. Directories that
// have no explicit entry are inferred from paths of their descendants.
type Tree struct {
	children map[string][]engine.Entry
}

// NewTree builds a tree from archive entries. Children expose each entry's
// basename in Name, while all other entry metadata is preserved.
func NewTree(entries []engine.Entry) *Tree {
	t := &Tree{children: map[string][]engine.Entry{"": nil}}
	seen := make(map[string]bool)
	addDir := func(dir string, entry engine.Entry) {
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		parent := path.Dir(dir)
		if parent == "." {
			parent = ""
		}
		t.children[parent] = append(t.children[parent], entry)
		t.children[dir] = nil
	}

	for _, entry := range entries {
		name := strings.Trim(entry.Name, "/")
		if name == "" || path.IsAbs(entry.Name) || name == ".." || strings.HasPrefix(name, "../") {
			continue
		}
		clean := path.Clean(name)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			continue
		}
		parts := strings.Split(clean, "/")
		for i := 1; i < len(parts); i++ {
			dir := strings.Join(parts[:i], "/")
			addDir(dir, engine.Entry{Name: parts[i-1], IsDir: true})
		}
		if entry.IsDir || strings.HasSuffix(entry.Name, "/") {
			addDir(clean, engine.Entry{Name: path.Base(clean), IsDir: true})
			continue
		}
		parent := path.Dir(clean)
		if parent == "." {
			parent = ""
		}
		entry.Name = path.Base(clean)
		t.children[parent] = append(t.children[parent], entry)
	}
	for dir := range t.children {
		sort.SliceStable(t.children[dir], func(i, j int) bool {
			a, b := t.children[dir][i], t.children[dir][j]
			if a.IsDir != b.IsDir {
				return a.IsDir
			}
			return a.Name < b.Name
		})
	}
	return t
}

// Children returns the immediate entries of dir. The root is "/"; nested
// paths may be supplied with or without a leading slash.
func (t *Tree) Children(dir string) []engine.Entry {
	if t == nil {
		return nil
	}
	return append([]engine.Entry(nil), t.children[cleanDir(dir)]...)
}

// Parent returns the parent directory, keeping the root as "/".
func (t *Tree) Parent(dir string) string {
	key := cleanDir(dir)
	if key == "" {
		return "/"
	}
	parent := path.Dir(key)
	if parent == "." {
		return "/"
	}
	return "/" + parent
}

func cleanDir(dir string) string {
	clean := path.Clean(strings.Trim(dir, "/"))
	if clean == "." || clean == "/" {
		return ""
	}
	return clean
}

// Nav tracks the current directory within a tree. Its path is always rooted
// at "/"; entering a child accepts only one directory-name component.
type Nav struct {
	stack []string
}

// Enter moves into a named child directory. Invalid or multi-component names
// are ignored so archive paths cannot escape the current directory.
func (n *Nav) Enter(name string) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return
	}
	n.stack = append(n.stack, name)
}

// Up moves to the parent directory; at the root it does nothing.
func (n *Nav) Up() {
	if len(n.stack) > 0 {
		n.stack = n.stack[:len(n.stack)-1]
	}
}

// Path returns the current directory in slash-prefixed archive notation.
func (n *Nav) Path() string {
	if len(n.stack) == 0 {
		return "/"
	}
	return "/" + strings.Join(n.stack, "/")
}
