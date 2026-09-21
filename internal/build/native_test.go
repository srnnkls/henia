package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
)

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNativeAgentsAndSupportingDocuments(t *testing.T) {
	root := t.TempDir()
	put(t, filepath.Join(root, "skills/code/SKILL.md"), "---\nname: code\ndescription: Code carefully.\n---\n\nSkill.\n")
	put(t, filepath.Join(root, "agents/tester.md"), "---\nname: tester\ndescription: Prove failures.\nhenia:\n  targets:\n    claude-agent:\n      frontmatter:\n        skills: [test]\n---\n\nRole contract.\n")
	put(t, filepath.Join(root, ".henia/harnesses/claude-agent/transform.toml"), "fields = [\"name\", \"description\", \"skills\"]\n")
	put(t, filepath.Join(root, ".henia/harnesses/portable-agent/transform.toml"), "fields = [\"name\", \"description\"]\n")
	put(t, filepath.Join(root, "instructions/AGENTS.md"), "Read [code](../skills/code/SKILL.md).\n")
	harnesses := map[string]henia.Harness{}
	for _, name := range []string{"claude", "codex"} {
		profile, entry := "portable-agent", "AGENTS.md"
		if name == "claude" {
			profile, entry = "claude-agent", "CLAUDE.md"
		}
		harnesses[name] = henia.Harness{
			ProjectRoot: root, Profile: name, Strict: true,
			ArtifactMappings: map[string]henia.ArtifactMapping{"agents": {Profile: profile, Structure: "flat"}},
			Files: map[string]henia.File{
				"instructions/AGENTS.md": {Source: "instructions/AGENTS.md"},
				entry:                    {Source: "instructions/AGENTS.md", Replace: map[string]string{"](../": "]("}},
			},
		}
	}
	output := filepath.Join(root, "build")
	result, err := RunClean(t.Context(), []string{root}, output, harnesses)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 || result.Built != 4 {
		t.Fatalf("build: %+v", result)
	}
	for _, name := range []string{"claude", "codex"} {
		data, err := os.ReadFile(filepath.Join(output, name, "agents/tester.md"))
		if err != nil {
			t.Fatal(err)
		}
		agent, err := artifact.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := agent.Frontmatter["henia"]; exists {
			t.Fatal("canonical metadata leaked")
		}
		_, hasSkills := agent.Frontmatter["skills"]
		if hasSkills != (name == "claude") {
			t.Fatalf("agent metadata for %s: %v", name, agent.Frontmatter)
		}
		if agent.Body != "Role contract.\n" {
			t.Fatal("role body changed")
		}
		entry := "AGENTS.md"
		if name == "claude" {
			entry = "CLAUDE.md"
		}
		data, err = os.ReadFile(filepath.Join(output, name, entry))
		if err != nil || string(data) != "Read [code](skills/code/SKILL.md).\n" {
			t.Fatalf("entrypoint: %s, %v", data, err)
		}
		if _, err := os.Stat(filepath.Join(output, name, "skills/code/SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCleanBuildPrunesAndKeepsPreviousOutputOnFailure(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	main := filepath.Join(source, "skills/code/SKILL.md")
	put(t, main, "Original.\n")
	resource := filepath.Join(source, "skills/code/old.txt")
	put(t, resource, "Resource.\n")
	output := filepath.Join(root, "output")
	harnesses := map[string]henia.Harness{"claude": {}}
	build := func() *Result {
		t.Helper()
		result, err := RunClean(t.Context(), []string{source}, output, harnesses)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := build(); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	put(t, main, "{{if}}\n")
	if result := build(); len(result.Errors) == 0 {
		t.Fatal("invalid template succeeded")
	}
	data, err := os.ReadFile(filepath.Join(output, "claude/skills/code/SKILL.md"))
	if err != nil || string(data) != "Original.\n" {
		t.Fatal("failed build changed published artifacts")
	}
	put(t, main, "Updated.\n")
	if err := os.Remove(resource); err != nil {
		t.Fatal(err)
	}
	if result := build(); len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	if _, err := os.Stat(filepath.Join(output, "claude/skills/code/old.txt")); !os.IsNotExist(err) {
		t.Fatal("stale resource survived")
	}
}

func TestSupportingFilesParticipateInPreflight(t *testing.T) {
	for _, outputPath := range []string{"../escape.md", "skills/code/SKILL.md"} {
		t.Run(outputPath, func(t *testing.T) {
			root := t.TempDir()
			put(t, filepath.Join(root, "skills/code/SKILL.md"), "Canonical.\n")
			put(t, filepath.Join(root, "instructions.md"), "Instructions.\n")
			output := filepath.Join(root, "build")
			result, err := RunClean(t.Context(), []string{root}, output, map[string]henia.Harness{"claude": {Files: map[string]henia.File{outputPath: {Source: "instructions.md"}}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Errors) == 0 {
				t.Fatal("unsafe output accepted")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("preflight failure wrote outputs")
			}
		})
	}
}

func TestCleanBuildCannotReplaceCanonicalSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	put(t, filepath.Join(source, "skills/code/SKILL.md"), "Canonical.\n")
	for _, output := range []string{source, root, filepath.Join(source, "skills"), filepath.Join(source, "skills/code")} {
		_, err := RunClean(t.Context(), []string{source}, output, map[string]henia.Harness{"claude": {}})
		if err == nil || !strings.Contains(err.Error(), "contains canonical") {
			t.Fatalf("unsafe clean output: %v", err)
		}
	}
}
