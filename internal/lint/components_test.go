package lint_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/lint"
)

const (
	noteComponent  = "---\nform: container\nattributes:\n  title:\n    type: string\n    required: true\nbody:\n  kind: markdown\n---\n{{.title}}\n\n::body\n"
	tagsComponent  = "---\nform: leaf\nattributes:\n  slots:\n    type: list\n    of: slot\nbody:\n  kind: none\n---\n{{.slots}}\n"
	rowsComponent  = "---\nform: leaf\nattributes:\n  items:\n    type: list\nbody:\n  kind: none\n---\n{{range .items}}{{.label}}{{end}}\n{{$.nope}}\n"
	titleComponent = "---\nform: leaf\nattributes:\n  title:\n    type: string\nbody:\n  kind: none\n---\n# {{.title}}\n"
	badComponent   = "---\nform: leaf\nbody:\n  kind: none\n---\n::include{template=\"title\" title=\"X\" nope=1}\n"
)

func componentProject(t *testing.T) (string, []lint.Diagnostic) {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{"note": noteComponent, "tags": tagsComponent, "rows": rowsComponent, "title": titleComponent, "bad": badComponent} {
		write(t, root, ".henia/components/"+name+".md", content)
	}
	skill := func(name, frontmatter, body string) {
		write(t, root, "skills/"+name+"/SKILL.md", "---\nname: "+name+"\ndescription: The "+name+" skill.\n"+frontmatter+"---\n\n"+body)
	}
	skill("dup", "", "intro\n\n:x[a]{k=1 k=2}\n")
	skill("note", "", ":::note{title=a bogus=1}\ntext\n:::\n")
	skill("git", "henia:\n  slots:\n    git.commits: list\n", "Git.\n")
	skill("tagged", "", "::tags{slots=\"git.commits nope\"}\n")
	skill("target", "", ":::when{target=\"clod\"}\nx\n:::\n")
	skill("variable", "", ":::when{nope=true}\nx\n:::\n")
	skill("gen", "", "intro\n\n::bad\n")
	diagnostics, err := lint.Run(t.Context(), []string{filepath.Join(root, "skills")}, lint.Options{Project: root})
	if err != nil {
		t.Fatal(err)
	}
	return root, diagnostics
}

func placed(root string, d lint.Diagnostic) string {
	base := filepath.Base(root) + string(filepath.Separator)
	rel := d.Path[strings.LastIndex(d.Path, base)+len(base):]
	return fmt.Sprintf("%s %s %s:%d:%d", d.Rule, d.Stage, filepath.ToSlash(rel), d.Line, d.Column)
}

func ruled(root string, diagnostics []lint.Diagnostic, rule, file string) []string {
	var got []string
	for _, d := range diagnostics {
		if d.Rule == rule && strings.HasSuffix(filepath.ToSlash(d.Path), file) {
			got = append(got, placed(root, d))
		}
	}
	return got
}

func TestSourceStageComponentDiagnostics(t *testing.T) {
	root, diagnostics := componentProject(t)
	for _, c := range []struct{ rule, file, want string }{
		{"duplicate-attribute", "skills/dup/SKILL.md", "duplicate-attribute source skills/dup/SKILL.md:8:11"},
		{"unknown-field", ".henia/components/rows.md", "unknown-field source .henia/components/rows.md:10:3"},
		{"invalid-attribute", "skills/tagged/SKILL.md", "invalid-attribute source skills/tagged/SKILL.md:6:1"},
	} {
		if got := ruled(root, diagnostics, c.rule, c.file); !slices.Equal(got, []string{c.want}) {
			t.Errorf("%s = %q, want [%q]", c.rule, got, c.want)
		}
	}
	for _, d := range diagnostics {
		if d.Rule == "invalid-attribute" && (!strings.Contains(d.Message, "nope") || strings.Contains(d.Message, "git.commits")) {
			t.Errorf("invalid-attribute message %q must name nope and not the declared git.commits", d.Message)
		}
	}
	if got := ruled(root, diagnostics, "unknown-attribute", "skills/note/SKILL.md"); len(got) != 1 || !strings.HasSuffix(got[0], "skills/note/SKILL.md:6:1") {
		t.Errorf("authored call unknown-attribute = %q, want it once at skills/note/SKILL.md:6:1", got)
	}
}

func TestExpandedStageComponentDiagnostics(t *testing.T) {
	root, diagnostics := componentProject(t)
	for _, c := range []struct{ rule, file, want string }{
		{"unknown-target", "skills/target/SKILL.md", "unknown-target expanded skills/target/SKILL.md:6:1"},
		{"unknown-variable", "skills/variable/SKILL.md", "unknown-variable expanded skills/variable/SKILL.md:6:1"},
		{"unknown-attribute", "skills/gen/SKILL.md", "unknown-attribute expanded skills/gen/SKILL.md:8:1"},
	} {
		got := ruled(root, diagnostics, c.rule, c.file)
		if len(got) == 0 || slices.ContainsFunc(got, func(s string) bool { return s != c.want }) {
			t.Errorf("%s = %q, want only %q", c.rule, got, c.want)
		}
	}
	note := filepath.ToSlash(filepath.Join(".henia", "components", "bad.md")) + ":6"
	for _, d := range diagnostics {
		if d.Rule == "unknown-attribute" && strings.HasSuffix(filepath.ToSlash(d.Path), "skills/gen/SKILL.md") &&
			!slices.ContainsFunc(d.Notes, func(n string) bool { return strings.Contains(filepath.ToSlash(n), note) }) {
			t.Errorf("generated call notes = %q, want one naming %s", d.Notes, note)
		}
	}
}
