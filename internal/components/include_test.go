package components

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

const (
	includingPath = "/pkg/skills/a/SKILL.md"
	includedPath  = "/pkg/skills/b/SKILL.md"
)

const includedSkill = `# B

## Usage

:::when{target="codex"}
codex only
:::

:::when{target="claude"}
claude only
:::

::sig

## Other

other text
`

func units(t *testing.T, decls map[string]Declaration, sources map[string]string) ([]Unit, map[string]*markup.Document) {
	t.Helper()
	docs := map[string]*markup.Document{}
	var out []Unit
	for name, path := range map[string]string{"a": includingPath, "b": includedPath} {
		source, ok := sources[name]
		if !ok {
			continue
		}
		doc, err := markup.Parse(path, []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		docs[name] = doc
		out = append(out, Unit{Name: name, Package: "pkg", Document: doc, Components: decls})
	}
	return out, docs
}

func TestExpandDocumentsGraftsIncludedSection(t *testing.T) {
	decls, _ := declare(t, map[string]string{"sig": "---\nform: leaf\nbody:\n  kind: none\n---\nsigned\n"})
	list, docs := units(t, decls, map[string]string{
		"a": "# A\n\nintro\n\n::include{ref=\"b#usage\"}\n\ntail\n",
		"b": includedSkill,
	})
	if diags := ExpandDocuments(list, environment("codex")); len(diags) > 0 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	got := string(markup.Write(docs["a"]))
	for _, want := range []string{"# A\n", "intro\n", "## Usage\n", "codex only\n", "signed\n", "tail\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"claude only", "# B", "## Other", "other text", "::include", ":::when", "::sig"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output holds %q:\n%s", unwanted, got)
		}
	}
	if !(strings.Index(got, "intro") < strings.Index(got, "## Usage") && strings.Index(got, "signed") < strings.Index(got, "tail")) {
		t.Errorf("included section is not at the call site:\n%s", got)
	}
}

func TestExpandDocumentsReportsIncludeCycle(t *testing.T) {
	decls, _ := declare(t, map[string]string{"sig": "---\nform: leaf\nbody:\n  kind: none\n---\nsigned\n"})
	list, _ := units(t, decls, map[string]string{
		"a": "# A\n\nintro\n\n::include{ref=\"b#usage\"}\n",
		"b": "# B\n\n## Usage\n\n::include{ref=\"a\"}\n",
	})
	diags := ExpandDocuments(list, environment("codex"))
	var sites []string
	for _, d := range diags {
		if d.ID != "include-cycle" {
			continue
		}
		if d.Level != LevelError {
			t.Errorf("include-cycle level = %s, want error", d.Level)
		}
		sites = append(sites, fmt.Sprintf("%s:%d:%d", d.Path, d.Line, d.Column))
		sites = append(sites, d.Notes...)
	}
	for _, call := range []string{includingPath + ":5", includedPath + ":5"} {
		if !slices.ContainsFunc(sites, func(s string) bool { return strings.Contains(s, call) }) {
			t.Errorf("include-cycle reports %q, want call site %s among them", sites, call)
		}
	}
}

func TestExpandDocumentsIncludeDiagnostics(t *testing.T) {
	decls, _ := declare(t, map[string]string{"title": titleComponent, "sig": "---\nform: leaf\nbody:\n  kind: none\n---\nsigned\n"})
	cases := []struct{ name, call, want string }{
		{"unknown skill", "::include{ref=\"nope#usage\"}", "missing-include error " + includingPath + ":3:1"},
		{"unknown section anchor", "::include{ref=\"b#nope\"}", "missing-include error " + includingPath + ":3:1"},
		{"ref and component", "::include{ref=\"b#usage\" component=\"title\" title=\"X\"}", "invalid-include error " + includingPath + ":3:1"},
		{"neither ref nor component", "::include", "invalid-include error " + includingPath + ":3:1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			list, _ := units(t, decls, map[string]string{"a": "intro\n\n" + c.call + "\n", "b": includedSkill})
			got := located(ExpandDocuments(list, environment("codex")), func(Diagnostic) bool { return true })
			if !slices.Contains(got, c.want) {
				t.Fatalf("diagnostics = %q, want %q among them", got, c.want)
			}
		})
	}
}

