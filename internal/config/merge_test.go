package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func TestConfigDataReplacesArraysAcrossLayers(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), []byte(`artifacts = ["skills", "agents"]
[shared.mcp.github]
command = "gh-mcp"
args = ["user"]
[harness.codex]
artifacts = ["skills"]
[harness.codex.config]
notify = ["user"]
[harness.codex.variables]
keep = "user"
`), []byte(`artifacts = ["config"]
[shared.mcp.github]
args = ["project"]
[harness.codex]
artifacts = ["config"]
[harness.codex.config]
notify = ["project"]
`))
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Harness["codex"]
	github := cfg.Shared["mcp"].(map[string]any)["github"].(map[string]any)
	if !slices.Equal(cfg.Artifacts, []string{"config"}) || !slices.Equal(h.Artifacts, []string{"config"}) {
		t.Fatalf("artifacts extended: %v %v", cfg.Artifacts, h.Artifacts)
	}
	if !reflect.DeepEqual(github["args"], []any{"project"}) || github["command"] != "gh-mcp" {
		t.Fatalf("shared: %v", github)
	}
	if !reflect.DeepEqual(h.Config["notify"], []any{"project"}) || h.Variables["keep"] != "user" {
		t.Fatalf("harness: %+v", h)
	}
	if !reflect.DeepEqual(h.Shared, cfg.Shared) {
		t.Fatal("harness does not carry [shared]")
	}
}

func TestProfileExtendedArraysKeepEveryLayersItemsOnce(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), []byte(`[harness.claude.config]
model = "sonnet"
[harness.claude.config.hooks]
PreToolUse = [{ hooks = [{ command = "fas" }] }, { hooks = [{ command = "peer" }] }]
Stop = [{ hooks = [{ command = "fas" }] }]
[harness.claude.config.permissions]
allow = ["Read"]
`), []byte(`[harness.claude.config]
model = "opus"
[harness.claude.config.hooks]
PreToolUse = [{ hooks = [{ command = "peer" }] }, { hooks = [{ command = "limen" }] }]
[harness.claude.config.permissions]
allow = ["Bash"]
`))
	if err != nil {
		t.Fatal(err)
	}
	native := cfg.Harness["claude"].Config
	hooks := native["hooks"].(map[string]any)
	command := func(item any) any { return item.(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] }
	var pre []any
	for _, item := range hooks["PreToolUse"].([]any) {
		pre = append(pre, command(item))
	}
	if !reflect.DeepEqual(pre, []any{"fas", "peer", "limen"}) || len(hooks["Stop"].([]any)) != 1 {
		t.Fatalf("hooks: %v", hooks)
	}
	if native["model"] != "opus" || !reflect.DeepEqual(native["permissions"].(map[string]any)["allow"], []any{"Bash"}) {
		t.Fatalf("native: %v", native)
	}
}

