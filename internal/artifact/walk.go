package artifact

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Filter decides whether the walk below root keeps path.
type Filter func(root, path string, info fs.FileInfo) bool

// SkippedDir reports whether a directory named name holds tooling or vendored
// files rather than authored ones.
func SkippedDir(name string) bool {
	return slices.Contains([]string{".git", "node_modules", "vendor"}, name)
}

// Files returns the regular files at or below root, in walk order, that Git
// does not ignore and file accepts. Directories dir rejects are skipped with
// their contents. Both filters see root itself; Git ignores apply only below it.
func Files(ctx context.Context, root string, dir, file Filter) ([]string, error) {
	ignored, err := GitIgnored(ctx, root)
	if err != nil {
		return nil, err
	}
	var files []string
	err = Walk(root, func(path string, info fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if absolute, err := filepath.Abs(path); err == nil && path != root && ignored[absolute] {
			if info.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if !dir(root, path, info) {
				return fs.SkipDir
			}
			return nil
		}
		if info.Mode().IsRegular() && file(root, path, info) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	return files, nil
}

// GitIgnored returns the absolute paths Git ignores below root. Ignored
// directories are reported once rather than per file. Outside a repository,
// or without git, nothing is ignored.
func GitIgnored(ctx context.Context, root string) (map[string]bool, error) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory").Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, nil
	}
	ignored := make(map[string]bool)
	for entry := range strings.SplitSeq(string(out), "\x00") {
		if entry != "" {
			ignored[filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(entry, "/")))] = true
		}
	}
	return ignored, nil
}

// Walk visits root and everything below it through logical paths, following
// file and directory symlinks. Info describes the link target. Returning
// fs.SkipDir from a directory skips its contents.
func Walk(root string, fn func(path string, info fs.FileInfo) error) error {
	return walk(root, nil, fn)
}

func walk(path string, ancestors []fs.FileInfo, fn func(string, fs.FileInfo) error) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err := fn(path, info); err != nil || !info.IsDir() {
		if err == fs.SkipDir {
			return nil
		}
		return err
	}
	if slices.ContainsFunc(ancestors, func(ancestor fs.FileInfo) bool { return os.SameFile(ancestor, info) }) {
		return fmt.Errorf("symlink cycle at %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	ancestors = append(ancestors, info)
	for _, entry := range entries {
		if err := walk(filepath.Join(path, entry.Name()), ancestors, fn); err != nil {
			return err
		}
	}
	return nil
}
