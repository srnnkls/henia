package lint_test

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/pipeline"
)

func skillFile(name, frontmatter, body string) string {
	return "---\nname: " + name + "\ndescription: The " + name + " skill.\n" + frontmatter + "---\n\n" + body
}

func lintProject(t *testing.T, options lint.Options, paths ...string) []lint.Diagnostic {
	t.Helper()
	diagnostics, err := lint.Run(t.Context(), paths, options)
	if err != nil {
		t.Fatal(err)
	}
	return diagnostics
}

func withRule(diagnostics []lint.Diagnostic, rules ...string) []lint.Diagnostic {
	var out []lint.Diagnostic
	for _, d := range diagnostics {
		if slices.Contains(rules, d.Rule) {
			out = append(out, d)
		}
	}
	return out
}

func TestLintResolvesIncludesAsBuild(t *testing.T) {
	root, deps := t.TempDir(), t.TempDir()
	dep, one, two := filepath.Join(deps, "dep"), filepath.Join(deps, "one"), filepath.Join(deps, "two")
	write(t, root, "skills/a/SKILL.md", skillFile("a", "", "::include{ref=\"dep:a\"}\n"))
	write(t, dep, "skills/a/SKILL.md", skillFile("a", "", "Dep A.\n"))
	write(t, root, "skills/foo/SKILL.md", skillFile("foo", "", "# Foo\n\n## Part\n\nFoo part.\n"))
	write(t, root, "commands/foo.md", "---\ndescription: The foo command.\n---\n\nCommand.\n")
	write(t, root, "skills/bar/SKILL.md", skillFile("bar", "", "::include{ref=\"foo#part\"}\n"))
	write(t, one, "skills/x/SKILL.md", skillFile("x", "", "One.\n"))
	write(t, two, "skills/x/SKILL.md", skillFile("x", "", "Two.\n"))
	write(t, root, "skills/amb/SKILL.md", skillFile("amb", "", "::include{ref=\"x\"}\n"))
	options := lint.Options{Project: root, Packages: []pipeline.Dependency{{Root: dep}, {Root: one}, {Root: two}}}
	found := withRule(lintProject(t, options, filepath.Join(root, "skills"), filepath.Join(root, "commands")), "include-cycle", "missing-include")
	if len(found) != 1 || !strings.HasSuffix(found[0].Path, filepath.Join("amb", "SKILL.md")) || !strings.Contains(found[0].Message, "ambiguous") {
		t.Fatalf("include diagnostics = %+v, want only an ambiguous missing-include in amb", found)
	}
}

