package components

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadShadowsUserAndReservesBuiltinNames(t *testing.T) {
	project, user := t.TempDir(), t.TempDir()
	projectHead := filepath.Join(project, ".henia", "components", "head.md")
	include := filepath.Join(project, ".henia", "components", "include.md")
	write(t, projectHead, "---\nform: container\n---\nproject head\n")
	write(t, filepath.Join(user, "components", "head.md"), "---\nform: container\n---\nuser head\n")
	write(t, include, "---\nform: leaf\n---\nshadowed include\n")

	decls, problems := Load(project, user)

	if got := decls["head"].Path; got != projectHead {
		t.Errorf("head declaration path = %q, want project %q", got, projectHead)
	}
	var reserved bool
	for _, p := range problems {
		if p.ID == "invalid-component" && p.Path == include {
			reserved = true
		}
	}
	if !reserved {
		t.Errorf("problems = %+v, want invalid-component at %s", problems, include)
	}
}

func TestLoadSkipsEmptyRoots(t *testing.T) {
	cwd := t.TempDir()
	write(t, filepath.Join(cwd, "components", "stray.md"), "---\nform: leaf\n---\nstray\n")
	write(t, filepath.Join(cwd, ".henia", "components", "other.md"), "---\nform: leaf\n---\nother\n")
	t.Chdir(cwd)

	decls, problems := Load("", "")

	for _, name := range []string{"stray", "other"} {
		if _, ok := decls[name]; ok {
			t.Errorf("Load(\"\", \"\") loaded %s from the working directory", name)
		}
	}
	if len(problems) > 0 {
		t.Errorf("problems = %+v, want none", problems)
	}
}

func TestLoadReadsRootContainingGlobMeta(t *testing.T) {
	project := filepath.Join(t.TempDir(), "a[b")
	write(t, filepath.Join(project, ".henia", "components", "note.md"), "---\nform: leaf\n---\nnote\n")

	decls, problems := Load(project, "")

	if _, ok := decls["note"]; !ok {
		t.Errorf("Load() skipped note under %s; problems = %+v", project, problems)
	}
}
