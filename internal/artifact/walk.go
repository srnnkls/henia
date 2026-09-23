package artifact

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

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
