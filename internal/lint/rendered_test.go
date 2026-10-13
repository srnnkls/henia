package lint_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/lint"
)

func splitByHarness(t *testing.T, found []lint.Diagnostic, reported, silent string) {
	t.Helper()
	var hit int
	for _, d := range found {
		switch d.Harness {
		case silent:
			t.Errorf("%s reported for %s: %+v", d.Rule, silent, d)
		case reported:
			hit++
		}
	}
	if hit == 0 {
		t.Errorf("diagnostics = %+v, want at least one naming harness %s", found, reported)
	}
}

func TestUnusedSlotIsCheckedPerHarness(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/context.md", "---\nform: leaf\nbody:\n  kind: none\n---\n:::when{target=\"claude\"}\n:slot[git.commits]\n:::\n")
	write(t, root, "skills/git/SKILL.md", skillFile("git", "henia:\n  slots:\n    git.commits:\n", "# Git\n\n::context\n"))
	options := lint.Options{Project: root, Configured: map[string]henia.Harness{"claude": {}, "codex": {}}}
	found := withRule(lintProject(t, options, filepath.Join(root, "skills")), "unused-slot")
	splitByHarness(t, found, "codex", "claude")
	for _, d := range found {
		if d.Stage != lint.StageRendered {
			t.Errorf("unused-slot stage = %q, want %q", d.Stage, lint.StageRendered)
		}
	}
}

func TestRenderedStageCoversOnlyConfiguredHarnessesAndFullHybridBodies(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/foo/SKILL.md", skillFile("foo", "", "# Foo\n\n:::when{flavor=on}\n## Part\n\nFoo part.\n:::\n"))
	write(t, root, "skills/bar/SKILL.md", skillFile("bar", "", "# Bar\n\n::include{ref=\"foo#part\"}\n"))
	write(t, root, "skills/scope/SKILL.md", skillFile("scope",
		"henia:\n  slots:\n    scope.templates:\n  provides:\n    scope.templates: {section: templates}\n",
		"# Scope\n\n:::head\nCreate and review scopes.\n:::\n\n:slot[scope.templates]\n\n## Templates\n\nScope templates.\n"))
	options := lint.Options{Project: root, Configured: map[string]henia.Harness{
		"codex": {Variables: map[string]string{"flavor": "on"}, Skills: henia.Artifacts{Hybrid: []string{"scope"}}},
	}}
	if found := withRule(lintProject(t, options, filepath.Join(root, "skills")), "missing-include", "missing-section", "unknown-variable"); len(found) != 0 {
		t.Fatalf("diagnostics = %+v, want none: only codex is built, it sets flavor, and #templates renders outside the hybrid head", found)
	}
}

func TestHybridHeadSizeIsCheckedPerHarness(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/big/SKILL.md", skillFile("big", "",
		"# Big\n\n::::head\nShared head line.\n\n:::when{target=\"claude\"}\nOne.\n\nTwo.\n\nThree.\n\nFour.\n\nFive.\n:::\n::::\n\n## Detail\n\nMore.\n"))
	hybrid := henia.Artifacts{Hybrid: []string{"big"}}
	options := lint.Options{
		Project:    root,
		Configured: map[string]henia.Harness{"claude": {Skills: hybrid}, "codex": {Skills: hybrid}},
		Config:     map[string]map[string]any{"large-head": {"max-head-lines": 4}},
	}
	found := withRule(lintProject(t, options, filepath.Join(root, "skills")), "large-head")
	splitByHarness(t, found, "claude", "codex")
}

func positions(found []lint.Diagnostic) []string {
	var got []string
	for _, d := range found {
		got = append(got, fmt.Sprintf("%s %s:%d:%d %s", d.Rule, filepath.Base(filepath.Dir(d.Path)), d.Line, d.Column, d.Harness))
	}
	return got
}

func TestRenderedPositionsFollowItemOrigins(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/dup/SKILL.md", skillFile("dup", "", "## Dup\n\n:::when{target=\"claude\"}\n## Kept\n\nText.\n:::\n\n## Dup\n"))
	options := lint.Options{Project: root, Configured: map[string]henia.Harness{"claude": {}}}
	got := positions(withRule(lintProject(t, options, filepath.Join(root, "skills")), "duplicate-heading"))
	if want := []string{"duplicate-heading dup:14:4 "}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("duplicate-heading = %q, want %q", got, want)
	}
}

