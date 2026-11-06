package library

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResources(t *testing.T) {
	skill := t.TempDir()
	write(t, filepath.Join(skill, "SKILL.md"), "---\nname: s\n---\n")
	write(t, filepath.Join(skill, "reference/config.md"), "---\ndescription: Configuration keys\n  and defaults.\n---\n\n# Ignored\n")
	write(t, filepath.Join(skill, "reference/flow.md"), "Intro.\n\n# Flow of a task\n")
	write(t, filepath.Join(skill, "scripts/run.sh"), "echo\n")
	write(t, filepath.Join(skill, ".hidden/x.md"), "# Hidden\n")
	got := Resources(skill, "")
	want := []Resource{
		{Path: "reference/config.md", Description: "Configuration keys and defaults."},
		{Path: "reference/flow.md", Description: "Flow of a task"},
		{Path: "scripts/run.sh"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestResourcesCollapse(t *testing.T) {
	skill := t.TempDir()
	write(t, filepath.Join(skill, "guide.md"), "# Guide\n")
	for i := range 20 {
		write(t, filepath.Join(skill, fmt.Sprintf("refs/lang/go/%02d.md", i)), "# Go\n")
		write(t, filepath.Join(skill, fmt.Sprintf("refs/lang/py/%02d.md", i)), "# Py\n")
	}
	write(t, filepath.Join(skill, "refs/lang/go/README.md"), "---\ndescription: Go guidance.\n---\n")
	top := Resources(skill, "")
	if !slices.Equal(top, []Resource{{Path: "guide.md", Description: "Guide"}, {Path: "refs/", Files: 41}}) {
		t.Fatalf("top: %+v", top)
	}
	nested := Resources(skill, "refs")
	if !slices.Equal(nested, []Resource{{Path: "refs/lang/go/", Description: "Go guidance.", Files: 21}, {Path: "refs/lang/py/", Files: 20}}) {
		t.Fatalf("nested: %+v", nested)
	}
}

func TestResourcePath(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout/skills/s")
	write(t, filepath.Join(checkout, "SKILL.md"), "---\nname: s\n---\n")
	write(t, filepath.Join(checkout, "reference/a.md"), "---\ndescription: A.\n---\nBody\n")
	write(t, filepath.Join(root, "secret"), "secret\n")
	installed := filepath.Join(root, "installed/skills/s")
	if err := os.MkdirAll(filepath.Join(installed, "reference"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, link := range [][2]string{
		{filepath.Join(checkout, "SKILL.md"), filepath.Join(installed, "SKILL.md")},
		{filepath.Join(checkout, "reference/a.md"), filepath.Join(installed, "reference/a.md")},
		{filepath.Join(root, "secret"), filepath.Join(installed, "reference/leak.md")},
	} {
		if err := os.Symlink(link[0], link[1]); err != nil {
			t.Fatal(err)
		}
	}
	full, err := ResourcePath(installed, "reference/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if text, err := ReadResource(full); err != nil || text != "Body\n" {
		t.Fatalf("read %q %v", text, err)
	}
	for _, path := range []string{"../../../secret", "/etc/passwd", "reference/leak.md", "reference/../../s2/x"} {
		if _, err := ResourcePath(installed, path); err == nil {
			t.Errorf("%s: allowed", path)
		}
	}
}

func TestPackageResourcesResolveThroughTheirLinks(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills/loqui")
	write(t, filepath.Join(skill, "SKILL.md"), "---\nname: loqui\nhenia:\n  resources:\n    package:\n      languages/: languages/\n---\n")
	write(t, filepath.Join(root, "languages/python/README.md"), "# Python\n")
	write(t, filepath.Join(root, "unrelated/secret.md"), "secret\n")
	for _, link := range [][2]string{{"../../languages", "languages"}, {"../../unrelated", "other"}} {
		if err := os.Symlink(link[0], filepath.Join(skill, link[1])); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ResourcePath(skill, "languages/python/README.md"); err != nil {
		t.Errorf("declared resource: %v", err)
	}
	if _, err := ResourcePath(skill, "other/secret.md"); err == nil {
		t.Error("an undeclared link out of the skill resolves")
	}
	var paths []string
	for _, r := range Resources(skill, "languages") {
		paths = append(paths, r.Path)
	}
	if !slices.Equal(paths, []string{"languages/python/README.md"}) {
		t.Errorf("resources = %v", paths)
	}
}

func TestPackageFileMapsAMissingResourceToItsSource(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills/loqui")
	write(t, filepath.Join(skill, "SKILL.md"), "---\nname: loqui\nhenia:\n  resources:\n    package:\n      guides/: languages/\n---\n")

	got, ok := PackageFile(filepath.Join(skill, "guides/python/README.md"))
	if want := filepath.Join(root, "languages/python/README.md"); !ok || got != want {
		t.Errorf("PackageFile = %q %v, want %q", got, ok, want)
	}
	if _, ok := PackageFile(filepath.Join(skill, "reference/a.md")); ok {
		t.Error("an undeclared path maps")
	}
}
