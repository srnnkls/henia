package library

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkResourcesLinksDeclaredResourcesOutOfGit(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	skill := filepath.Join(root, "skills/loqui")
	write(t, filepath.Join(skill, "SKILL.md"), "---\nname: loqui\nhenia:\n  resources:\n    package:\n      languages/: languages/\n---\n")
	write(t, filepath.Join(root, "languages/python/README.md"), "# Python\n")
	status := func() string {
		out, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before := status()

	if err := LinkResources(root, true); err != nil {
		t.Fatal(err)
	}

	if target, err := os.Readlink(filepath.Join(skill, "languages")); err != nil || target != filepath.Join("..", "..", "languages") {
		t.Errorf("link = %q (%v)", target, err)
	}
	if _, err := ResourcePath(skill, "languages/python/README.md"); err != nil {
		t.Errorf("linked resource: %v", err)
	}
	if after := status(); after != before {
		t.Errorf("git sees the link:\n%s", after)
	}
	if err := LinkResources(root, true); err != nil {
		t.Fatalf("relinking: %v", err)
	}

	if err := LinkResources(root, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(skill, "languages")); err == nil {
		t.Error("the link survives link = false")
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".git/info/exclude")); strings.Contains(string(data), "skills/loqui") {
		t.Errorf("exclude keeps the link:\n%s", data)
	}
}
