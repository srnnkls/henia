package similarity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	repoID     = regexp.MustCompile(`^[A-Za-z0-9][\w.-]*/[A-Za-z0-9][\w.-]*$`)
	commitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// IsRepoID reports whether s has the form of a Hugging Face repo id, org/name.
func IsRepoID(s string) bool {
	return repoID.MatchString(s) && !strings.Contains(s, "..")
}

// HubCache returns the Hugging Face hub cache: $HF_HUB_CACHE, else
// $HF_HOME/hub, else ~/.cache/huggingface/hub.
func HubCache() (string, error) {
	if dir := os.Getenv("HF_HUB_CACHE"); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv("HF_HOME"); dir != "" {
		return filepath.Join(dir, "hub"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "huggingface", "hub"), nil
}

// Snapshot resolves a repo revision to its snapshot directory in the local
// Hugging Face cache. A revision names a ref, main by default, or a commit hash.
func Snapshot(repo, revision string) (string, error) {
	if !IsRepoID(repo) {
		return "", fmt.Errorf("semantic model %q is not a Hugging Face repo id (org/name)", repo)
	}
	if revision == "" {
		revision = "main"
	}
	if filepath.IsAbs(revision) || strings.Contains(revision, "..") {
		return "", fmt.Errorf("semantic model %s: invalid revision %q", repo, revision)
	}
	cache, err := HubCache()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "models--"+strings.ReplaceAll(repo, "/", "--"))
	hash := revision
	if !commitHash.MatchString(revision) {
		ref := filepath.Join(dir, "refs", filepath.FromSlash(revision))
		data, err := os.ReadFile(ref)
		if errors.Is(err, os.ErrNotExist) {
			return "", missing(repo, revision, ref)
		}
		if err != nil {
			return "", err
		}
		if hash = strings.TrimSpace(string(data)); !commitHash.MatchString(hash) {
			return "", fmt.Errorf("semantic model %s@%s: %s holds no commit hash", repo, revision, ref)
		}
	}
	snapshot := filepath.Join(dir, "snapshots", hash)
	if info, err := os.Stat(snapshot); err != nil || !info.IsDir() {
		return "", missing(repo, revision, snapshot)
	}
	return snapshot, nil
}

func missing(repo, revision, path string) error {
	download := "hf download " + repo
	if revision != "main" {
		download += " --revision " + revision
	}
	return fmt.Errorf("semantic model %s@%s is not in the Hugging Face cache: %s does not exist; run `%s`", repo, revision, path, download)
}
