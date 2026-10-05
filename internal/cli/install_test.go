package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
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
	if written, removed, err := apply(target, files, manifest, false, nil); err != nil || written != 2 || removed != 0 {
		t.Fatalf("first install: %d %d %v", written, removed, err)
	}
	if _, _, err := apply(target, files, manifest, false, nil); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	fixture(t, filepath.Join(target, "skills", "a", "SKILL.md"), "edited\n")
	if _, _, err := apply(target, files, manifest, false, nil); err == nil || !strings.Contains(err.Error(), "skills/a/SKILL.md") {
		t.Fatalf("edited file: %v", err)
	}
	if _, _, err := apply(target, files, manifest, true, nil); err != nil {
		t.Fatalf("force: %v", err)
	}
	link := filepath.Join(target, "skills", "b", "SKILL.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "skills", "mine", "SKILL.md"), link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := apply(target, files, manifest, false, nil); err == nil {
		t.Fatal("wrote through a symlink it does not own")
	}
	if _, _, err := apply(target, files, manifest, true, nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target, "skills", "mine", "SKILL.md")); string(data) != "mine\n" {
		t.Fatalf("symlink target overwritten: %q", data)
	}
	delete(files, filepath.Join("skills", "b", "SKILL.md"))
	if _, removed, err := apply(target, files, manifest, false, nil); err != nil || removed != 1 {
		t.Fatalf("prune: %d %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "b")); !os.IsNotExist(err) {
		t.Fatalf("pruned skill directory remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "mine", "SKILL.md")); err != nil {
		t.Fatalf("foreign skill removed: %v", err)
	}
}

func TestInstallTargets(t *testing.T) {
	home, _ := os.UserHomeDir()
	off := false
	env := map[string]string{"CODEX_HOME": "/srv/codex"}
	present := map[string]bool{filepath.Join(home, ".claude"): true, "/srv/codex": true}
	targets, err := installTargets(
		map[string]config.InstallTarget{"omp": {Enabled: &off}, "custom": {Path: "~/agents/custom"}},
		map[string]bool{"claude": true, "codex": true, "pi": true, "omp": true, "gemini": true},
		func(key string) string { return env[key] },
		func(path string) bool { return present[path] },
	)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, target := range targets {
		got = append(got, fmt.Sprintf("%s=%s %v", target.harness, target.path, target.explicit))
	}
	want := []string{"claude=" + filepath.Join(home, ".claude") + " false", "codex=/srv/codex false", "custom=" + filepath.Join(home, "agents/custom") + " true"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("targets:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, err := installTargets(map[string]config.InstallTarget{"gemini": {}}, nil, func(string) string { return "" }, func(string) bool { return true }); err == nil {
		t.Fatal("a listed harness without a known home needs a path")
	}
}

func TestStaleHead(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	skill := filepath.Join(t.TempDir(), "SKILL.md")
	fixture(t, skill, "# Route\n")
	entry := library.Entry{Name: "route", Path: skill}
	if stale("claude", entry) {
		t.Fatal("stale without an install record")
	}
	manifest := library.InstallManifest("claude")
	data, _ := json.Marshal(installRecord{Revisions: map[string]string{"route": build.Digest([]byte("# Route\n"))}})
	fixture(t, manifest, string(data))
	if stale("claude", entry) || stale("", entry) {
		t.Fatal("fresh install reported stale")
	}
	fixture(t, skill, "# Route\n\nEdited.\n")
	if !stale("claude", entry) {
		t.Fatal("edited library skill not reported stale")
	}
}
