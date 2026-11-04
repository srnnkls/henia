package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/library"
)

func gitIn(t *testing.T, dir string, args ...string) string {
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

func TestCheckoutsSwitchToALockThatDropsADeployedFile(t *testing.T) {
	if os.Getenv("HENIA_PHORA") == "" {
		t.Skip("needs a real phora in HENIA_PHORA")
	}
	for _, env := range []string{"XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(env, t.TempDir())
	}
	root := t.TempDir()
	pkg := filepath.Join(root, "pkg")
	fixture(t, filepath.Join(pkg, "skills", "pkg", "SKILL.md"), "---\nname: pkg\ndescription: pkg.\n---\n")
	fixture(t, filepath.Join(pkg, "extra.txt"), "dropped later\n")
	gitIn(t, pkg, "init", "-q", "-b", "main")
	gitIn(t, pkg, "add", "-A")
	gitIn(t, pkg, "commit", "-q", "-m", "first")
	manifest := "[dependencies.pkg]\ngit = \"file://" + pkg + "\"\nbranch = \"main\"\n"
	checkouts := map[string]string{}
	for _, name := range []string{"a", "b"} {
		checkouts[name] = filepath.Join(root, name)
		fixture(t, filepath.Join(checkouts[name], library.ConfigFile), manifest)
	}
	sync := func(name string, update bool) {
		t.Helper()
		if _, _, err := syncProject(context.Background(), checkouts[name], &bytes.Buffer{}, update); err != nil {
			t.Fatalf("sync %s: %v", name, err)
		}
	}
	lock := func(name string) string { return filepath.Join(checkouts[name], library.LockFile) }
	copyLock := func(from, to string) {
		t.Helper()
		data, err := os.ReadFile(lock(from))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lock(to), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sync("a", false)
	copyLock("a", "b")
	gitIn(t, pkg, "rm", "-q", "extra.txt")
	gitIn(t, pkg, "commit", "-q", "-m", "drop extra")
	sync("b", true)

	copyLock("b", "a")
	sync("a", false)

	keyA := library.StoreKey(checkouts["a"], filepath.Join(checkouts["a"], library.ConfigFile))
	if keyB := library.StoreKey(checkouts["b"], filepath.Join(checkouts["b"], library.ConfigFile)); keyA != keyB {
		t.Fatalf("checkouts on one lock use the stores %q and %q", keyA, keyB)
	}
	if _, err := os.Stat(filepath.Join(library.PackagesDir(keyA), "pkg", "extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("the store of the new lock still holds the dropped file: %v", err)
	}
}
