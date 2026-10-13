package transform

import (
	"cmp"
	"regexp"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
)

func TestExecuteTemplate(t *testing.T) {
	content := `---
name: code-test
model: {{.model_strong}}
---

# Code Test

Use {{.model_strong}} for complex tasks.
Use {{.model_weak}} for simple tasks.
`

	vars := map[string]string{
		"model_strong": "opus",
		"model_weak":   "haiku",
	}

	result, err := ExecuteTemplate(content, vars)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	expected := `---
name: code-test
model: opus
---

# Code Test

Use opus for complex tasks.
Use haiku for simple tasks.
`

	if result != expected {
		t.Errorf("ExecuteTemplate() =\n%s\nwant:\n%s", result, expected)
	}
}

func TestExecuteTemplateConditional(t *testing.T) {
	content := `{{if .feature_enabled}}Feature is ON{{else}}Feature is OFF{{end}}`

	vars := map[string]string{
		"feature_enabled": "true",
	}

	result, err := ExecuteTemplate(content, vars)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	if result != "Feature is ON" {
		t.Errorf("ExecuteTemplate() = %q, want %q", result, "Feature is ON")
	}
}

func TestExecuteTemplateMissingVariable(t *testing.T) {
	content := `{{define "note"}}[{{.absent}}]{{end}}A{{.absent}}B{{if .flag}}{{.absent}}{{end}}{{range .items}}<{{.}}{{$.absent}}>{{end}}{{with .flag}}{{$.absent}}{{end}}{{template "note" .}}{{.flag | printf "%s"}}`
	vars := map[string]any{"flag": "on", "items": []string{"x"}}

	result, err := ExecuteTemplate(content, vars)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	if want := "AB<x>[]on"; result != want {
		t.Errorf("ExecuteTemplate() = %q, want %q", result, want)
	}
}

func TestApplyMappings(t *testing.T) {
	fm := map[string]any{
		"name":          "code-test",
		"allowed_tools": []string{"read", "write"},
		"resources":     []string{"reference/"},
	}

	mappings := map[string]string{
		"allowed_tools": "tools",
		"resources":     "files",
	}

	result := ApplyMappings(fm, mappings)

	if _, ok := result["allowed_tools"]; ok {
		t.Error("allowed_tools should be renamed to tools")
	}
	if _, ok := result["tools"]; !ok {
		t.Error("tools key missing")
	}
	if _, ok := result["files"]; !ok {
		t.Error("files key missing")
	}
	if result["name"] != "code-test" {
		t.Error("name should be unchanged")
	}
}

func TestApplyMappingsEmpty(t *testing.T) {
	fm := map[string]any{
		"name": "test",
	}

	result := ApplyMappings(fm, nil)

	if result["name"] != "test" {
		t.Error("name should be unchanged with nil mappings")
	}
}

