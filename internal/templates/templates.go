// Package templates loads the shared body templates that skills compose.
package templates

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	Suffix = ".md.tmpl"
	Dir    = "templates"
)

type File struct {
	Name string
	Path string
	User bool
}

func Dirs(project, user string) []string {
	var dirs []string
	if project != "" {
		dirs = append(dirs, filepath.Join(project, ".henia", Dir))
	}
	if user != "" {
		dirs = append(dirs, filepath.Join(user, Dir))
	}
	return dirs
}

// Files lists project and user templates; a project template shadows a user
// template of the same name.
func Files(project, user string) []File {
	var files []File
	seen := map[string]bool{}
	for i, dir := range Dirs(project, user) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name, ok := strings.CutSuffix(entry.Name(), Suffix)
			if !ok || name == "" || entry.IsDir() || seen[name] {
				continue
			}
			seen[name] = true
			files = append(files, File{Name: name, Path: filepath.Join(dir, entry.Name()), User: project == "" || i > 0})
		}
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Name, b.Name) })
	return files
}

func Load(project, user string) (map[string]string, error) {
	files := Files(project, user)
	if len(files) == 0 {
		return nil, nil
	}
	loaded := make(map[string]string, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		loaded[f.Name] = string(data)
	}
	return loaded, nil
}

func Digest(loaded map[string]string) string {
	h := sha256.New()
	for _, name := range slices.Sorted(maps.Keys(loaded)) {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(loaded[name]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
