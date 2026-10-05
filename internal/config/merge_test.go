package config

import (
	"path/filepath"
	"testing"
)

func TestLayeredConfigPreservesFalseAndMergesTables(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), []byte(`[harness.claude]
profile = "claude"
strict = true
[harness.claude.skills]
static = ["one"]
[harness.claude.variables]
custom = "user"
[harness.claude.tools]
read = "UserRead"
`), []byte(`[harness.claude]
strict = false
[harness.claude.skills]
static = ["two"]
[harness.claude.variables]
custom = "project"
`))
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Harness["claude"]
	if h.Strict || h.Variables["custom"] != "project" || h.Variables["model_strong"] != "opus" || h.Tools["read"] != "UserRead" || len(h.Skills.Static) != 2 {
		t.Fatalf("%+v", h)
	}
	if len(cfg.Harness) != 1 {
		t.Fatal("explicit config enabled unrelated harnesses")
	}
}

func TestRuleConfigRejectsUnknownKeys(t *testing.T) {
	_, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte(`[[lint.rules]]
id = "example"
select = "document"
assert = "true"
message = "Test"
severty = "error"
`))
	if err == nil {
		t.Fatal("accepted misspelled severity")
	}
}

func TestSemanticModelPathFollowsDeclaringLayer(t *testing.T) {
	projectRoot, userRoot := t.TempDir(), t.TempDir()
	user := []byte("[lint.semantic]\nenabled=true\nmodel_path='models/potion'\n")
	cfg, err := decodeLayers(projectRoot, userRoot, user, []byte("[lint.config.semantic-content]\nthreshold=0.5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lint.Semantic.ModelPath != filepath.Join(userRoot, "models/potion") || cfg.Lint.Config["semantic-content"]["threshold"] != 0.5 {
		t.Fatalf("%+v", cfg.Lint.Semantic)
	}
	cfg, err = decodeLayers(projectRoot, userRoot, user, []byte("[lint.semantic]\nmodel_path='local-model'\nenabled=false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lint.Semantic.ModelPath != filepath.Join(projectRoot, "local-model") || cfg.Lint.Semantic.Enabled {
		t.Fatalf("%+v", cfg.Lint.Semantic)
	}
}
