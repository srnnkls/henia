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

func TestSyncPlacesPackageResourcesInsideTheirSkill(t *testing.T) {
	realPhora(t)
	root := t.TempDir()
	pkg := filepath.Join(root, "loqui")
	commitPackage(t, pkg, map[string]string{
		"henia.toml":                 "",
		".henia/lint/rule.md":        "rule\n",
		"skills/loqui/SKILL.md":      "---\nname: loqui\nhenia:\n  resources:\n    package:\n      languages/: languages/\n      principles.md: shared/principles.md\n---\n",
		"skills/other/SKILL.md":      "---\nname: other\n---\n",
		"languages/python/README.md": "# Python\n",
		"shared/principles.md":       "# Principles\n",
		"unrelated/notes.md":         "notes\n",
	})
	for _, tc := range []struct {
		name     string
		declared string
		want     []string
		absent   []string
	}{
		{
			name:     "whole package",
			declared: "",
			want:     []string{"henia.toml", ".henia/lint/rule.md", "skills/other/SKILL.md", "unrelated/notes.md"},
			absent:   []string{"languages", "shared"},
		},
		{
			name:     "taken skills",
			declared: "skills = [\"loqui\"]\n",
			want:     []string{"henia.toml", ".henia/lint/rule.md"},
			absent:   []string{"languages", "shared", "skills/other", "unrelated"},
		},
		{
			name:     "linked",
			declared: "link = true\n",
			want:     []string{"skills/other/SKILL.md"},
			absent:   []string{"languages", "shared"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			manifest := filepath.Join(dir, "henia.toml")
			source := "git = \"file://" + pkg + "\"\nbranch = \"main\"\n"
			if strings.Contains(tc.declared, "link") {
				source = "path = \"" + pkg + "\"\n"
			}
			if err := os.WriteFile(manifest, []byte("[dependencies.loqui]\n"+source+tc.declared), 0o644); err != nil {
				t.Fatal(err)
			}
			scope := Scope{
				Manifest: manifest,
				Lock:     filepath.Join(dir, "henia.lock"),
				State:    filepath.Join(dir, "state"),
				Store:    filepath.Join(dir, "store"),
				Tools:    filepath.Join(dir, "tools"),
			}
			if _, err := Sync(context.Background(), scope, false); err != nil {
				t.Fatal(err)
			}
			deployed := filepath.Join(scope.Store, "loqui")
			for _, path := range append(tc.want, "skills/loqui/SKILL.md", "skills/loqui/languages/python/README.md", "skills/loqui/principles.md") {
				if _, err := os.Stat(filepath.Join(deployed, path)); err != nil {
					t.Errorf("%s is not deployed: %v", path, err)
				}
			}
			for _, path := range tc.absent {
				if _, err := os.Lstat(filepath.Join(deployed, path)); err == nil {
					t.Errorf("%s is deployed outside its skill", path)
				}
			}
			if status := git(t, pkg, "status", "--porcelain", "--untracked-files=all"); status != "" {
				t.Errorf("sync changed the package checkout:\n%s", status)
			}
		})
	}
}

func TestSyncLinkedPackageGainingResourcesLeavesItsCheckoutAlone(t *testing.T) {
	realPhora(t)
	root := t.TempDir()
	pkg := filepath.Join(root, "loqui")
	commitPackage(t, pkg, map[string]string{
		"henia.toml":                 "",
		"skills/loqui/SKILL.md":      "---\nname: loqui\n---\n",
		"languages/python/README.md": "# Python\n",
	})
	manifest := filepath.Join(root, "henia.toml")
	if err := os.WriteFile(manifest, []byte("[dependencies.loqui]\npath = \""+pkg+"\"\nlink = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scope := Scope{
		Manifest: manifest,
		Lock:     filepath.Join(root, "henia.lock"),
		State:    filepath.Join(root, "state"),
		Store:    filepath.Join(root, "store"),
		Tools:    filepath.Join(root, "tools"),
	}
	if _, err := Sync(context.Background(), scope, false); err != nil {
		t.Fatal(err)
	}
	commitPackage(t, pkg, map[string]string{
		"skills/loqui/SKILL.md": "---\nname: loqui\nhenia:\n  resources:\n    package:\n      languages/: languages/\n---\n",
	})

	if _, err := Sync(context.Background(), scope, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(scope.Store, "loqui", "skills", "loqui", "languages", "python", "README.md")); err != nil {
		t.Errorf("the resource is not deployed into its skill: %v", err)
	}
	if status := git(t, pkg, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Errorf("sync changed the linked checkout:\n%s", status)
	}
}
