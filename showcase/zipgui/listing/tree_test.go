package listing

import (
	"reflect"
	"testing"

	"github.com/unxed/goWidgets/showcase/zipgui/engine"
)

func TestTreeNestedAndImpliedDirectories(t *testing.T) {
	tree := NewTree([]engine.Entry{
		{Name: "root.txt"},
		{Name: "sub/deep/file.txt", Size: 12},
		{Name: "sub/other.txt"},
	})

	root := tree.Children("/")
	if got, want := names(root), []string{"sub", "root.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	if !root[0].IsDir {
		t.Fatalf("sub entry is not a directory: %#v", root[0])
	}
	if got, want := names(tree.Children("/sub")), []string{"deep", "other.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sub children = %v, want %v", got, want)
	}
	deep := tree.Children("sub/deep")
	if len(deep) != 1 || deep[0].Name != "file.txt" || deep[0].Size != 12 {
		t.Fatalf("deep children = %#v, want file.txt with size 12", deep)
	}
	if got := tree.Parent("/sub/deep"); got != "/sub" {
		t.Fatalf("Parent(/sub/deep) = %q, want /sub", got)
	}
}

func TestTreeExplicitDirectoriesAreNotDuplicated(t *testing.T) {
	tree := NewTree([]engine.Entry{
		{Name: "sub/", IsDir: true},
		{Name: "sub/file.txt"},
	})
	children := tree.Children("/")
	if len(children) != 1 || children[0].Name != "sub" || !children[0].IsDir {
		t.Fatalf("root children = %#v, want one sub directory", children)
	}
}

func TestNavEnterAndUp(t *testing.T) {
	var nav Nav
	if got := nav.Path(); got != "/" {
		t.Fatalf("initial Path() = %q, want /", got)
	}
	nav.Up()
	if got := nav.Path(); got != "/" {
		t.Fatalf("Path() after Up at root = %q, want /", got)
	}
	nav.Enter("sub")
	nav.Enter("deep")
	if got := nav.Path(); got != "/sub/deep" {
		t.Fatalf("Path() = %q, want /sub/deep", got)
	}
	nav.Up()
	if got := nav.Path(); got != "/sub" {
		t.Fatalf("Path() after Up = %q, want /sub", got)
	}
	nav.Up()
	nav.Up()
	if got := nav.Path(); got != "/" {
		t.Fatalf("Path() after returning to root = %q, want /", got)
	}
}

func names(entries []engine.Entry) []string {
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Name
	}
	return got
}