func TestLintExpandsDependenciesWithTheirHarnesses(t *testing.T) {
	root := t.TempDir()
	dep := filepath.Join(t.TempDir(), "dep")
	builtin, err := pipeline.Targets(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	own := map[string]henia.Harness{}
	for _, name := range append(slices.Sorted(maps.Keys(builtin)), "extra") {
		own[name] = henia.Harness{Variables: map[string]string{"flavor": "dep"}}
	}
	write(t, dep, "skills/d/SKILL.md", skillFile("d", "", ":::when{flavor=dep}\n## Part\n\nDep part.\n:::\n"))
	write(t, root, "skills/p/SKILL.md", skillFile("p", "", "::include{ref=\"dep:d#part\"}\n\n:::when{target=\"extra\"}\nx\n:::\n\n:::when{flavor=dep}\ny\n:::\n"))
	options := lint.Options{Project: root, Packages: []pipeline.Dependency{{Root: dep, Harnesses: own}}}
	if found := withRule(lintProject(t, options, filepath.Join(root, "skills")), "unknown-target", "unknown-variable", "missing-include"); len(found) != 0 {
		t.Fatalf("diagnostics = %+v, want none: dependency harnesses declare the target and the variable", found)
	}
}

func TestExpandedStageRulesSeeComponentOutput(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/title.md", "---\nform: leaf\nbody:\n  kind: none\n---\n# Generated\n")
	write(t, root, "skills/s/SKILL.md", skillFile("s", "", "Intro.\n\n::title\n"))
	modules := filepath.Join(root, ".henia", "lint")
	write(t, modules, "team/headings.md", "---\nstage: expanded\n---\n\n## any-heading\n\n```hq\n(rule any-heading :message \"heading\" (heading) @h)\n```\n")
	found := withRule(lintProject(t, lint.Options{Project: root, Modules: []lint.ModuleDir{{Dir: modules}}}, filepath.Join(root, "skills")), "any-heading")
	if len(found) != 1 || found[0].Line != 8 || found[0].Stage != lint.StageExpanded {
		t.Fatalf("any-heading = %+v, want one expanded finding at the ::title call on line 8", found)
	}
}

func TestSlotAttributesFollowSlotTypes(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/tags.md", tagsComponent)
	write(t, root, ".henia/components/verbatim.md", "---\nform: container\nbody:\n  kind: text\n---\nVerbatim.\n")
	write(t, root, "skills/git/SKILL.md", skillFile("git", "henia:\n  slots:\n    git.commits: list\n", "Git.\n"))
	write(t, root, "skills/nested/SKILL.md", skillFile("nested", "", "::tags{slots=\"git.commits.nope\"}\n"))
	write(t, root, "skills/forward/SKILL.md", skillFile("forward", "", "::include{component=\"tags\" slots=\"nope\"}\n"))
	write(t, root, "skills/literal/SKILL.md", skillFile("literal", "", ":::verbatim\n::tags{slots=\"nope\"}\n:::\n"))
	var got []string
	for _, d := range withRule(lintProject(t, lint.Options{Project: root}, filepath.Join(root, "skills")), "invalid-attribute") {
		got = append(got, filepath.Base(filepath.Dir(d.Path)))
	}
	if !slices.Equal(got, []string{"forward", "nested"}) {
		t.Fatalf("invalid-attribute in %q, want forward and nested", got)
	}
}

func TestDistinctDiagnosticsAtOnePositionSurvive(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/title.md", titleComponent)
	write(t, root, ".henia/components/twice.md", "---\nform: leaf\nbody:\n  kind: none\n---\n::include{component=\"title\" extra=1 nope=2}\n")
	write(t, root, "skills/gen/SKILL.md", skillFile("gen", "", "::twice\n"))
	var got []string
	for _, d := range withRule(lintProject(t, lint.Options{Project: root}, filepath.Join(root, "skills")), "unknown-attribute") {
		if strings.HasSuffix(d.Path, filepath.Join("gen", "SKILL.md")) {
			got = append(got, d.Message)
		}
	}
	if len(got) != 2 || !strings.Contains(strings.Join(got, " "), "extra") || !strings.Contains(strings.Join(got, " "), "nope") {
		t.Fatalf("unknown-attribute at the call = %q, want one for extra and one for nope", got)
	}
}

func TestIncludedDependencyErrorsSurface(t *testing.T) {
	root := t.TempDir()
	dep := filepath.Join(t.TempDir(), "dep")
	write(t, root, "skills/a/SKILL.md", skillFile("a", "", "::include{ref=\"dep:b\"}\n"))
	write(t, dep, "skills/b/SKILL.md", skillFile("b", "", "::gone\n"))
	found := withRule(lintProject(t, lint.Options{Project: root, Packages: []pipeline.Dependency{{Root: dep}}}, filepath.Join(root, "skills")), "unknown-component")
	if len(found) != 1 || !strings.HasSuffix(found[0].Path, filepath.Join("b", "SKILL.md")) ||
		!slices.ContainsFunc(found[0].Notes, func(n string) bool { return strings.Contains(n, "included from") }) {
		t.Fatalf("unknown-component = %+v, want the dependency's error with an included-from note", found)
	}
}

func TestKnownValuesArePackageWide(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/components/show.md", "---\nform: leaf\nbody:\n  kind: none\n---\n{{.extra}}\n")
	write(t, root, "skills/sibling/SKILL.md", skillFile("sibling", "extra: x\n", "Sibling.\n"))
	one := write(t, root, "skills/one/SKILL.md", skillFile("one", "", "::show\n"))
	if found := withRule(lintProject(t, lint.Options{Project: root}, one), "unknown-field", "invalid-template"); len(found) != 0 {
		t.Fatalf("diagnostics = %+v, want none: a sibling skill declares extra", found)
	}
}
