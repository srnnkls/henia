package artifact

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("# "+path+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func relFiles(t *testing.T, root string, files []string) []string {
	t.Helper()
	var got []string
	for _, file := range files {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.ToSlash(rel))
	}
	slices.Sort(got)
	return got
}

func keepAll(string, string, fs.FileInfo) bool { return true }

func authoredDirs(root, path string, info fs.FileInfo) bool {
	return path == root || !strings.HasPrefix(info.Name(), ".") && !SkippedDir(info.Name())
}

func markdown(_, path string, _ fs.FileInfo) bool { return filepath.Ext(path) == ".md" }

func TestFilesAppliesFiltersToRoot(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "a.md", "sub/b.md")

	var seenRoots []string
	rejectRoot := func(r, path string, _ fs.FileInfo) bool {
		seenRoots = append(seenRoots, r)
		return path != root
	}
	files, err := Files(t.Context(), root, rejectRoot, keepAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("dir filter rejecting the root: files = %v, want none", files)
	}
	if !slices.Equal(seenRoots, []string{root}) {
		t.Errorf("dir filter saw roots %v, want [%s]", seenRoots, root)
	}

	file := filepath.Join(root, "a.md")
	rejectFile := func(r, path string, _ fs.FileInfo) bool { return !(r == file && path == file) }
	if files, err := Files(t.Context(), file, keepAll, rejectFile); err != nil || len(files) != 0 {
		t.Errorf("file filter rejecting a file root: files = %v, err = %v, want none", files, err)
	}
	if files, err := Files(t.Context(), file, keepAll, keepAll); err != nil || !slices.Equal(files, []string{file}) {
		t.Errorf("file root accepted: files = %v, err = %v, want [%s]", files, err, file)
	}
}

func TestFilesSkipsDotAndToolingDirsBelowRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vendor")
	writeTree(t, root,
		"a.md",
		"notes.txt",
		".hidden/b.md",
		"node_modules/c.md",
		"vendor/d.md",
		".git/e.md",
		"deep/f.md",
		"deep/vendor/g.md",
	)

	files, err := Files(t.Context(), root, authoredDirs, markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := relFiles(t, root, files), []string{"a.md", "deep/f.md"}; !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
}

func TestFilesSkipsGitIgnoredBelowRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored.md\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTree(t, repo, "docs/kept.md", "docs/ignored.md", "docs/build/out.md", "build/inner.md")

	docs := filepath.Join(repo, "docs")
	files, err := Files(t.Context(), docs, authoredDirs, markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := relFiles(t, docs, files), []string{"kept.md"}; !slices.Equal(got, want) {
		t.Errorf("below an unignored root: files = %v, want %v", got, want)
	}

	build := filepath.Join(repo, "build")
	files, err = Files(t.Context(), build, authoredDirs, markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := relFiles(t, build, files), []string{"inner.md"}; !slices.Equal(got, want) {
		t.Errorf("ignored root itself: files = %v, want %v", got, want)
	}
}