func TestRenderedStageUsesHarnessRendering(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/tools/SKILL.md", skillFile("tools",
		"henia:\n  slots:\n    tools.fetch:\n  provides:\n    tools.fetch: {section: read-file}\n",
		"# Tools\n\n:slot[tools.fetch]\n\n## `!read`\n\nUse `!bash` here.\n\n## `read_file`\n"))
	options := lint.Options{
		Project:    root,
		Configured: map[string]henia.Harness{"codex": {Directives: "md", Tools: map[string]string{"read": "read_file", "bash": "OldBash"}}},
		Config:     map[string]map[string]any{"outdated-reference": {"outdated": map[string]any{"OldBash": "Bash"}}},
	}
	got := positions(withRule(lintProject(t, options, filepath.Join(root, "skills")), "duplicate-heading", "missing-section", "outdated-reference"))
	want := []string{"outdated-reference tools:17:6 ", "duplicate-heading tools:19:4 "}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("diagnostics = %q, want %q", got, want)
	}
}

func TestHybridHeadRendersWithHarnessDirectives(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/note.md", "---\nform: container\nbody:\n  kind: markdown\nrender: keep\n---\n")
	write(t, root, "skills/big/SKILL.md", skillFile("big", "", "# Big\n\n::::head\nOne.\n\n:::note\nTwo.\n:::\n::::\n\n## Detail\n\nMore.\n"))
	options := lint.Options{
		Project:    root,
		Configured: map[string]henia.Harness{"codex": {Directives: "md", Skills: henia.Artifacts{Hybrid: []string{"big"}}}},
		Config:     map[string]map[string]any{"large-head": {"max-head-lines": 4}},
	}
	if found := withRule(lintProject(t, options, filepath.Join(root, "skills")), "large-head"); len(found) != 0 {
		t.Fatalf("large-head = %+v, want none: with directives=md the rendered head has four lines", found)
	}
}

func TestRenderedReferencesComeFromRenderedBodies(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/guide.md", "---\nform: leaf\nbody:\n  kind: none\n---\nSee [guide](missing.md) and `$missing`.\n")
	write(t, root, ".henia/components/part.md", "---\nform: leaf\nbody:\n  kind: none\n---\n## Part\n\nFoo part.\n")
	write(t, root, "skills/foo/SKILL.md", skillFile("foo", "", "# Foo\n\n::part\n"))
	write(t, root, "skills/bar/SKILL.md", skillFile("bar", "", "# Bar\n\nRun `henia show foo#part`.\n\n::guide\n"))
	options := lint.Options{Project: root, Configured: map[string]henia.Harness{"claude": {}}}
	got := positions(withRule(lintProject(t, options, filepath.Join(root, "skills")), "broken-link", "missing-reference"))
	want := []string{"broken-link bar:10:1 ", "missing-reference bar:10:1 "}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("diagnostics = %q, want %q", got, want)
	}
}

func TestRenderedSkillReferencesFollowHarnessSyntax(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/other/SKILL.md", skillFile("other", "", "# Other\n"))
	write(t, root, "skills/user/SKILL.md", skillFile("user", "", "# User\n\nUse `$other`, then `$nope`, then `/help`.\n"))
	harness := func(skill string) henia.Harness { return henia.Harness{References: map[string]string{"skill": skill}} }
	options := lint.Options{
		Project: root,
		Builtin: []string{"command:help"},
		Configured: map[string]henia.Harness{
			"claude": harness("/{{.Name}}"),
			"pi":     harness("/skill:{{.Name}}"),
			"omp":    harness("/skill:{{.Name}}"),
			"codex":  harness("${{.Name}}"),
		},
	}
	harnesses := map[string]bool{}
	for _, d := range withRule(lintProject(t, options, filepath.Join(root, "skills")), "missing-reference") {
		if d.Message != `skill reference "nope" is absent from scanned artifacts` || d.Line != 8 || harnesses[d.Harness] {
			t.Errorf("missing-reference = %+v, want one skill nope report per rendered position", d)
		}
		harnesses[d.Harness] = true
	}
	if len(harnesses) == 0 {
		t.Fatal("missing-reference silent, want skill nope reported")
	}
}

func TestRenderedSkillSyntaxYieldsToResolvingCommand(t *testing.T) {
	root := t.TempDir()
	write(t, root, "commands/deploy.md", "---\ndescription: Deploy.\n---\nDeploy.\n")
	write(t, root, "skills/user/SKILL.md", skillFile("user", "", "# User\n\nRun `/deploy`.\n"))
	options := lint.Options{Project: root, Configured: map[string]henia.Harness{"claude": {References: map[string]string{"skill": "/{{.Name}}"}}}}
	if found := withRule(lintProject(t, options, root), "missing-reference"); len(found) != 0 {
		t.Fatalf("missing-reference = %+v, want none: /deploy resolves as a command", found)
	}
}
