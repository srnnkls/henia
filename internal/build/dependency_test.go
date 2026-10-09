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
	put(t, filepath.Join(project, "skills/own/SKILL.md"), "---\nname: own\ndescription: Own skill.\n---\n\nFlavor: :flavor[].\n")
	put(t, filepath.Join(dependency, "skills/dep/SKILL.md"), "---\nname: dep\ndescription: Dependency skill.\n---\n\nFlavor: :flavor[], mode: :mode[].\n")
	value := func(name string) string { return "---\nform: text\nbody:\n  kind: none\n---\n{{." + name + "}}\n" }
	put(t, filepath.Join(project, ".henia/components/flavor.md"), value("flavor"))
	put(t, filepath.Join(dependency, ".henia/components/flavor.md"), value("flavor"))
	put(t, filepath.Join(dependency, ".henia/components/mode.md"), value("mode"))
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
			harnesses := map[string]henia.Harness{"claude": {Artifacts: []string{"skills"}, Variables: test.project, ProjectRoot: project}}
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

func TestDependencyComponents(t *testing.T) {
	root := t.TempDir()
	project, dependency, user := filepath.Join(root, "project"), filepath.Join(root, "project", ".henia", "sources", "dep"), filepath.Join(root, "config")
	static := func(text string) string { return "---\nform: text\nbody:\n  kind: none\n---\n" + text + "\n" }
	put(t, filepath.Join(project, ".henia/components/intro.md"), static("Project static."))
	put(t, filepath.Join(dependency, ".henia/components/intro.md"), static("Dependency static."))
	put(t, filepath.Join(user, "components/footer.md"), static("User footer."))
	put(t, filepath.Join(project, "skills/own/SKILL.md"), "---\nname: own\ndescription: Own skill.\n---\n\n:intro[] :footer[]\n")
	put(t, filepath.Join(dependency, "skills/dep/SKILL.md"), "---\nname: dep\ndescription: Dependency skill.\n---\n\n:intro[]\n")
	harnesses := map[string]henia.Harness{"claude": {Artifacts: []string{"skills"}, ProjectRoot: project, UserRoot: user}}
	output := filepath.Join(root, "build")
	result, err := Run(t.Context(), []string{project, dependency}, output, harnesses, Dependency{Root: dependency})
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("%v %v", err, result.Errors)
	}
	for skill, want := range map[string]string{"own": "Project static. User footer.", "dep": "Dependency static."} {
		data, err := os.ReadFile(filepath.Join(output, "claude", "skills", skill, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s: %q lacks %q", skill, data, want)
		}
	}

	put(t, filepath.Join(dependency, ".henia/templates/head.md.tmpl"), "Dependency template.")
	result, err = Run(t.Context(), []string{project, dependency}, filepath.Join(root, "removed"), harnesses, Dependency{Root: dependency})
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "[removed-composition]") {
			removed = append(removed, e.Error())
		}
	}
	if len(removed) != 1 || !strings.Contains(removed[0], filepath.Join(dependency, ".henia", "templates")) {
		t.Fatalf("dependency templates directory: %v", result.Errors)
	}
}
