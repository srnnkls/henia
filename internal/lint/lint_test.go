package lint_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{Config: map[string]map[string]any{"large-skill": {"max-lines": 4}, "outdated-reference": {"outdated": map[string]any{"legacy-model": "current-model"}}}, Now: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
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

func TestExternalReferencesResolve(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/one/SKILL.md", "---\nname: one\ndescription: First skill\n---\n\nUse `$vendored`, `/plan` and `$absent`.\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{Config: map[string]map[string]any{"missing-reference": {"external": []any{"skill:vendored", "command:plan"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "missing-reference" || !strings.Contains(diagnostics[0].Message, `"absent"`) {
		t.Fatalf("diagnostics = %+v, want one missing-reference for absent", diagnostics)
	}
}

func TestReferencesResolveAndExamplesAreIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/one/SKILL.md", "---\nname: one\ndescription: First skill\n---\n\n:::note\nSee `$two`, [Guide](reference/guide.md), `#reference/guide.md`.\n:::\n\n```md\n:x[bad\n[broken](absent.md) `$absent` legacy-model\n```\n")
	write(t, root, "skills/one/reference/guide.md", "# Guide\n\nWorking guide.\n")
	write(t, root, "skills/two/SKILL.md", "---\nname: two\ndescription: Second skill\n---\n\nA second skill.\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{Config: map[string]map[string]any{"outdated-reference": {"outdated": map[string]any{"legacy-model": "current-model"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestEmptyHeadingsAreIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/one/SKILL.md", "---\nname: one\ndescription: First skill\n---\n\n###\n\nBody.\n\n### \n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestGitIgnoredPathsAreSkipped(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	write(t, root, ".gitignore", "vendored/\nnotes.md\n")
	write(t, root, "skills/one/SKILL.md", "---\nname: one\ndescription: First skill\n---\n\nBody.\n")
	write(t, root, "vendored/guide/README.md", "[broken](absent.md)\n")
	write(t, root, "notes.md", "[broken](absent.md)\n")
	for _, path := range []string{root, filepath.Join(root, "skills")} {
		diagnostics, err := lint.Run(t.Context(), []string{path}, lint.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(diagnostics) != 0 {
			t.Fatalf("lint %s: unexpected diagnostics: %+v", path, diagnostics)
		}
	}
	for _, path := range []string{filepath.Join(root, "vendored"), filepath.Join(root, "notes.md")} {
		diagnostics, err := lint.Run(t.Context(), []string{path}, lint.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(diagnostics) != 1 {
			t.Fatalf("lint %s: explicit ignored path not linted: %+v", path, diagnostics)
		}
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

func TestLinkedLeavesAreLinted(t *testing.T) {
	root := t.TempDir()
	target := write(t, root, "checkout/SKILL.md", "---\nname: one\n---\n\n:::broken\n")
	source := filepath.Join(root, "source/skills/one")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := lint.Run(t.Context(), []string{filepath.Join(root, "source")}, lint.Options{Disable: []string{"metadata"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Severity != "error" {
		t.Fatalf("linked leaf not linted: %+v", diagnostics)
	}
}

func TestShownSectionLinks(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/guide/SKILL.md", "---\nname: guide\ndescription: Guide skill\n---\n\n# Guide\n\n## Go\n\nText.\n")
	write(t, root, "skills/guide/ref.md", "# Ref\n\n## Errors\n")
	write(t, root, "skills/user/SKILL.md", "---\nname: user\ndescription: User skill\n---\n\n# User\n\nRead `henia show guide#go`, `henia show guide#rust`, `henia show tropos:guide/ref.md#errors`,\n`henia show guide/ref.md#panics`, `henia show guide/missing.md` and `henia show elsewhere#x`.\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diagnostics {
		if d.Rule == "broken-link" {
			got = append(got, fmt.Sprintf("%d:%s", d.Line, d.Message))
		}
	}
	want := []string{
		"8:Markdown heading anchor does not exist: henia show guide#rust",
		"9:Markdown heading anchor does not exist: henia show guide/ref.md#panics",
		"9:local reference does not exist: henia show guide/missing.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
