package lint_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/srnnkls/henia/internal/lint"
)

func write(t *testing.T, root, path, content string) string {
	t.Helper()
	path = filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRulesAndPositions(t *testing.T) {
	root := t.TempDir()
	paragraph := "These twelve or more words explain how to execute the same repeated workflow safely and consistently."
	write(t, root, "skills/example/SKILL.md", "---\nname: example\nlast_verified: 2020-01-01\n---\n\n# Repeated\n\n"+paragraph+"\n\n# Repeated\n\n"+paragraph+"\n\n[Guide](missing.md) and `$missing`. Use `legacy-model`.\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{Outdated: map[string]string{"legacy-model": "current-model"}, MaxLines: 4, Now: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diagnostics {
		got = append(got, d.Rule)
		if d.Line < 1 || d.Column < 1 {
			t.Errorf("invalid location: %+v", d)
		}
		if d.Rule == "broken-link" && d.Line != 14 {
			t.Errorf("link location: %+v", d)
		}
	}
	want := []string{"metadata", "stale-review", "large-skill", "duplicate-heading", "duplicate-content", "broken-link", "missing-reference", "outdated-reference"}
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v; diagnostics: %+v", got, want, diagnostics)
	}
}

func TestReferencesResolveAndExamplesAreIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/one/SKILL.md", "---\nname: one\ndescription: First skill\n---\n\n:::note\nSee `$two`, [Guide](reference/guide.md), `#reference/guide.md`.\n:::\n\n```md\n:x[bad\n[broken](absent.md) `$absent` legacy-model\n```\n")
	write(t, root, "skills/one/reference/guide.md", "# Guide\n\nWorking guide.\n")
	write(t, root, "skills/two/SKILL.md", "---\nname: two\ndescription: Second skill\n---\n\nA second skill.\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{Outdated: map[string]string{"legacy-model": "current-model"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestDuplicateNamesAndCrossFileContent(t *testing.T) {
	root := t.TempDir()
	content := "---\nname: same\ndescription: Example\n---\n\nThis is a long paragraph which contains enough words to detect a copy across two different skills.\n"
	write(t, root, "skills/a/SKILL.md", content)
	write(t, root, "skills/b/SKILL.md", content)
	diagnostics, err := lint.Run(t.Context(), []string{root, root}, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("expected name and paragraph duplicates, got %+v", diagnostics)
	}
	again, err := lint.Run(t.Context(), []string{root}, lint.Options{})
	if err != nil || !reflect.DeepEqual(diagnostics, again) {
		t.Fatalf("not deterministic: %+v / %+v (%v)", diagnostics, again, err)
	}
}

func TestConfigurationAndFailures(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "SKILL.md", "---\nname: one\n---\n\n:::broken\n")
	diagnostics, err := lint.Run(t.Context(), []string{path}, lint.Options{Disable: []string{"metadata"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Severity != "error" || diagnostics[0].Line != 5 {
		t.Fatalf("expected located syntax error: %+v", diagnostics)
	}
	if _, err := lint.Run(t.Context(), []string{root}, lint.Options{Disable: []string{"typo"}}); err == nil {
		t.Error("expected unknown-rule error")
	}
	if _, err := lint.Run(t.Context(), []string{filepath.Join(root, "missing")}, lint.Options{}); err == nil {
		t.Error("expected missing-path error")
	}
	write(t, root, "SKILL.md", "---\nname: [invalid YAML\n---\n")
	diagnostics, err = lint.Run(t.Context(), []string{path}, lint.Options{})
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Severity != "error" {
		t.Fatalf("malformed YAML: %+v (%v)", diagnostics, err)
	}
}

func TestTemplateDirectives(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "SKILL.md", "---\nname: example\ndescription: Example\n---\n\n{{if .enabled}}\n:::note{priority={{.priority}}}\n{{range .items}}- {{.}}{{end}}\n:::\n{{end}}\n")
	diagnostics, err := lint.Run(t.Context(), []string{path}, lint.Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("valid template: %+v (%v)", diagnostics, err)
	}
	write(t, root, "SKILL.md", "{{if .enabled}}\n:::note\n:::\n")
	diagnostics, err = lint.Run(t.Context(), []string{path}, lint.Options{Disable: []string{"metadata"}})
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Rule != "invalid-template" {
		t.Fatalf("invalid template: %+v (%v)", diagnostics, err)
	}
}

func TestHeadingReferences(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "guide.md", "# Current Heading\n\n[valid](#current-heading) [stale](#renamed-heading) [external](https://example.invalid/old)\n")
	diagnostics, err := lint.Run(t.Context(), []string{path}, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "broken-link" {
		t.Fatalf("expected one stale anchor: %+v", diagnostics)
	}
}
