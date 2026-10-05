package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyOwnsOnlyWhatItInstalled(t *testing.T) {
	staging, target := t.TempDir(), t.TempDir()
	manifest := filepath.Join(t.TempDir(), "claude.json")
	fixture(t, filepath.Join(staging, "skills", "a", "SKILL.md"), "a\n")
	fixture(t, filepath.Join(staging, "skills", "b", "SKILL.md"), "b\n")
	fixture(t, filepath.Join(target, "skills", "mine", "SKILL.md"), "mine\n")
	files := map[string]string{
		filepath.Join("skills", "a", "SKILL.md"): filepath.Join(staging, "skills", "a", "SKILL.md"),
		filepath.Join("skills", "b", "SKILL.md"): filepath.Join(staging, "skills", "b", "SKILL.md"),
	}
	if written, removed, err := apply(target, files, manifest, false); err != nil || written != 2 || removed != 0 {
		t.Fatalf("first install: %d %d %v", written, removed, err)
	}
	if _, _, err := apply(target, files, manifest, false); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	fixture(t, filepath.Join(target, "skills", "a", "SKILL.md"), "edited\n")
	if _, _, err := apply(target, files, manifest, false); err == nil || !strings.Contains(err.Error(), "skills/a/SKILL.md") {
		t.Fatalf("edited file: %v", err)
	}
	if _, _, err := apply(target, files, manifest, true); err != nil {
		t.Fatalf("force: %v", err)
	}
	link := filepath.Join(target, "skills", "b", "SKILL.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "skills", "mine", "SKILL.md"), link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := apply(target, files, manifest, false); err == nil {
		t.Fatal("wrote through a symlink it does not own")
	}
	if _, _, err := apply(target, files, manifest, true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target, "skills", "mine", "SKILL.md")); string(data) != "mine\n" {
		t.Fatalf("symlink target overwritten: %q", data)
	}
	delete(files, filepath.Join("skills", "b", "SKILL.md"))
	if _, removed, err := apply(target, files, manifest, false); err != nil || removed != 1 {
		t.Fatalf("prune: %d %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "b")); !os.IsNotExist(err) {
		t.Fatalf("pruned skill directory remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "mine", "SKILL.md")); err != nil {
		t.Fatalf("foreign skill removed: %v", err)
	}
}
