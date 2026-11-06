package library

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/artifact"
)

const (
	excludeStart = "# henia: package resources linked into skills"
	excludeEnd   = "# henia: end"
)

// LinkResources links each package resource the skills below root declare
// to where a deployment places it, listing the links in the repository's
// info/exclude; with link off, it removes the links it listed.
func LinkResources(root string, link bool) error {
	exclude, prefix := excludeFile(root)
	previous := linkedPaths(exclude)
	var wanted []string
	if link {
		entries, err := os.ReadDir(filepath.Join(root, SkillsDir))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, entry := range entries {
			skill := filepath.Join(root, SkillsDir, entry.Name())
			resources, err := artifact.SkillResources(skill)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: %w", skill, err)
			}
			for _, r := range resources {
				at := filepath.Join(skill, filepath.FromSlash(strings.TrimSuffix(r.Path, "/")))
				target, err := filepath.Rel(filepath.Dir(at), filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(r.Source, "/"))))
				if err != nil {
					return err
				}
				if err := placeLink(at, target); err != nil {
					return err
				}
				rel, _ := filepath.Rel(root, at)
				wanted = append(wanted, prefix+filepath.ToSlash(rel))
			}
		}
	}
	for _, path := range previous {
		if slices.Contains(wanted, path) {
			continue
		}
		at := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, prefix)))
		if info, err := os.Lstat(at); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(at); err != nil {
				return err
			}
		}
	}
	if exclude == "" || slices.Equal(previous, wanted) {
		return nil
	}
	return writeExclude(exclude, wanted)
}

func placeLink(at, target string) error {
	info, err := os.Lstat(at)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	case info.Mode()&os.ModeSymlink == 0:
		return fmt.Errorf("%s exists, so the package resource declared there cannot be linked", at)
	default:
		if current, err := os.Readlink(at); err == nil && current == target {
			return nil
		}
		if err := os.Remove(at); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, at)
}

// excludeFile also returns the prefix that roots root's paths at the
// repository; outside a repository it returns none.
func excludeFile(root string) (string, string) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude", "--show-prefix").Output()
	if err != nil {
		return "", ""
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 2 {
		return "", ""
	}
	return lines[0], "/" + lines[1]
}

func linkedPaths(exclude string) []string {
	data, err := os.ReadFile(exclude)
	if err != nil {
		return nil
	}
	_, block, found := strings.Cut(string(data), excludeStart+"\n")
	if !found {
		return nil
	}
	block, _, _ = strings.Cut(block, excludeEnd)
	return strings.Fields(block)
}

func writeExclude(exclude string, paths []string) error {
	data, err := os.ReadFile(exclude)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if start := bytes.Index(data, []byte(excludeStart+"\n")); start >= 0 {
		end := len(data)
		if i := bytes.Index(data[start:], []byte(excludeEnd+"\n")); i >= 0 {
			end = start + i + len(excludeEnd) + 1
		}
		data = append(data[:start:start], data[end:]...)
	}
	if len(paths) > 0 {
		if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
			data = append(data, '\n')
		}
		data = append(data, excludeStart+"\n"+strings.Join(paths, "\n")+"\n"+excludeEnd+"\n"...)
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	return os.WriteFile(exclude, data, 0o644)
}
