package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomDirectiveRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	source := "---\nname: example\ndescription: Example\n---\n\n:::instruction{priority=low}\nText.\n:::instruction{priority=high}\nNested.\n:::\n:::\n\n:instruction[missing]\n\n```md\n:::instruction\n:::\n```\n\n:::instruction{priority={{.priority}}}\nDynamic.\n:::\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Rules: []Rule{{ID: "instruction-priority", Select: "directive", When: `node.name == "instruction"`, Assert: `node.attrs.priority in ["normal", "high", "critical"]`, Message: "Invalid priority"}}}
	diagnostics, err := Run(t.Context(), []string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 || diagnostics[0].Line != 6 || diagnostics[1].Line != 13 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
	for _, d := range diagnostics {
		if d.Rule != "instruction-priority" || d.Message != "Invalid priority" || d.Severity != "warning" {
			t.Fatalf("unexpected diagnostic: %+v", d)
		}
	}
	opts.Disable = []string{"instruction-priority"}
	diagnostics, err = Run(t.Context(), []string{path}, opts)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("disabled rule: %+v, %v", diagnostics, err)
	}
}

func TestCustomRuleSelectors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: example\ndescription: Short\n---\n\n# Heading\n\n[link](https://example.test)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Rules: []Rule{
		{ID: "description-length", Select: "frontmatter", When: `node.name == "description"`, Assert: `len(node.value) >= 20`, Message: "Describe when to use this skill", Severity: "error"},
		{ID: "heading-level", Select: "heading", Assert: `node.level == 2`, Message: "Use level 2"},
		{ID: "approved-host", Select: "link", Assert: `node.destination startsWith "https://docs.example.test"`, Message: "Use approved documentation"},
		{ID: "requires-license", Select: "document", Assert: `"license" in frontmatter`, Message: "Add license"},
	}}
	diagnostics, err := Run(t.Context(), []string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 4 {
		t.Fatalf("%+v", diagnostics)
	}
	for i, line := range []int{1, 3, 6, 8} {
		if diagnostics[i].Line != line {
			t.Fatalf("%+v", diagnostics)
		}
	}
}

func TestInvalidCustomRules(t *testing.T) {
	base := Rule{ID: "test-rule", Select: "directive", Assert: "true", Message: "Test"}
	for _, edit := range []func(*Rule){
		func(r *Rule) { r.Assert = "node.no_such_field == 1" },
		func(r *Rule) { r.Assert = "42" },
		func(r *Rule) { r.When = "unknown" },
		func(r *Rule) { r.Select = "xpath" },
		func(r *Rule) { r.Severity = "fatal" },
		func(r *Rule) { r.ID = "metadata" },
	} {
		rule := base
		edit(&rule)
		if err := (Options{Rules: []Rule{rule}}).Validate(); err == nil {
			t.Fatalf("accepted %+v", rule)
		}
	}
	if err := (Options{Rules: []Rule{base, base}}).Validate(); err == nil {
		t.Fatal("accepted duplicate rule")
	}
}

func TestRuleRuntimeErrorsAreErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guide.md")
	if err := os.WriteFile(path, []byte("# Guide\n"), 0644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Rules: []Rule{{ID: "bad-runtime", Select: "document", Assert: `int(frontmatter.missing) > 0`, Message: "ordinary failure"}}}
	diagnostics, err := Run(t.Context(), []string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Severity != "error" || !strings.Contains(diagnostics[0].Message, "evaluation failed") {
		t.Fatalf("%+v", diagnostics)
	}
}
