package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/lint"
)

func fixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func testConfig(t *testing.T, content string) {
	t.Helper()
	previous := configPath
	t.Cleanup(func() { configPath = previous })
	configPath = filepath.Join(t.TempDir(), "henia.toml")
	fixture(t, configPath, content)
}

func TestBuildMultipleHarnesses(t *testing.T) {
	root := t.TempDir()
	fixture(t, filepath.Join(root, "skills/example/SKILL.md"), "---\nname: example\ndescription: Example\npriority: critical\n---\n\n:::instruction{priority={{.priority}}}\nUse `!read`.\n:::\n")
	fixture(t, filepath.Join(root, "skills/example/reference/guide.md"), "# Guide\n")
	testConfig(t, "[harness.claude]\nformat = 'xml'\n[harness.claude.tools]\nread = 'Read'\n[harness.codex]\nformat = 'directives'\n")
	output := filepath.Join(t.TempDir(), "out")
	cmd := newBuildCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{root, "--output", output})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"claude": "<instruction priority=\"critical\">\nUse `Read`.", "codex": ":::instruction{priority=\"critical\"}\nUse `!read`."} {
		data, err := os.ReadFile(filepath.Join(output, name, "skills/example/SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s output: %s", name, data)
		}
		if _, err := os.Stat(filepath.Join(output, name, "skills/example/reference/guide.md")); err != nil {
			t.Fatal(err)
		}
	}
	// Rebuilding produces exactly the same bytes.
	first, err := os.ReadFile(filepath.Join(output, "claude/skills/example/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(output, "claude/skills/example/SKILL.md"))
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("rebuild differs: %v", err)
	}
}

func TestBuildPreflightAndSelection(t *testing.T) {
	root := t.TempDir()
	fixture(t, filepath.Join(root, "skills/a.md"), "# Valid\n")
	fixture(t, filepath.Join(root, "skills/z.md"), ":::broken\n")
	testConfig(t, "[harness.claude]\nformat = 'xml'\n")
	output := filepath.Join(t.TempDir(), "out")
	cmd := newBuildCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{root, "--output", output})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected malformed directive to fail build")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed preflight wrote output")
	}
	cmd.SetArgs([]string{root, "--harness", "unknown"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown harness error")
	}
}

func TestLintJSONAndExitStatus(t *testing.T) {
	root := t.TempDir()
	fixture(t, filepath.Join(root, "SKILL.md"), "---\nname: example\ndescription: Example\n---\n\n[Guide](missing.md)\n")
	testConfig(t, "")
	for _, strict := range []bool{false, true} {
		cmd := newLintCommand()
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SilenceUsage = true
		args := []string{root, "--format", "json"}
		if strict {
			args = append(args, "--strict")
		}
		cmd.SetArgs(args)
		err := cmd.Execute()
		if (err != nil) != strict {
			t.Fatalf("strict=%v: %v", strict, err)
		}
		var diagnostics []lint.Diagnostic
		if err := json.Unmarshal(out.Bytes(), &diagnostics); err != nil {
			t.Fatalf("invalid JSON: %s (%v)", out, err)
		}
		if len(diagnostics) != 1 || diagnostics[0].Rule != "broken-link" {
			t.Fatalf("unexpected diagnostics: %+v", diagnostics)
		}
	}
}

