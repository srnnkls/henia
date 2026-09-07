package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
)

func TestBuildLocalSource(t *testing.T) {
	tmpDir := t.TempDir()

	sourceDir := filepath.Join(tmpDir, "source")
	skillsDir := filepath.Join(sourceDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "simple.md"), []byte(`---
name: simple
---

# Simple Skill
`), 0644)

	targetDir := filepath.Join(tmpDir, "target")
	os.MkdirAll(targetDir, 0755)

	harnesses := map[string]henia.Harness{
		"claude": {
			Artifacts: []string{"skills"},
		},
	}

	sources := []string{sourceDir}

	result, err := Run(t.Context(), sources, targetDir, harnesses)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if result.Built != 1 {
		t.Errorf("Synced = %d, want 1", result.Built)
	}
}

func TestBuildTransformApplied(t *testing.T) {
	tmpDir := t.TempDir()

	sourceDir := filepath.Join(tmpDir, "source")
	skillsDir := filepath.Join(sourceDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "test.md"), []byte(`---
name: test
model: strong
---

# Test
`), 0644)

	targetDir := filepath.Join(tmpDir, "target")
	os.MkdirAll(targetDir, 0755)

	harnesses := map[string]henia.Harness{
		"claude": {
			Artifacts: []string{"skills"},
			Keys: map[string]string{
				"model": "model_preference",
			},
			Values: map[string]map[string]string{
				"model_preference": {
					"strong": "opus",
				},
			},
		},
	}

	sources := []string{sourceDir}

	result, err := Run(t.Context(), sources, targetDir, harnesses)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if result.Built != 1 {
		t.Errorf("Synced = %d, want 1", result.Built)
	}

	targetPath := filepath.Join(targetDir, "claude", "skills", "test", "SKILL.md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	content := string(data)
	art, err := artifact.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if art.Frontmatter["model_preference"] != "opus" {
		t.Errorf("model_preference = %v, want opus (content: %s)", art.Frontmatter["model_preference"], content)
	}
}

func TestBuildFiltersByHarnessArtifacts(t *testing.T) {
	tmpDir := t.TempDir()

	sourceDir := filepath.Join(tmpDir, "source")

	skillsDir := filepath.Join(sourceDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "my-skill.md"), []byte(`---
name: my-skill
---
# My Skill
`), 0644)

	commandsDir := filepath.Join(sourceDir, "commands")
	os.MkdirAll(commandsDir, 0755)
	os.WriteFile(filepath.Join(commandsDir, "my-command.md"), []byte(`---
name: my-command
---
# My Command
`), 0644)

	targetDir := filepath.Join(tmpDir, "target")
	os.MkdirAll(targetDir, 0755)

	harnesses := map[string]henia.Harness{
		"skills-only": {
			Artifacts: []string{"skills"},
		},
	}

	sources := []string{sourceDir}

	result, err := Run(t.Context(), sources, targetDir, harnesses)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if result.Built != 1 {
		t.Errorf("Synced = %d, want 1 (only skill)", result.Built)
	}

	skillPath := filepath.Join(targetDir, "skills-only", "skills", "my-skill", "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Error("Expected skill to be synced")
	}

	commandPath := filepath.Join(targetDir, "skills-only", "commands", "my-command", "COMMAND.md")
	if _, err := os.Stat(commandPath); !os.IsNotExist(err) {
		t.Error("Expected command to NOT be synced (harness only wants skills)")
	}
}

func TestBuildMultipleLocalSourcesAndHarnesses(t *testing.T) {
	root := t.TempDir()
	sources := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	for i, source := range sources {
		skill := filepath.Join(source, "skills", []string{"first", "second"}[i])
		if err := os.MkdirAll(skill, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Canonical\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(root, "out")
	result, err := Run(t.Context(), sources, output, map[string]henia.Harness{"claude": {}, "codex": {}})
	if err != nil || len(result.Errors) != 0 || result.Built != 4 {
		t.Fatalf("%+v (%v)", result, err)
	}
	for _, harness := range []string{"claude", "codex"} {
		for _, skill := range []string{"first", "second"} {
			if _, err := os.Stat(filepath.Join(output, harness, "skills", skill, "SKILL.md")); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestBuildRejectsInvalidHarnessAndCancellation(t *testing.T) {
	for _, name := range []string{"../escape", ".", "nested/name"} {
		if _, err := Run(t.Context(), []string{t.TempDir()}, t.TempDir(), map[string]henia.Harness{name: {}}); err == nil {
			t.Fatalf("accepted harness %q", name)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Run(ctx, []string{t.TempDir()}, t.TempDir(), map[string]henia.Harness{"claude": {}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