func TestTransformArtifact(t *testing.T) {
	art := &artifact.Artifact{
		Name: "code-test",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name":          "code-test",
			"model":         "{{.model_strong}}",
			"allowed_tools": []string{"read"},
		},
		Body: "Use the strong model.\n",
	}

	tr := &Transformer{
		Variables: map[string]string{
			"model_strong": "opus",
		},
		Keys: map[string]string{
			"allowed_tools": "tools",
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	if result.Frontmatter["model"] != "opus" {
		t.Errorf("model = %v, want opus", result.Frontmatter["model"])
	}
	if _, ok := result.Frontmatter["tools"]; !ok {
		t.Error("tools key missing after mapping")
	}
	if result.Body != "Use the strong model.\n" {
		t.Errorf("Body = %q", result.Body)
	}
}

func TestTransformPreservesMetadata(t *testing.T) {
	art := &artifact.Artifact{
		Name:        "test",
		Type:        artifact.TypeSkill,
		SourcePath:  "/some/path",
		IsDirectory: true,
		Resources:   []string{"ref/"},
		Frontmatter: map[string]any{"name": "test"},
		Body:        "Content",
	}

	tr := &Transformer{}
	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	if result.Name != art.Name {
		t.Error("Name not preserved")
	}
	if result.Type != art.Type {
		t.Error("Type not preserved")
	}
	if result.SourcePath != art.SourcePath {
		t.Error("SourcePath not preserved")
	}
	if result.IsDirectory != art.IsDirectory {
		t.Error("IsDirectory not preserved")
	}
	if len(result.Resources) != len(art.Resources) {
		t.Error("Resources not preserved")
	}
}

func TestTransformReferencesInBody(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name": "test-skill",
		},
		Body: "Use `$code-test` for TDD.\nRun `/commit` when done.",
	}

	tr := &Transformer{
		References: map[string]ReferenceConfig{
			"skill":   {Output: "/{{.Name}}"},
			"command": {Output: "/{{.Name}}"},
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	expected := "Use `/code-test` for TDD.\nRun `/commit` when done."
	if result.Body != expected {
		t.Errorf("Body =\n%s\nwant:\n%s", result.Body, expected)
	}
}

func TestTransformToolReferences(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name": "test-skill",
		},
		Body: "Use `!bash` for shell commands and `!read` to view files.",
	}

	tr := &Transformer{
		Tools: map[string]string{
			"bash": "Bash",
			"read": "Read",
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	expected := "Use `Bash` for shell commands and `Read` to view files."
	if result.Body != expected {
		t.Errorf("Body =\n%s\nwant:\n%s", result.Body, expected)
	}
}

func TestTransformMixedReferences(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name": "test-skill",
		},
		Body: "Use `$code-test` and `!bash` for development.",
	}

	tr := &Transformer{
		References: map[string]ReferenceConfig{
			"skill": {Output: "/{{.Name}}"},
		},
		Tools: map[string]string{
			"bash": "Bash",
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	expected := "Use `/code-test` and `Bash` for development."
	if result.Body != expected {
		t.Errorf("Body =\n%s\nwant:\n%s", result.Body, expected)
	}
}

func TestApplyValueMappings(t *testing.T) {
	fm := map[string]any{
		"name":  "test-skill",
		"model": "opus",
	}

	values := map[string]map[string]string{
		"model": {
			"opus":  "claude-opus-4-5-20251101",
			"haiku": "claude-3-5-haiku-latest",
		},
	}

	result := ApplyValueMappings(fm, values)

	if result["model"] != "claude-opus-4-5-20251101" {
		t.Errorf("model = %v, want claude-opus-4-5-20251101", result["model"])
	}
	if result["name"] != "test-skill" {
		t.Error("name should be unchanged")
	}
}

func TestApplyValueMappingsUnmappedValue(t *testing.T) {
	fm := map[string]any{
		"model": "sonnet",
	}

	values := map[string]map[string]string{
		"model": {
			"opus": "claude-opus-4-5-20251101",
		},
	}

	result := ApplyValueMappings(fm, values)

	if result["model"] != "sonnet" {
		t.Errorf("model = %v, want sonnet (unmapped values should pass through)", result["model"])
	}
}

func TestApplyValueMappingsNil(t *testing.T) {
	fm := map[string]any{
		"model": "opus",
	}

	result := ApplyValueMappings(fm, nil)

	if result["model"] != "opus" {
		t.Error("value should be unchanged with nil values map")
	}
}

func TestTransformWithValueMappings(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name":  "test-skill",
			"model": "opus",
		},
		Body: "Content",
	}

	tr := &Transformer{
		Values: map[string]map[string]string{
			"model": {
				"opus": "claude-opus-4-5-20251101",
			},
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	if result.Frontmatter["model"] != "claude-opus-4-5-20251101" {
		t.Errorf("model = %v, want claude-opus-4-5-20251101", result.Frontmatter["model"])
	}
}

func TestTransformKeyThenValueMapping(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name":          "test-skill",
			"allowed_tools": "bash",
		},
		Body: "Content",
	}

	tr := &Transformer{
		Keys: map[string]string{
			"allowed_tools": "tools",
		},
		Values: map[string]map[string]string{
			"tools": {
				"bash": "Bash",
			},
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	if _, ok := result.Frontmatter["allowed_tools"]; ok {
		t.Error("allowed_tools should be renamed to tools")
	}
	if result.Frontmatter["tools"] != "Bash" {
		t.Errorf("tools = %v, want Bash", result.Frontmatter["tools"])
	}
}

func TestTransformReferenceWithTemplate(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name": "test-skill",
		},
		Body: "Reference `@my-agent` to delegate work.",
	}

	tr := &Transformer{
		References: map[string]ReferenceConfig{
			"agent": {Output: "**{{.Name}}** (agent)"},
		},
	}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	expected := "Reference **my-agent** (agent) to delegate work."
	if result.Body != expected {
		t.Errorf("Body =\n%s\nwant:\n%s", result.Body, expected)
	}
}