func TestBuildProfilesWithTemplatesAndSidecars(t *testing.T) {
	root := t.TempDir()
	fixture(t, filepath.Join(root, "skills/example/SKILL.md"), `---
name: example
description: Example skill
henia:
  auto_invoke: false
  variables:
    priority: high
  targets:
    codex:
      openai:
        interface:
          display_name: "{{.title}}"
---

{{define "instructions"}}:::instruction{priority={{.priority}}}
Use `+"`$example`"+`.
:::
{{end}}{{template "instructions" .}}`)
	testConfig(t, `[harness.claude]
profile = "claude"
strict = true
[harness.codex]
profile = "codex"
strict = true
[harness.codex.variables]
title = "Example display"
`)
	output := filepath.Join(t.TempDir(), "out")
	cmd := newBuildCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{root, "--output", output})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	claude, err := os.ReadFile(filepath.Join(output, "claude/skills/example/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claude), "disable-model-invocation: true") || !strings.Contains(string(claude), "<instruction priority=\"high\">") {
		t.Fatalf("%s", claude)
	}
	sidecar, err := os.ReadFile(filepath.Join(output, "codex/skills/example/agents/openai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sidecar), "display_name: Example display") || !strings.Contains(string(sidecar), "allow_implicit_invocation: false") {
		t.Fatalf("%s", sidecar)
	}
}

func TestLintRulesFromConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	fixture(t, path, "---\nname: example\ndescription: Example\n---\n\n:::instruction\nText.\n:::\n")
	testConfig(t, `[[lint.rules]]
id = "instruction-priority"
select = "directive"
assert = 'node.attrs.priority == "high"'
message = "Set priority"
severity = "error"
`)
	cmd := newLintCommand()
	cmd.SilenceUsage = true
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{path, "--format", "json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("custom error rule did not fail lint")
	}
	var diagnostics []lint.Diagnostic
	if err := json.Unmarshal(output.Bytes(), &diagnostics); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "instruction-priority" || diagnostics[0].Line != 6 {
		t.Fatalf("%+v", diagnostics)
	}
}

func TestLintSimilarityConfigAndJSON(t *testing.T) {
	root := t.TempDir()
	paragraph := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	fixture(t, filepath.Join(root, "a.md"), paragraph+"\n")
	fixture(t, filepath.Join(root, "b.md"), strings.Replace(paragraph, "every", "each", 1)+"\n")
	testConfig(t, "[lint]\nduplicate_similarity = 0.7\nduplicate_min_words = 12\n")
	cmd := newLintCommand()
	cmd.SilenceUsage = true
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{root, "--format", "json", "--strict"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("strict similarity warning did not fail lint")
	}
	var diagnostics []lint.Diagnostic
	if err := json.Unmarshal(out.Bytes(), &diagnostics); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "similar-content" || diagnostics[0].Method != "jaccard" || len(diagnostics[0].SharedPhrases) == 0 || diagnostics[0].Similarity == nil || len(diagnostics[0].Related) != 1 {
		t.Fatalf("%+v", diagnostics)
	}
}

func TestLintSemanticConfigAndJSON(t *testing.T) {
	root := t.TempDir()
	fixture(t, filepath.Join(root, "a.md"), "Repair broken program.\n")
	fixture(t, filepath.Join(root, "b.md"), "Fix faulty code.\n")
	modelPath, err := filepath.Abs("../lint/testdata/model")
	if err != nil {
		t.Fatal(err)
	}
	testConfig(t, "[lint]\nduplicate_min_words=1\n[lint.semantic]\nenabled=true\nthreshold=0.8\nmodel_path='"+filepath.ToSlash(modelPath)+"'\n")
	cmd := newLintCommand()
	cmd.SilenceUsage = true
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{root, "--format", "json", "--strict"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("strict semantic warning did not fail lint")
	}
	var diagnostics []lint.Diagnostic
	if err := json.Unmarshal(output.Bytes(), &diagnostics); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "semantic-content" || diagnostics[0].Method != "cosine" || diagnostics[0].Model != modelPath || diagnostics[0].Similarity == nil {
		t.Fatalf("%+v", diagnostics)
	}
}

func TestBuildOutputSettingAndCLIOverride(t *testing.T) {
	source := t.TempDir()
	fixture(t, filepath.Join(source, "skills/example/SKILL.md"), "---\nname: example\ndescription: Example\n---\n# Example\n")
	testConfig(t, "[build]\noutput='artifacts'\n[harness.claude]\nprofile='claude'\n")
	for _, override := range []bool{false, true} {
		destination := filepath.Join(filepath.Dir(configPath), "artifacts")
		args := []string{source}
		if override {
			destination = filepath.Join(t.TempDir(), "override")
			args = append(args, "--output", destination)
		}
		cmd := newBuildCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(destination, "claude/skills/example/SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}
