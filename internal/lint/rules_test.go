package lint

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStdlibExamples(t *testing.T) {
	failures, count, err := TestModules(nil, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("the standard library has no examples")
	}
	for _, f := range failures {
		t.Error(f)
	}
}

func TestInlineRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	source := "---\nname: example\ndescription: Example\n---\n\n:::instruction{priority=low}\nText.\n:::instruction{priority=high}\nNested.\n:::\n:::\n\n:instruction[missing]\n\n```md\n:::instruction\n:::\n```\n\n:::instruction{priority={{.priority}}}\nDynamic.\n:::\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	rule := InlineRule{ID: "instruction-priority", Message: "Invalid priority {@d.priority}", Query: `(directive :name "instruction" :priority (not /^(normal|high|critical)$/) :dynamic (not "true")) @d`}
	opts := Options{Rules: []InlineRule{rule}, Disable: []string{"metadata"}}
	diagnostics, err := Run(t.Context(), []string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diagnostics {
		got = append(got, d.Rule+":"+d.Message+":"+d.Severity+":"+filepath.Base(d.Path)+":"+strconv.Itoa(d.Line))
	}
	want := "instruction-priority:Invalid priority low:warning:SKILL.md:6 instruction-priority:Invalid priority :warning:SKILL.md:13"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
	opts.Disable = append(opts.Disable, "instruction-priority")
	if diagnostics, err := Run(t.Context(), []string{path}, opts); err != nil || len(diagnostics) != 0 {
		t.Fatalf("disabled rule: %+v, %v", diagnostics, err)
	}
	opts.Disable = []string{"metadata"}
	opts.Config = map[string]map[string]any{"instruction-priority": {"severity": "error"}}
	if diagnostics, err := Run(t.Context(), []string{path}, opts); err != nil || diagnostics[0].Severity != "error" {
		t.Fatalf("severity override: %+v, %v", diagnostics, err)
	}
}

func TestModulesCompose(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills", "guide", "SKILL.md")
	writeFile(t, root, "skills/guide/SKILL.md", "---\nname: guide\ndescription: A guide skill for tests.\n---\n\n# Guide\n\n## Usage\n\nText.\n\n## Usage\n\nMore.\n")
	modules := filepath.Join(root, ".henia", "lint")
	writeFile(t, modules, "team/headings.md", "---\ndata:\n  banned: [usage]\n---\n\n## repeated-usage\n\n```hq\n(import std/structure)\n(rule repeated-usage\n  :message \"{@h.text} repeats; first on line {@first.line}\"\n  :at @h\n  (repeated-heading @first @h ?text)\n  (row :table banned :value ?text))\n```\n")
	diagnostics, err := Run(t.Context(), []string{skill}, Options{Modules: []string{modules}, Disable: []string{"duplicate-heading"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "repeated-usage" || diagnostics[0].Message != "Usage repeats; first on line 8" || diagnostics[0].Line != 12 {
		t.Fatalf("%+v", diagnostics)
	}
	writeFile(t, modules, "team/clash.md", "## duplicate-heading\n\n```hq\n(rule duplicate-heading :message \"m\" (heading) @h)\n```\n")
	if _, err := Run(t.Context(), []string{skill}, Options{Modules: []string{modules}}); err == nil || !strings.Contains(err.Error(), "lint rule duplicate-heading is defined in std/structure and team/clash") {
		t.Fatalf("shadowing a stdlib rule: %v", err)
	}
}

func TestInvalidRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("# Guide\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		options Options
		message string
	}{
		{Options{Rules: []InlineRule{{ID: "bad", Message: "m", Query: "(headng) @h"}}}, `unknown type "headng"`},
		{Options{Rules: []InlineRule{{ID: "Bad", Message: "m", Query: "(heading) @h"}}}, "lowercase letters"},
		{Options{Rules: []InlineRule{{ID: "x", Message: "{@y}", Query: "(heading) @h"}}}, "uses @y"},
		{Options{Rules: []InlineRule{{ID: "x", Message: "m", Severity: "fatal", Query: "(heading) @h"}}}, "warning or error"},
		{Options{Rules: []InlineRule{{ID: "large-skill", Message: "m", Query: "(heading) @h"}}}, "is defined in std/structure and henia.toml"},
		{Options{Disable: []string{"no-such-rule"}}, `unknown lint rule "no-such-rule"`},
		{Options{Config: map[string]map[string]any{"large-skill": {"severity": "fatal"}}}, "severity must be warning or error"},
	} {
		if _, err := Run(t.Context(), []string{path}, tc.options); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Errorf("%+v: err = %v, want %q", tc.options, err, tc.message)
		}
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
