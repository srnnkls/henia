package vendor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func compileFixture(t *testing.T, profile, source string) *Result {
	t.Helper()
	definition, err := Load(profile, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := NewCompiler(definition)
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := yaml.Unmarshal([]byte(source), &input); err != nil {
		t.Fatal(err)
	}
	original, err := yaml.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Compile("example", input, Context{Profile: profile, Variables: map[string]string{"model_strong": "opus"}, Tools: map[string]string{"read": "Read"}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := yaml.Marshal(input)
	if err != nil || string(after) != string(original) {
		t.Fatal("mutated canonical input")
	}
	return result
}

func TestAllSkillProfiles(t *testing.T) {
	for _, profile := range []string{"claude", "claude-upload", "codex", "chatgpt", "opencode", "gemini", "github", "cursor", "agentskills"} {
		t.Run(profile, func(t *testing.T) {
			result := compileFixture(t, profile, "name: example\ndescription: Example skill\nmetadata:\n  owner: team\nhenia:\n  variables:\n    priority: high\n")
			if len(result.Warnings) != 0 || len(result.Files) != 0 || len(result.Frontmatter) != 3 {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestComputedVendorMappings(t *testing.T) {
	result := compileFixture(t, "claude", `name: example
description: Example skill
model_tier: strong
tools: [read]
user_invocable: false
henia:
  auto_invoke: false
`)
	want := map[string]any{"name": "example", "description": "Example skill", "model": "opus", "allowed-tools": "Read", "disable-model-invocation": true, "user-invocable": false}
	if !reflect.DeepEqual(result.Frontmatter, want) || len(result.Warnings) != 0 {
		t.Fatalf("%+v", result)
	}
	for _, profile := range []string{"codex", "chatgpt"} {
		result = compileFixture(t, profile, "name: example\ndescription: Example skill\nhenia:\n  auto_invoke: false\n  targets:\n    "+profile+":\n      openai:\n        interface:\n          display_name: Example\n")
		var sidecar map[string]any
		if err := yaml.Unmarshal(result.Files["skills/example/agents/openai.yaml"], &sidecar); err != nil {
			t.Fatal(err)
		}
		if sidecar["policy"].(map[string]any)["allow_implicit_invocation"] != false || sidecar["interface"].(map[string]any)["display_name"] != "Example" {
			t.Fatalf("%+v", result)
		}
		if len(result.Frontmatter) != 2 {
			t.Fatalf("sidecar leaked into frontmatter: %+v", result)
		}
	}
}

func TestNativeOverridesAndUnsupportedFields(t *testing.T) {
	result := compileFixture(t, "claude", "name: example\ndescription: Example\nhenia:\n  auto_invoke: false\n  targets:\n    claude:\n      frontmatter:\n        disable-model-invocation: false\n        context: fork\n")
	if result.Frontmatter["disable-model-invocation"] != false || result.Frontmatter["context"] != "fork" {
		t.Fatalf("%+v", result)
	}
	result = compileFixture(t, "opencode", "name: example\ndescription: Example\nallowed_tools: [read]\n")
	if len(result.Warnings) != 1 || len(result.Frontmatter) != 2 {
		t.Fatalf("%+v", result)
	}
}

func TestProfileResolutionAndCustomExpressions(t *testing.T) {
	project, user := t.TempDir(), t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(user, "harnesses/custom/transform.toml"), `fields = ["name", "description", "enabled"]
[computed]
enabled = "false"
`)
	write(filepath.Join(project, ".henia/harnesses/custom/transform.toml"), `[computed]
enabled = 'input.description == "Example"'
[files.manifest]
path = "manifest/{name}.json"
format = "json"
value = '{skill: input.name}'
`)
	p, err := Load("custom", project, user)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCompiler(p)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Compile("example", map[string]any{"name": "example", "description": "Example"}, Context{Profile: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Frontmatter["enabled"] != true || !strings.Contains(string(result.Files["manifest/example.json"]), `"skill": "example"`) {
		t.Fatalf("%+v", result)
	}
}

func TestProfileRejectsInvalidExpressionsAndPaths(t *testing.T) {
	for _, p := range []Profile{
		{Fields: []string{"name"}, Computed: map[string]string{"name": "unknown.field"}},
		{Fields: []string{"name"}, Files: map[string]File{"escape": {Path: "../escape", Format: "text", Value: `"bad"`}}},
		{Fields: []string{"name"}, Files: map[string]File{"escape": {Path: "a/../escape", Format: "text", Value: `"bad"`}}},
	} {
		if _, err := NewCompiler(p); err == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
}

func TestProfileRejectsNonDataAndInvalidOutputTypes(t *testing.T) {
	for _, p := range []Profile{
		{Fields: []string{"name", "description", "custom"}, Computed: map[string]string{"custom": "toolNames"}},
		{Fields: []string{"name", "description", "enabled"}, Computed: map[string]string{"enabled": `'false'`}},
		{Fields: []string{"name", "description"}, Files: map[string]File{"invalid": {Path: "output.yaml", Format: "yaml", Value: "toolNames"}}},
	} {
		c, err := NewCompiler(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Compile("example", map[string]any{"name": "example", "description": "Example"}, Context{}); err == nil {
			t.Fatalf("accepted invalid output: %+v", p)
		}
	}
}
