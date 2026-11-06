package library

import (
	"errors"
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
	seen := map[string]bool{}
	var visit func(dir string)
	visit = func(dir string) {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || seen[real] {
			return
		}
		seen[real] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			switch info, err := os.Stat(path); {
			case strings.HasPrefix(entry.Name(), "."):
			case err == nil && info.IsDir():
				visit(path)
			case dir != root || entry.Name() != "SKILL.md":
				files = append(files, path)
			}
		}
	}
	visit(root)
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
		if within(root, full) {
			return filepath.Join(skillDir, clean), nil
		}
	}
	declared, _ := artifact.SkillResources(skillDir)
	if r, _, ok := artifact.PackageSource(declared, filepath.ToSlash(clean)); ok {
		for _, root := range roots {
			source, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(filepath.Dir(root)), filepath.FromSlash(strings.TrimSuffix(r.Source, "/"))))
			if err == nil && within(source, full) {
				return filepath.Join(skillDir, clean), nil
			}
		}
	}
	return "", errOutsideSkill
}

// PackageFile maps local, a missing file inside a skill, to the package file
// a resource the skill declares deploys there.
func PackageFile(local string) (string, bool) {
	for dir := filepath.Dir(local); filepath.Dir(dir) != dir; dir = filepath.Dir(dir) {
		if filepath.Base(filepath.Dir(dir)) != SkillsDir {
			continue
		}
		rel, err := filepath.Rel(dir, local)
		if err != nil {
			return "", false
		}
		declared, _ := artifact.SkillResources(dir)
		if _, source, ok := artifact.PackageSource(declared, filepath.ToSlash(rel)); ok {
			return filepath.Join(filepath.Dir(filepath.Dir(dir)), filepath.FromSlash(source)), true
		}
		return "", false
	}
	return "", false
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
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