func includeOf(t *testing.T, decls map[string]Declaration, b string) string {
	t.Helper()
	list, docs := units(t, decls, map[string]string{"a": "# A\n\n::include{ref=\"b#usage\"}\n", "b": b})
	if diags := ExpandDocuments(list, environment("codex")); len(diags) > 0 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	return string(markup.Write(docs["a"]))
}

func TestExpandDocumentsIncludeAnchorAfterDirectiveAndRule(t *testing.T) {
	decls, _ := declare(t, map[string]string{"note": noteComponent})
	got := includeOf(t, decls, "# B\n\n:::note{title=\"T\"}\nDo X.\n:::\n---\n\n## Usage\n\nusage text\n\n## Other\n\nother text\n")
	if !strings.Contains(got, "## Usage\n") || !strings.Contains(got, "usage text") {
		t.Errorf("include lacks the Usage section:\n%s", got)
	}
	for _, unwanted := range []string{"## Other", "other text", "Do X."} {
		if strings.Contains(got, unwanted) {
			t.Errorf("include holds %q:\n%s", unwanted, got)
		}
	}
}

func TestExpandDocumentsIncludeSectionSpansExpandedWrapper(t *testing.T) {
	decls, _ := declare(t, nil)
	got := includeOf(t, decls, "# B\n\n:::when{target=\"codex\"}\n## Usage\ninside\n:::\n\noutside\n\n## Next\n\nnext text\n")
	for _, want := range []string{"## Usage\n", "inside", "outside"} {
		if !strings.Contains(got, want) {
			t.Errorf("include lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"## Next", "next text", ":::when"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("include holds %q:\n%s", unwanted, got)
		}
	}
}

func TestExpandDocumentsExpandsEachUnitWithItsOwnValues(t *testing.T) {
	decls, _ := declare(t, map[string]string{"who": "---\nform: leaf\nbody:\n  kind: none\n---\nname is {{.name}}\n"})
	list, docs := units(t, decls, map[string]string{
		"a": "# A\n\n::who\n\n::include{ref=\"b#usage\"}\n",
		"b": "# B\n\n## Usage\n\n::who\n",
	})
	for i := range list {
		list[i].Values = map[string]any{"name": list[i].Name}
	}
	if diags := ExpandDocuments(list, environment("codex")); len(diags) > 0 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	got := string(markup.Write(docs["a"]))
	if want := "# A\n\nname is a\n\n## Usage\n\nname is b\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func dependencyUnits(t *testing.T, a string) ([]Unit, *markup.Document) {
	t.Helper()
	including, err := markup.Parse(includingPath, []byte(a))
	if err != nil {
		t.Fatal(err)
	}
	git, err := markup.Parse("/dep/skills/git/SKILL.md", []byte("# Git\n\n## Commits\n\ncommit text\n"))
	if err != nil {
		t.Fatal(err)
	}
	decls, _ := declare(t, nil)
	return []Unit{{Name: "a", Package: "pkg", Document: including, Components: decls}, {Name: "git", Package: "dep", Document: git, Components: decls}}, including
}

func TestExpandDocumentsIncludesUniqueCrossPackageSkill(t *testing.T) {
	list, doc := dependencyUnits(t, "# A\n\n::include{ref=\"git#commits\"}\n")
	if diags := ExpandDocuments(list, environment("codex")); len(diags) > 0 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	if got, want := string(markup.Write(doc)), "# A\n\n## Commits\n\ncommit text\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestExpandDocumentsIncludesThroughSuppliedResolver(t *testing.T) {
	list, doc := dependencyUnits(t, "# A\n\n::include{ref=\"commits-of-git\"}\n")
	env := environment("codex")
	var asked []string
	env.Resolve = func(from Unit, ref string) (Key, string, error) {
		asked = append(asked, from.Name+" "+ref)
		if ref != "commits-of-git" {
			return Key{}, "", fmt.Errorf("unknown %s", ref)
		}
		return Key{Package: "dep", Name: "git"}, "commits", nil
	}
	if diags := ExpandDocuments(list, env); len(diags) > 0 {
		t.Fatalf("diagnostics = %+v", diags)
	}
	if got, want := string(markup.Write(doc)), "# A\n\n## Commits\n\ncommit text\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if want := []string{"a commits-of-git"}; !slices.Equal(asked, want) {
		t.Errorf("resolver calls = %q, want %q", asked, want)
	}
}
