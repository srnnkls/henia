package lint_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/srnnkls/henia/internal/lint"
)

func linkedLibrary(t *testing.T) (root, a, b, c string) {
	t.Helper()
	root = t.TempDir()
	a = write(t, root, "skills/a/SKILL.md", "---\nname: a\ndescription: Skill a\n---\n\n# A\n\nRead `henia show b`.\n")
	b = write(t, root, "skills/b/SKILL.md", "---\nname: b\ndescription: Skill b\n---\n\n# B\n\nRead `henia show c`.\n")
	c = write(t, root, "skills/c/SKILL.md", "---\nname: c\ndescription: Skill c\n---\n\n# C\n\nText.\n")
	return root, a, b, c
}

func ruleDiagnostics(t *testing.T, root string, options lint.Options, rule string) []string {
	t.Helper()
	diagnostics, err := lint.Run(t.Context(), []string{filepath.Join(root, "skills")}, options)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diagnostics {
		if d.Rule == rule {
			got = append(got, fmt.Sprintf("%s:%d:%s", d.Path, d.Line, d.Message))
		}
	}
	return got
}

func TestGroupedModuleRuleReportsUnreferencedSkill(t *testing.T) {
	root, a, _, _ := linkedLibrary(t)
	modules := filepath.Join(root, ".henia", "lint")
	write(t, modules, "team/inbound.md", "---\nparams:\n  min-inbound: 1\n---\n\n## unreferenced-skill\n\n```hq\n(rule unreferenced-skill\n  :message \"{@t.id} has {?n} inbound links; under {$min-inbound}\"\n  :at @t\n  (skill :id ?s) @t\n  (skill :id (not ?s) (link :target ?s) @l)?\n  (group @t (count @l ?n (< $min-inbound))))\n```\n")
	got := ruleDiagnostics(t, root, lint.Options{Modules: []lint.ModuleDir{{Dir: modules}}}, "unreferenced-skill")
	want := []string{a + ":1:a has 0 inbound links; under 1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestGroupedRuleLocatesCollectedCaptureWithConfiguredThreshold(t *testing.T) {
	root, a, b, _ := linkedLibrary(t)
	modules := filepath.Join(root, ".henia", "lint")
	write(t, modules, "team/inbound.md", "---\nparams:\n  min-inbound: 1\n---\n\n## weakly-referenced-skill\n\n```hq\n(rule weakly-referenced-skill\n  :message \"{@t.id} has {?n} inbound links; under {$min-inbound}\"\n  :at @l\n  (skill :id ?s) @t\n  (skill :id (not ?s) (link :target ?s) @l)\n  (group @t (count @l ?n (< $min-inbound))))\n```\n")
	options := lint.Options{Modules: []lint.ModuleDir{{Dir: modules}}}
	if got := ruleDiagnostics(t, root, options, "weakly-referenced-skill"); len(got) != 0 {
		t.Fatalf("module default min-inbound 1 reported %q", got)
	}
	options.Config = map[string]map[string]any{"weakly-referenced-skill": {"min-inbound": 2}}
	got := ruleDiagnostics(t, root, options, "weakly-referenced-skill")
	want := []string{
		a + ":8:b has 1 inbound links; under 2",
		b + ":8:c has 1 inbound links; under 2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
