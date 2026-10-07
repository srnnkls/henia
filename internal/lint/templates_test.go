package lint_test

import (
	"context"
	"slices"
	"testing"

	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/templates"
)

func TestTemplateReferencesAndUse(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".henia/templates/head.md.tmpl", "Context.\n")
	write(t, root, ".henia/templates/skill.md.tmpl", "{{block \"lead\" .}}{{end}}\n")
	write(t, root, ".henia/templates/stale.md.tmpl", "Old.\n")
	user := t.TempDir()
	write(t, user, "templates/global.md.tmpl", "Unused but global.\n")
	write(t, root, "skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Ship.\nhenia:\n  layout: skill\n---\n{{define \"lead\"}}{{template \"head\" .}}{{template \"gone\" .}}{{end}}\n")

	diagnostics, err := lint.Run(context.Background(), []string{root + "/skills"}, lint.Options{Templates: templates.Files(root, user)})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diagnostics {
		if d.Rule == "unknown-template" || d.Rule == "unused-template" {
			got = append(got, d.Rule+" "+d.Message)
		}
	}
	want := []string{
		"unused-template template stale is not used by any scanned skill or template",
		`unknown-template template "gone" is neither a shared template nor defined in this file`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
