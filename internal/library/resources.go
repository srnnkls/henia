package library

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/artifact"
)

const flatResources = 30

type Resource struct {
	Path        string
	Description string
	Files       int
}

func resourceFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && path != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && !(filepath.Dir(path) == root && d.Name() == "SKILL.md") {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func Resources(skillDir, sub string) []Resource {
	base := filepath.Join(skillDir, sub)
	files := resourceFiles(base)
	relative := func(path string) string {
		rel, _ := filepath.Rel(skillDir, path)
		return filepath.ToSlash(rel)
	}
	var out []Resource
	if len(files) <= flatResources {
		for _, file := range files {
			out = append(out, Resource{Path: relative(file), Description: describe(file)})
		}
		return out
	}
	dirs := map[string]int{}
	for _, file := range files {
		rel, _ := filepath.Rel(base, file)
		top, _, nested := strings.Cut(filepath.ToSlash(rel), "/")
		if nested {
			dirs[top]++
			continue
		}
		out = append(out, Resource{Path: relative(file), Description: describe(file)})
	}
	if len(out) == 0 && len(dirs) == 1 {
		for dir := range dirs {
			return Resources(skillDir, filepath.Join(sub, dir))
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(dirs)) {
		path := filepath.Join(base, dir)
		out = append(out, Resource{Path: relative(path) + "/", Description: describeDir(path), Files: dirs[dir]})
	}
	slices.SortFunc(out, func(a, b Resource) int { return strings.Compare(a.Path, b.Path) })
	return out
}

func describeDir(dir string) string {
	for _, index := range []string{"README.md", "index.md"} {
		if description := describe(filepath.Join(dir, index)); description != "" {
			return description
		}
	}
	return ""
}

func describe(path string) string {
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	art, err := artifact.Parse(data)
	if err != nil {
		return ""
	}
	if description, ok := art.Frontmatter["description"].(string); ok && strings.TrimSpace(description) != "" {
		return strings.Join(strings.Fields(description), " ")
	}
	for line := range strings.SplitSeq(art.Body, "\n") {
		if title, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(title)
		}
	}
	return ""
}

var errOutsideSkill = errors.New("resource paths stay inside the skill directory")

func ResourcePath(skillDir, path string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean != "." && !filepath.IsLocal(clean) {
		return "", errOutsideSkill
	}
	full, err := filepath.EvalSymlinks(filepath.Join(skillDir, clean))
	if err != nil {
		return "", err
	}
	var roots []string
	if root, err := filepath.EvalSymlinks(skillDir); err == nil {
		roots = append(roots, root)
	}
	if main, err := filepath.EvalSymlinks(filepath.Join(skillDir, "SKILL.md")); err == nil {
		roots = append(roots, filepath.Dir(main))
	}
	for _, root := range roots {
		if rel, err := filepath.Rel(root, full); err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return filepath.Join(skillDir, clean), nil
		}
	}
	return "", errOutsideSkill
}

func ReadResource(full string) (string, error) {
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Ext(full), ".md") {
		if art, err := artifact.Parse(data); err == nil {
			return art.Body, nil
		}
	}
	return string(data), nil
}
