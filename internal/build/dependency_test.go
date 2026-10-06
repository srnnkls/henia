package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	henia "github.com/srnnkls/henia"
)

func TestDependencyHarnessDefaults(t *testing.T) {
	root := t.TempDir()
	project, dependency := filepath.Join(root, "project"), filepath.Join(root, "project", ".henia", "sources", "dep")
	put(t, filepath.Join(project, "skills/own/SKILL.md"), "---\nname: own\ndescription: Own skill.\n---\n\nFlavor: {{.flavor}}.\n")
	put(t, filepath.Join(dependency, "skills/dep/SKILL.md"), "---\nname: dep\ndescription: Dependency skill.\n---\n\nFlavor: {{.flavor}}, mode: {{.mode}}.\n")
	dependencies := []Dependency{{Root: dependency, Harnesses: map[string]henia.Harness{"claude": {Variables: map[string]string{"flavor": "dependency", "mode": "strict"}}}}}
	for _, test := range []struct {
		name     string
		project  map[string]string
		own, dep string
	}{
		{"dependency defaults", map[string]string{}, "Flavor: .", "Flavor: dependency, mode: strict."},
		{"project overrides", map[string]string{"flavor": "project"}, "Flavor: project.", "Flavor: project, mode: strict."},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(root, test.name)
			harnesses := map[string]henia.Harness{"claude": {Artifacts: []string{"skills"}, Variables: test.project}}
			result, err := Run(t.Context(), []string{project, dependency}, output, harnesses, dependencies...)
			if err != nil || len(result.Errors) != 0 {
				t.Fatalf("%v %v", err, result.Errors)
			}
			for skill, want := range map[string]string{"own": test.own, "dep": test.dep} {
				data, err := os.ReadFile(filepath.Join(output, "claude", "skills", skill, "SKILL.md"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), want) {
					t.Errorf("%s: %q lacks %q", skill, data, want)
				}
			}
		})
	}
}
