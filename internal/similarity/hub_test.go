package similarity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hash = "6fc8051fab2a1e0ee76689cf08c853792ac285e7"

func cachedModel(t *testing.T, cache, ref string) string {
	t.Helper()
	dir := filepath.Join(cache, "models--minishlab--potion-retrieval-32M")
	snapshot := filepath.Join(dir, "snapshots", hash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatal(err)
	}
	if ref != "" {
		path := filepath.Join(dir, "refs", ref)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(hash), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}

func TestHubCacheResolutionOrder(t *testing.T) {
	home, hfHome, hubCache := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HF_HOME", hfHome)
	t.Setenv("HF_HUB_CACHE", hubCache)
	for _, want := range []string{hubCache, filepath.Join(hfHome, "hub"), filepath.Join(home, ".cache", "huggingface", "hub")} {
		got, err := HubCache()
		if err != nil || got != want {
			t.Fatalf("HubCache() = %q, %v; want %q", got, err, want)
		}
		if os.Getenv("HF_HUB_CACHE") != "" {
			t.Setenv("HF_HUB_CACHE", "")
		} else {
			t.Setenv("HF_HOME", "")
		}
	}
}

func TestSnapshotFollowsRef(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	want := cachedModel(t, cache, "main")
	for _, revision := range []string{"", "main"} {
		if got, err := Snapshot("minishlab/potion-retrieval-32M", revision); err != nil || got != want {
			t.Fatalf("Snapshot(%q) = %q, %v; want %q", revision, got, err, want)
		}
	}
	cachedModel(t, cache, "pr/1")
	if got, err := Snapshot("minishlab/potion-retrieval-32M", "pr/1"); err != nil || got != want {
		t.Fatalf("Snapshot(pr/1) = %q, %v", got, err)
	}
}

func TestSnapshotTakesCommitHash(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	want := cachedModel(t, cache, "")
	if got, err := Snapshot("minishlab/potion-retrieval-32M", hash); err != nil || got != want {
		t.Fatalf("Snapshot(hash) = %q, %v; want %q", got, err, want)
	}
}

func TestSnapshotMissingModel(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HF_HUB_CACHE", cache)
	_, err := Snapshot("minishlab/potion-retrieval-32M", "")
	ref := filepath.Join(cache, "models--minishlab--potion-retrieval-32M", "refs", "main")
	if err == nil || !strings.Contains(err.Error(), ref) || !strings.Contains(err.Error(), "`hf download minishlab/potion-retrieval-32M`") {
		t.Fatalf("missing model: %v", err)
	}
	_, err = Snapshot("minishlab/potion-retrieval-32M", hash)
	snapshot := filepath.Join(cache, "models--minishlab--potion-retrieval-32M", "snapshots", hash)
	if err == nil || !strings.Contains(err.Error(), snapshot) || !strings.Contains(err.Error(), "`hf download minishlab/potion-retrieval-32M --revision "+hash+"`") {
		t.Fatalf("missing snapshot: %v", err)
	}
	for _, repo := range []string{"potion", "../x", "a/b/c", "/abs/path"} {
		if _, err := Snapshot(repo, ""); err == nil || !strings.Contains(err.Error(), "not a Hugging Face repo id") {
			t.Fatalf("Snapshot(%q): %v", repo, err)
		}
	}
	if _, err := Snapshot("minishlab/potion-retrieval-32M", "../../escape"); err == nil || !strings.Contains(err.Error(), "invalid revision") {
		t.Fatalf("escaping revision: %v", err)
	}
}