func TestPerTypeProfilesValidateAtLoad(t *testing.T) {
	if _, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte(`[harness.claude.agents]
profile = "missing-profile"
`)); err == nil {
		t.Fatal("accepted an unknown per-type profile")
	}
	if _, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte(`[harness.claude.skills]
layout = "flat"
`)); err == nil {
		t.Fatal("accepted a skill profile with a flat skills layout")
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".henia/harnesses/claude-agent"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".henia/harnesses/claude-agent/transform.toml"), []byte("fields = [\"name\", \"description\"]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeLayers(project, t.TempDir(), nil, []byte(`[harness.claude.agents]
profile = "claude-agent"
layout = "flat"
`)); err != nil {
		t.Fatalf("rejected a flat agent profile: %v", err)
	}
}

func TestStatusLineResolvesAndReplacesPerLayer(t *testing.T) {
	project, user := t.TempDir(), t.TempDir()
	cfg, err := decodeLayers(project, user, []byte("[harness.claude.statusline]\nfile = \"line.sh\"\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Harness["claude"].StatusLine["file"]; got != filepath.Join(user, "line.sh") {
		t.Fatalf("user file resolved to %v", got)
	}
	cfg, err = decodeLayers(project, user, []byte("[harness.claude.statusline]\nfile = \"line.sh\"\npadding = 1\n"), []byte("[harness.claude.statusline]\ncommand = \"echo hi\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Harness["claude"].StatusLine; !reflect.DeepEqual(got, map[string]any{"command": "echo hi"}) {
		t.Fatalf("project status line merged with the user's: %v", got)
	}
}

func TestLocalHarnessOverProjectWithoutHarness(t *testing.T) {
	merged := mergeLayer(map[string]any{"artifacts": []any{"skills"}}, map[string]any{"harness": map[string]any{"claude": map[string]any{"strict": true}}}, nil)
	if merged["harness"].(map[string]any)["claude"].(map[string]any)["strict"] != true {
		t.Fatalf("%v", merged)
	}
}

func TestPresetDirectivesSurviveACustomProfile(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".henia/harnesses/mine"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".henia/harnesses/mine/transform.toml"), []byte("fields = [\"name\", \"description\"]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeLayers(project, t.TempDir(), nil, []byte("[harness.claude]\nprofile = \"mine\"\n[harness.custom]\nprofile = \"claude\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Harness["claude"].Directives != "xml" || cfg.Harness["custom"].Directives != "xml" {
		t.Fatalf("%q %q", cfg.Harness["claude"].Directives, cfg.Harness["custom"].Directives)
	}
}

func TestHarnessTablesResolveStatusLineAgainstTheirFile(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "henia.toml")
	if err := os.WriteFile(path, []byte("[harness.claude.statusline]\nfile = \"line.sh\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tables, err := HarnessTables(path)
	if err != nil {
		t.Fatal(err)
	}
	statusline := tables["claude"].(map[string]any)["statusline"].(map[string]any)
	if got := statusline["file"]; got != filepath.Join(project, "line.sh") {
		t.Fatalf("status line file resolved to %v", got)
	}
}

func TestUserFilesStayOutOfProjectBuilds(t *testing.T) {
	project, user := t.TempDir(), t.TempDir()
	userData := []byte("[harness.claude.files]\n\"a.sh\" = { source = \"claude/a.sh\" }\n\"b.sh\" = { source = \"claude/b.sh\" }\n")
	cfg, err := decodeLayers(project, user, userData, []byte("[harness.claude.files]\n\"b.sh\" = { source = \"own/b.sh\" }\n"))
	if err != nil {
		t.Fatal(err)
	}
	files := cfg.Harness["claude"].Files
	if _, ok := files["a.sh"]; ok {
		t.Fatalf("user file in a project build: %+v", files["a.sh"])
	}
	if files["b.sh"].Root != "" || files["b.sh"].Source != "own/b.sh" {
		t.Fatalf("project file %+v", files["b.sh"])
	}
}

func TestUserFilesResolveInTheUserLayerForHomeInstalls(t *testing.T) {
	pkg, user := t.TempDir(), t.TempDir()
	pkgData := []byte("[harness.claude.files]\n\"b.sh\" = { source = \"own/b.sh\" }\n")
	userData := []byte("[harness.claude.files]\n\"a.sh\" = { source = \"claude/a.sh\" }\n")
	cfg, err := decode(pkg, user, []layer{{pkgData, pkg, false}, {userData, user, true}}, true)
	if err != nil {
		t.Fatal(err)
	}
	files := cfg.Harness["claude"].Files
	if files["a.sh"].Root != user {
		t.Fatalf("user file root %q", files["a.sh"].Root)
	}
	if files["b.sh"].Root != "" || files["b.sh"].Source != "own/b.sh" {
		t.Fatalf("package file %+v", files["b.sh"])
	}
}

func TestUserLocalLayersOverTheUserConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	user := filepath.Join(home, "henia")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"henia.toml":       "[harness.claude.variables]\nshared = \"user\"\nkept = \"user\"\n",
		"henia.local.toml": "[harness.claude.variables]\nshared = \"local\"\n",
	} {
		if err := os.WriteFile(filepath.Join(user, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	project := filepath.Join(t.TempDir(), "henia.toml")
	if err := os.WriteFile(project, []byte("[harness.claude]\nprofile = \"claude\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	load := map[string]func() (*Config, error){
		"project": func() (*Config, error) { return Load(project) },
		"package": func() (*Config, error) { return LoadPackage(project, nil, true) },
	}
	for name, load := range load {
		cfg, err := load()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := cfg.Harness["claude"].Variables; got["shared"] != "local" || got["kept"] != "user" {
			t.Fatalf("%s variables: %v", name, got)
		}
	}
}
