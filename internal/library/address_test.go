package library

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAddresses(t *testing.T) {
	for target, want := range map[string][]any{
		"git":                       {"git", []string(nil), "", false},
		"git.reference.worktree":    {"git", []string{"reference", "worktree"}, "", false},
		"tropos:git.reference":      {"tropos:git", []string{"reference"}, "", false},
		"pr/scripts/pr-context":     {"pr", []string(nil), "scripts/pr-context", true},
		"tropos.io:git.reference.x": {"tropos.io:git", []string{"reference", "x"}, "", false},
	} {
		skill, modules, resource, isResource := SplitAddress(target)
		if got := []any{skill, modules, resource, isResource}; !reflect.DeepEqual(got, want) {
			t.Errorf("SplitAddress(%q) = %v, want %v", target, got, want)
		}
	}
	dir := t.TempDir()
	for _, f := range []string{"reference/worktree.md", "reference/notes/a.md", "scripts/run.sh", "v1.2.md"} {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for modules, want := range map[string]string{"reference.worktree": "reference/worktree.md", "reference": "reference/", "reference.notes": "reference/notes/"} {
		if got, err := ModulePath(dir, splitDots(modules)); err != nil || got != want {
			t.Errorf("ModulePath(%s) = %q, %v; want %q", modules, got, err, want)
		}
	}
	if _, err := ModulePath(dir, []string{"nope"}); err == nil {
		t.Error("missing module resolved")
	}
	for path, want := range map[string]string{"reference/worktree.md": "git.reference.worktree", "reference/": "git.reference", "scripts/run.sh": "git/scripts/run.sh", "v1.2.md": "git/v1.2.md", "": "git"} {
		if got := Address("git", path); got != want {
			t.Errorf("Address(%q) = %q, want %q", path, got, want)
		}
	}
}

func splitDots(s string) []string { return strings.Split(s, ".") }
