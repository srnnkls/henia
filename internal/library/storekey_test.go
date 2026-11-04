package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func checkout(t *testing.T, manifest, lock string) (string, string) {
	t.Helper()
	project := t.TempDir()
	path := filepath.Join(project, ConfigFile)
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if lock != "" {
		if err := os.WriteFile(filepath.Join(project, LockFile), []byte(lock), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return project, path
}

const gitManifest = "[dependencies.pkg]\ngit = \"https://example.com/pkg.git\"\nbranch = \"main\"\n"

func TestStoreKeyIsSharedByCheckoutsOfOneLock(t *testing.T) {
	lock := "version = 1\n\n[[sources]]\nname = \"pkg\"\ncommit = \"aaaa\"\n"
	first, firstManifest := checkout(t, gitManifest, lock)
	second, secondManifest := checkout(t, gitManifest, lock)

	key := StoreKey(first, firstManifest)
	if key != StoreKey(second, secondManifest) {
		t.Fatalf("two checkouts of one lock key %q and %q", key, StoreKey(second, secondManifest))
	}
	if !strings.HasPrefix(key, "lock-") {
		t.Fatalf("a lock-keyed store is named lock-<digest>, got %q", key)
	}
}

func TestStoreKeyFollowsTheLock(t *testing.T) {
	project, manifest := checkout(t, gitManifest, "version = 1\n\n[[sources]]\nname = \"pkg\"\ncommit = \"aaaa\"\n")
	before := StoreKey(project, manifest)
	if err := os.WriteFile(filepath.Join(project, LockFile), []byte("version = 1\n\n[[sources]]\nname = \"pkg\"\ncommit = \"bbbb\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if after := StoreKey(project, manifest); after == before {
		t.Fatalf("a moved lock keeps the store %q", before)
	}
}

func TestStoreKeyOfAPathDependencyStaysWithTheCheckout(t *testing.T) {
	project, manifest := checkout(t, "[dependencies.pkg]\npath = \"../pkg\"\n", "")

	if got, want := StoreKey(project, manifest), ProjectKey(project); got != want {
		t.Fatalf("a path dependency deploys a working copy no lock records: key %q, want the checkout's %q", got, want)
	}
}

func TestPruneStoresRemovesOnlyStoresNothingUsed(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	old := time.Now().Add(-48 * time.Hour)
	for _, key := range []string{"lock-current", "lock-stale", "lock-recent", "global"} {
		for _, dir := range []string{PackagesDir(key), StateDir(key)} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if key != "lock-recent" {
				if err := os.Chtimes(dir, old, old); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	if err := PruneStores("lock-current", 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	for key, kept := range map[string]bool{"lock-current": true, "lock-stale": false, "lock-recent": true, "global": true} {
		for _, dir := range []string{PackagesDir(key), StateDir(key)} {
			if _, err := os.Stat(dir); (err == nil) != kept {
				t.Errorf("%s kept=%v, want %v", dir, err == nil, kept)
			}
		}
	}
}

func TestStoreKeyOfAProjectWithoutDependenciesStaysWithTheCheckout(t *testing.T) {
	project, manifest := checkout(t, "[harness.codex]\n", "")

	if got, want := StoreKey(project, manifest), ProjectKey(project); got != want {
		t.Fatalf("a project without dependencies shares nothing: key %q, want the checkout's %q", got, want)
	}
}
