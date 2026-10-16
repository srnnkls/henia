package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia"
)

func TestBuildConfigArtifact(t *testing.T) {
	source, output := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "skills", "demo.md"), []byte("---\nname: demo\ndescription: Demo.\n---\n\nBody.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	codex := henia.Harness{
		Profile:     "codex",
		Artifacts:   []string{"skills", "config"},
		ProjectRoot: source,
		Shared:      map[string]any{"mcp": map[string]any{"docs": map[string]any{"url": "https://docs.example.com/mcp"}}},
		Config:      map[string]any{"approval_policy": "never"},
	}
	skillsOnly := codex
	skillsOnly.Artifacts = []string{"skills"}
	result, err := Run(t.Context(), []string{source}, output, map[string]henia.Harness{"codex": codex, "plain": skillsOnly})
	if err != nil || len(result.Errors) > 0 {
		t.Fatal(err, result.Errors)
	}
	data, err := os.ReadFile(filepath.Join(output, "codex", "config.toml"))
	if err != nil || !strings.Contains(string(data), "approval_policy = 'never'") || !strings.Contains(string(data), "[mcp_servers.docs]") {
		t.Fatalf("%v\n%s", err, data)
	}
	if _, err := os.Stat(filepath.Join(output, "plain", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("config built without artifacts = [\"config\"]")
	}

	codex.Files = map[string]henia.File{"config.toml": {Source: "skills/demo.md"}}
	result, err = Run(t.Context(), []string{source}, t.TempDir(), map[string]henia.Harness{"codex": codex})
	if err != nil || len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Error(), "collision") {
		t.Fatalf("config overwrote a harness file: %v %v", err, result.Errors)
	}

	codex.Files, codex.Strict = nil, true
	codex.Config = map[string]any{"approval_policy": "sometimes"}
	result, err = Run(t.Context(), []string{source}, t.TempDir(), map[string]henia.Harness{"codex": codex})
	if err != nil || len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Error(), "invalid config") {
		t.Fatalf("strict accepted a schema violation: %v %v", err, result.Errors)
	}

	codex.Strict, codex.Config = false, nil
	claude := codex
	claude.Profile = "claude"
	script := filepath.Join(source, "line.sh")
	claude.StatusLine = map[string]any{"file": script}
	result, err = Run(t.Context(), []string{source}, t.TempDir(), map[string]henia.Harness{"claude": claude})
	if err != nil || len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Error(), "statusline") {
		t.Fatalf("accepted a missing status line script: %v %v", err, result.Errors)
	}
	if err := os.WriteFile(script, []byte("echo hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	result, err = Run(t.Context(), []string{source}, out, map[string]henia.Harness{"claude": claude})
	if err != nil || len(result.Errors) > 0 || len(result.Warnings) > 0 {
		t.Fatal(err, result.Errors, result.Warnings)
	}
	deployed := filepath.Join(out, "claude", "statusline.sh")
	written, err := os.ReadFile(deployed)
	info, statErr := os.Stat(deployed)
	if err != nil || statErr != nil || string(written) != "#!/bin/sh\necho hi\n" || info.Mode().Perm() != 0o755 {
		t.Fatalf("status line not deployed: %v %q %v", err, written, info)
	}
	settings, _ := os.ReadFile(filepath.Join(out, "claude", "settings.json"))
	if strings.Contains(string(settings), source) {
		t.Fatalf("settings reference the source path: %s", settings)
	}
	codex.StatusLine = map[string]any{"command": "echo hi"}
	result, err = Run(t.Context(), []string{source}, t.TempDir(), map[string]henia.Harness{"codex": codex})
	if err != nil || len(result.Errors) > 0 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "no status line") {
		t.Fatalf("codex took a status line silently: %v %v %v", err, result.Errors, result.Warnings)
	}
}
