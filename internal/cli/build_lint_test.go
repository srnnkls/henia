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
