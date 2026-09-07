package config

import (
	"path/filepath"
	"testing"
)

func TestLayeredConfigPreservesFalseAndMergesTables(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), []byte(`[harness.claude]
profile = "claude"
strict = true
include = ["one"]
[harness.claude.variables]
custom = "user"
[harness.claude.tools]
read = "UserRead"
`), []byte(`[harness.claude]
strict = false
include = ["two"]
[harness.claude.variables]
custom = "project"
`))
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Harness["claude"]
	if h.Strict || h.Variables["custom"] != "project" || h.Variables["model_strong"] != "opus" || h.Tools["read"] != "UserRead" || len(h.Include) != 2 {
		t.Fatalf("%+v", h)
	}
	if len(cfg.Harness) != 1 {
		t.Fatal("explicit config enabled unrelated harnesses")
	}
}

func TestLegacyConfigDoesNotOptIntoProfile(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte("[harness.claude]\npath='output'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Harness["claude"].Profile != "" {
		t.Fatal("legacy config unexpectedly enabled profile")
	}
	if len(cfg.Harness["claude"].Artifacts) != 0 || len(cfg.Artifacts) != 0 {
		t.Fatal("legacy config inherited the skill-only profile filter")
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
	user := []byte("[lint.semantic]\nenabled=true\nmodel_path='models/potion'\nthreshold=0.4\n")
	cfg, err := decodeLayers(projectRoot, userRoot, user, []byte("[lint.semantic]\nthreshold=0.5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lint.Semantic.ModelPath != filepath.Join(userRoot, "models/potion") || cfg.Lint.Semantic.Threshold != 0.5 {
		t.Fatalf("%+v", cfg.Lint.Semantic)
	}
	cfg, err = decodeLayers(projectRoot, userRoot, user, []byte("[lint.semantic]\nmodel_path='local-model'\nenabled=false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lint.Semantic.ModelPath != filepath.Join(projectRoot, "local-model") || cfg.Lint.Semantic.Enabled {
		t.Fatalf("%+v", cfg.Lint.Semantic)
	}
	if _, err := decodeLayers(projectRoot, userRoot, nil, []byte("[lint.semantic]\nenabled=true\nmodel_path='model'\n")); err == nil {
		t.Fatal("accepted uncalibrated implicit threshold")
	}
}
