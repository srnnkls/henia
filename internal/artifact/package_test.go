package artifact

import (
	"slices"
	"testing"
)

func TestPackageResources(t *testing.T) {
	declare := func(table any) *Artifact {
		return &Artifact{Frontmatter: map[string]any{"henia": map[string]any{"resources": map[string]any{"package": table}}}}
	}
	got, err := declare(map[string]any{"languages/": "languages/", "principles.md": "shared/principles.md"}).PackageResources()
	want := []PackageResource{{Path: "languages/", Source: "languages/"}, {Path: "principles.md", Source: "shared/principles.md"}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("PackageResources = %v (%v), want %v", got, err, want)
	}
	if r, source, ok := PackageSource(got, "languages/python/README.md"); !ok || r != want[0] || source != "languages/python/README.md" {
		t.Errorf("PackageSource = %v %q %v", r, source, ok)
	}
	if _, _, ok := PackageSource(got, "languagesx/a.md"); ok {
		t.Error("a sibling of a declared directory maps")
	}
	for name, table := range map[string]any{
		"escaping source":  map[string]any{"languages/": "../languages/"},
		"absolute path":    map[string]any{"/languages/": "languages/"},
		"directory mix":    map[string]any{"languages/": "languages"},
		"the skill itself": map[string]any{"SKILL.md": "README.md"},
		"hidden":           map[string]any{".cache/": "cache/"},
		"nested":           map[string]any{"a/": "a/", "a/b/": "b/"},
		"package root":     map[string]any{"all/": "./"},
		"not a table":      []any{"languages/"},
		"not a string":     map[string]any{"languages/": true},
	} {
		if _, err := declare(table).PackageResources(); err == nil {
			t.Errorf("%s: accepted %v", name, table)
		}
	}
}
