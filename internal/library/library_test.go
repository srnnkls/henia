package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skill(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, SkillsDir, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: "+filepath.Base(dir)+"\n---\n\n# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	project := filepath.Join(root, "repo")
	sources := filepath.Join(root, "data", "henia", "sources")
	skill(t, filepath.Join(project, ProjectDir), "shared")
	skill(t, filepath.Join(sources, "a"), "shared")
	skill(t, filepath.Join(sources, "a"), "twice")
	skill(t, filepath.Join(sources, "b"), "twice")
	skill(t, filepath.Join(sources, "b"), "only")
	lib := Open(project, nil)

	for ref, want := range map[string]string{
		"shared":   "project:shared",
		"a:shared": "a:shared",
		"only":     "b:only",
		"b:twice":  "b:twice",
	} {
		if got, err := lib.Resolve(ref); err != nil || got.ID != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", ref, got.ID, err, want)
		}
	}
	if _, err := lib.Resolve("twice"); err == nil || !strings.Contains(err.Error(), "a:twice, b:twice") {
		t.Errorf("Resolve(twice) error = %v; want an ambiguity listing a:twice, b:twice", err)
	}
	if _, err := lib.Resolve("c:only"); err == nil {
		t.Error("Resolve(c:only) resolved a skill from a source that does not exist")
	}
	for _, e := range lib.Entries {
		want := e.Name
		if e.Name == "twice" || e.ID == "a:shared" {
			want = e.ID
		}
		if got := lib.Reference(e); got != want {
			t.Errorf("Reference(%s) = %q; want %q", e.ID, got, want)
		}
	}
}

func TestEnsureGitignoreKeepsExisting(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(data) != gitignore {
		t.Fatalf(".gitignore = %q", data)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(data) != "custom\n" {
		t.Fatalf("EnsureGitignore overwrote an existing file: %q", data)
	}
}

func TestDependencySources(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	project := filepath.Join(root, "dotfiles")
	tropos := filepath.Join(project, ProjectDir, DependencyDir, "tropos")
	gestalt := filepath.Join(tropos, ProjectDir, DependencyDir, "gestalt")
	installed := filepath.Join(root, "data", "henia", "sources", "global")
	skill(t, tropos, "review")
	skill(t, tropos, "shared")
	skill(t, gestalt, "gestalt")
	skill(t, installed, "shared")
	if err := os.WriteFile(filepath.Join(tropos, ConfigFile), []byte("[harness.claude]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gestalt, ProjectDir, DependencyDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(tropos, filepath.Join(gestalt, ProjectDir, DependencyDir, "loop")); err != nil {
		t.Fatal(err)
	}
	lib := Open(project, nil)
	for ref, want := range map[string]string{"review": "tropos:review", "gestalt": "gestalt:gestalt", "shared": "tropos:shared", "global:shared": "global:shared"} {
		if got, err := lib.Resolve(ref); err != nil || got.ID != want || (want != "global:shared" && got.Tier != Dependency) {
			t.Errorf("Resolve(%q) = %q (%s), %v; want %q", ref, got.ID, got.Tier, err, want)
		}
	}
	if review, _ := lib.Resolve("review"); review.Origin.Config != filepath.Join(tropos, ConfigFile) {
		t.Errorf("tropos config = %q", review.Origin.Config)
	}
}
