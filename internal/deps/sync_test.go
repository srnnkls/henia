package deps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// realPhora returns the phora binary HENIA_PHORA names, or skips the test.
func realPhora(t *testing.T) {
	t.Helper()
	if os.Getenv("HENIA_PHORA") == "" {
		t.Skip("needs a real phora in HENIA_PHORA")
	}
	for _, env := range []string{"XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(env, t.TempDir())
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commitPackage writes files into the repository at dir and commits them,
// creating the repository on first use; it returns the new commit.
func commitPackage(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "init", "-q", "-b", "main")
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "update")
	return git(t, dir, "rev-parse", "HEAD")
}

func lockedCommits(t *testing.T, lock string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	var file lockFile
	if err := toml.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	commits := map[string]string{}
	for _, source := range file.Sources {
		name, _ := source["name"].(string)
		commit, _ := source["commit"].(string)
		commits[name] = commit
	}
	return commits
}

func TestSyncKeepsTransitivePinsAtTheirLockedCommit(t *testing.T) {
	realPhora(t)
	root := t.TempDir()
	leaf := filepath.Join(root, "leaf")
	locked := commitPackage(t, leaf, map[string]string{"skills/leaf/SKILL.md": "---\nname: leaf\n---\n"})
	middle := filepath.Join(root, "middle")
	commitPackage(t, middle, map[string]string{
		"henia.toml":             "[dependencies.leaf]\ngit = \"file://" + leaf + "\"\nbranch = \"main\"\n",
		"skills/middle/SKILL.md": "---\nname: middle\n---\n",
	})
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project, "henia.toml")
	if err := os.WriteFile(manifest, []byte("[dependencies.middle]\ngit = \"file://"+middle+"\"\nbranch = \"main\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scope := Scope{
		Manifest: manifest,
		Lock:     filepath.Join(project, "henia.lock"),
		State:    filepath.Join(root, "state"),
		Store:    filepath.Join(root, "store"),
		Tools:    filepath.Join(root, "tools"),
	}
	if _, err := Sync(context.Background(), scope, false); err != nil {
		t.Fatal(err)
	}
	if got := lockedCommits(t, scope.Lock)["leaf"]; got != locked {
		t.Fatalf("first sync locks leaf at %q, want %q", got, locked)
	}

	commitPackage(t, leaf, map[string]string{"skills/leaf/SKILL.md": "---\nname: leaf\n---\nnewer\n"})
	if _, err := Sync(context.Background(), scope, false); err != nil {
		t.Fatal(err)
	}

	if got := lockedCommits(t, scope.Lock)["leaf"]; got != locked {
		t.Fatalf("a sync moved the transitive leaf from its locked %q to %q", locked, got)
	}
}

func TestUpdatePastAStaleStoreThatDeclaresADependencyDifferently(t *testing.T) {
	realPhora(t)
	root := t.TempDir()
	leaf := filepath.Join(root, "leaf")
	commitPackage(t, leaf, map[string]string{"skills/leaf/SKILL.md": "---\nname: leaf\n---\n"})
	git(t, leaf, "branch", "dev")
	middle := filepath.Join(root, "middle")
	declares := func(branch string) map[string]string {
		return map[string]string{
			"henia.toml":             "[dependencies.leaf]\ngit = \"file://" + leaf + "\"\nbranch = \"" + branch + "\"\n",
			"skills/middle/SKILL.md": "---\nname: middle\n---\n",
		}
	}
	commitPackage(t, middle, declares("main"))
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project, "henia.toml")
	write := func(content string) {
		if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("[dependencies.middle]\ngit = \"file://" + middle + "\"\nbranch = \"main\"\n")
	scope := Scope{
		Manifest: manifest,
		Lock:     filepath.Join(project, "henia.lock"),
		State:    filepath.Join(root, "state"),
		Store:    filepath.Join(root, "store"),
		Tools:    filepath.Join(root, "tools"),
	}
	if _, err := Sync(context.Background(), scope, false); err != nil {
		t.Fatal(err)
	}

	commitPackage(t, middle, declares("dev"))
	write("[dependencies.middle]\ngit = \"file://" + middle + "\"\nbranch = \"main\"\n\n[dependencies.leaf]\ngit = \"file://" + leaf + "\"\nbranch = \"dev\"\n")

	if _, err := Sync(context.Background(), scope, true); err != nil {
		t.Fatalf("update must get past the stale store's declaration of leaf: %v", err)
	}
}
