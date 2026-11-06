package artifact

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// PackageResource is a resource a skill takes from elsewhere in its package.
// Source is relative to the package root, the directory holding skills/, and
// Path is where it lands inside the skill. A trailing slash marks a directory.
type PackageResource struct {
	Path   string
	Source string
}

func (r PackageResource) Dir() bool { return strings.HasSuffix(r.Path, "/") }

func (a *Artifact) PackageResources() ([]PackageResource, error) {
	settings, _ := a.Frontmatter["henia"].(map[string]any)
	resources, _ := settings["resources"].(map[string]any)
	declared, ok := resources["package"]
	if !ok {
		return nil, nil
	}
	table, ok := declared.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("henia.resources.package maps paths in the skill to paths in the package")
	}
	var out []PackageResource
	for at, value := range table {
		source, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("henia.resources.package.%s: the package path is a string", at)
		}
		if strings.HasSuffix(at, "/") != strings.HasSuffix(source, "/") {
			return nil, fmt.Errorf("henia.resources.package: %s and %s are not both directories; end both with / or neither", at, source)
		}
		for _, p := range []string{at, source} {
			trimmed := strings.TrimSuffix(p, "/")
			if trimmed == "." || path.Clean(trimmed) != trimmed || strings.Contains(trimmed, "\\") || !filepath.IsLocal(trimmed) {
				return nil, fmt.Errorf("henia.resources.package: %q is not a relative path inside the package", p)
			}
		}
		if first, _, _ := strings.Cut(at, "/"); strings.HasPrefix(first, ".") || at == "SKILL.md" {
			return nil, fmt.Errorf("henia.resources.package: %s cannot hold a resource", at)
		}
		out = append(out, PackageResource{Path: at, Source: source})
	}
	slices.SortFunc(out, func(a, b PackageResource) int { return strings.Compare(a.Path, b.Path) })
	for i := 1; i < len(out); i++ {
		if out[i-1].Dir() && strings.HasPrefix(out[i].Path, out[i-1].Path) {
			return nil, fmt.Errorf("henia.resources.package: %s lies inside %s", out[i].Path, out[i-1].Path)
		}
	}
	return out, nil
}

// SkillResources reads the package resources the SKILL.md in skillDir
// declares.
func SkillResources(skillDir string) ([]PackageResource, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	art, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return art.PackageResources()
}

func PackageSource(resources []PackageResource, rel string) (PackageResource, string, bool) {
	rel = path.Clean(rel)
	for _, r := range resources {
		at := strings.TrimSuffix(r.Path, "/")
		switch {
		case rel == at:
			return r, strings.TrimSuffix(r.Source, "/"), true
		case r.Dir() && strings.HasPrefix(rel, r.Path):
			return r, r.Source + strings.TrimPrefix(rel, r.Path), true
		}
	}
	return PackageResource{}, "", false
}