func TestTransformUnmappedReferencePassesThrough(t *testing.T) {
	art := &artifact.Artifact{
		Name: "test-skill",
		Type: artifact.TypeSkill,
		Frontmatter: map[string]any{
			"name": "test-skill",
		},
		Body: "Use `$code-test` skill.",
	}

	tr := &Transformer{}

	result, err := tr.Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	if result.Body != art.Body {
		t.Errorf("Body =\n%s\nwant (unchanged):\n%s", result.Body, art.Body)
	}
}

func TestExecuteTemplateKeepsRequired(t *testing.T) {
	result, err := ExecuteTemplate(`{{if false}}{{required "x"}}{{end}}ok`, map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	if result != "ok" {
		t.Errorf("ExecuteTemplate() = %q, want %q", result, "ok")
	}
}

func TestTransformRelatedForms(t *testing.T) {
	body := "# Skill\n\nSee :related[git] first.\n\n:::related\n- `git`: version control\n- `pr`: review threads\n:::\n"
	for _, format := range []string{"", "markdown"} {
		t.Run(cmp.Or(format, "directives"), func(t *testing.T) {
			art := &artifact.Artifact{Name: "code", Type: artifact.TypeSkill, Frontmatter: map[string]any{"name": "code"}, Body: body}
			result, err := (&Transformer{OutputFormat: format}).Transform(art)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if !strings.Contains(result.Body, "See !`henia context git` first.") {
				t.Errorf("text :related not expanded:\n%s", result.Body)
			}
			if !regexp.MustCompile("(?m)^## Related\n+- `git`: version control\n- `pr`: review threads\n").MatchString(result.Body) {
				t.Errorf("container :::related did not render a Related heading followed by its list:\n%s", result.Body)
			}
			if strings.Contains(result.Body, ":::") || strings.Contains(result.Body, "<related") {
				t.Errorf("container :::related left directive syntax:\n%s", result.Body)
			}
		})
	}
}

func TestTransformListingsByteIdentical(t *testing.T) {
	body := "# Route\n\n:slot[code.style code.check]\n\nUse :slot[code.check]{.inline} now.\n\nSections:\n\n:contents[]\n\nChecks: :contents[checks]\n\nRelated:\n\n:related[]\n\n- see :related[git]\n\nLiteral `:related[git]` stays.\n"
	want := "# Route\n\n!`henia slots --markdown code.style code.check`\n\nUse !`henia slots --inline code.check` now.\n\nSections:\n\n!`henia show route --toc`\n\nChecks: !`henia show checks --toc`\n\nRelated:\n\n!`henia context route`\n\n- see !`henia context git`\n\nLiteral `:related[git]` stays.\n"
	art := &artifact.Artifact{Name: "route", Type: artifact.TypeSkill, Frontmatter: map[string]any{"name": "route"}, Body: body}
	result, err := (&Transformer{}).Transform(art)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if result.Body != want {
		t.Errorf("Transform() body =\n%q\nwant:\n%q", result.Body, want)
	}
}

func TestExpandFileListsNamedSkillsOnly(t *testing.T) {
	body := "Style: :slot[code.style]{.inline}\n\n:contents[git]\n\n:related[git]\n\n:contents[]\n"
	want := "Style: !`henia slots --inline code.style`\n\n!`henia show git --toc`\n\n!`henia context git`\n\n:contents[]\n"
	if got := ExpandFile(body); got != want {
		t.Errorf("ExpandFile() =\n%q\nwant:\n%q", got, want)
	}
}
