package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/library"
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
	fixture(t, filepath.Join(root, "skills/example/SKILL.md"), "---\nname: example\ndescription: Example\npriority: critical\n---\n\n:::prioritized\nUse `!read`.\n:::\n")
	fixture(t, filepath.Join(root, "skills/example/reference/guide.md"), "# Guide\n")
	testConfig(t, "[harness.claude]\ndirectives = 'xml'\n[harness.claude.tools]\nread = 'Read'\n[harness.codex]\ndirectives = 'keep'\n")
	components := filepath.Join(filepath.Dir(configPath), ".henia", "components")
	fixture(t, filepath.Join(components, "instruction.md"), "---\nform: container\nattributes:\n  priority:\n    type: string\nbody:\n  kind: markdown\nrender: keep\n---\n")
	fixture(t, filepath.Join(components, "prioritized.md"), "---\nform: container\nbody:\n  kind: markdown\n---\n:::instruction{priority=\"{{.priority}}\"}\n::body\n:::\n")
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
	testConfig(t, "[harness.claude]\ndirectives = 'xml'\n")
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

::include{component="instructions"}
`)
	testConfig(t, `[harness.claude]
profile = "claude"
strict = true
[harness.codex]
profile = "codex"
strict = true
[harness.codex.variables]
title = "Example display"
`)
	components := filepath.Join(filepath.Dir(configPath), ".henia", "components")
	fixture(t, filepath.Join(components, "instruction.md"), "---\nform: container\nattributes:\n  priority:\n    type: string\nbody:\n  kind: markdown\nrender: keep\n---\n")
	fixture(t, filepath.Join(components, "instructions.md"), "---\nform: leaf\nbody:\n  kind: none\n---\n:::instruction{priority=\"{{.priority}}\"}\nUse `$example`.\n:::\n")
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
query = '(directive :priority (not "high")) @d'
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
	fixture(t, filepath.Join(root, "SKILL.md"), "---\nname: similar\ndescription: Similar paragraphs\n---\n")
	fixture(t, filepath.Join(root, "a.md"), paragraph+"\n")
	fixture(t, filepath.Join(root, "b.md"), strings.Replace(paragraph, "every", "each", 1)+"\n")
	testConfig(t, "[lint.config.similar-content]\nsimilarity = 0.7\n")
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
	fixture(t, filepath.Join(root, "SKILL.md"), "---\nname: semantic\ndescription: Semantic paragraphs\n---\n")
	fixture(t, filepath.Join(root, "a.md"), "Repair broken program.\n")
	fixture(t, filepath.Join(root, "b.md"), "Fix faulty code.\n")
	modelPath, err := filepath.Abs("../lint/testdata/model")
	if err != nil {
		t.Fatal(err)
	}
	testConfig(t, "[lint.semantic]\nenabled=true\nmodel_path='"+filepath.ToSlash(modelPath)+"'\n[lint.config.semantic-content]\nmin-words=1\nthreshold=0.8\n")
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

func executeBuild(t *testing.T, args ...string) error {
	t.Helper()
	previous := configPath
	t.Cleanup(func() { configPath = previous })
	root := &cobra.Command{Use: "henia", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&configPath, "config", "henia.toml", "Config file path")
	root.AddCommand(newBuildCommand())
	root.SetOut(&bytes.Buffer{})
	root.SetArgs(append([]string{"build"}, args...))
	return root.Execute()
}

// sourceConfigFixture lays out a source directory with its own henia.toml and a
// working directory whose henia.toml declares a different harness.
func sourceConfigFixture(t *testing.T, sourceConfig bool) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	source := t.TempDir()
	fixture(t, filepath.Join(source, "skills/example/SKILL.md"), "---\nname: example\ndescription: Example\n---\n# Example\n")
	if sourceConfig {
		fixture(t, filepath.Join(source, "henia.toml"), "[build]\noutput = 'artifacts'\n[harness.source-only]\ndirectives = 'markdown'\n")
	}
	cwd := t.TempDir()
	fixture(t, filepath.Join(cwd, "henia.toml"), "[harness.cwd-only]\ndirectives = 'markdown'\n")
	t.Chdir(cwd)
	return source
}

func assertHarnesses(t *testing.T, output string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("harnesses in %s = %v, want %v", output, got, want)
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(output, name, "skills/example/SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildReadsSourceConfig(t *testing.T) {
	source := sourceConfigFixture(t, true)
	if err := executeBuild(t, source); err != nil {
		t.Fatal(err)
	}
	assertHarnesses(t, filepath.Join(source, "artifacts"), "source-only")
	override := filepath.Join(t.TempDir(), "override")
	if err := executeBuild(t, source, "--output", override); err != nil {
		t.Fatal(err)
	}
	assertHarnesses(t, override, "source-only")
}

func TestBuildExplicitConfigOverridesSourceConfig(t *testing.T) {
	source := sourceConfigFixture(t, true)
	explicit := filepath.Join(t.TempDir(), "explicit.toml")
	fixture(t, explicit, "[harness.explicit-only]\ndirectives = 'markdown'\n")
	output := filepath.Join(t.TempDir(), "out")
	if err := executeBuild(t, "--config", explicit, source, "--output", output); err != nil {
		t.Fatal(err)
	}
	assertHarnesses(t, output, "explicit-only")
}

func TestBuildWithoutSourceConfigReadsWorkingDirectoryConfig(t *testing.T) {
	source := sourceConfigFixture(t, false)
	output := filepath.Join(t.TempDir(), "out")
	if err := executeBuild(t, source, "--output", output); err != nil {
		t.Fatal(err)
	}
	assertHarnesses(t, output, "cwd-only")
}

func TestLintLoadsDependencyModules(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	store := library.PackagesDir(library.ProjectKey(project))
	fixture(t, filepath.Join(project, "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill for tests.\n---\n\n# Own\n\nTODO: finish.\n")
	fixture(t, filepath.Join(store, "dep", "skills", "dep", "SKILL.md"), "---\nname: dep\ndescription: Dependency skill.\n---\n\n# Dep\n\nTODO: never linted here.\n")
	fixture(t, filepath.Join(store, "dep", ".henia", "lint", "todo.md"), "## no-todo\n\n```hq\n(rule no-todo :message \"resolve the TODO\" (paragraph :matches /TODO/) @p)\n```\n")
	cmd := newLintCommand()
	cmd.SilenceUsage = true
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"skills", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var diagnostics []lint.Diagnostic
	if err := json.Unmarshal(out.Bytes(), &diagnostics); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "no-todo" || !strings.HasSuffix(diagnostics[0].Path, filepath.Join("own", "SKILL.md")) {
		t.Fatalf("%+v", diagnostics)
	}
}

func TestLintDefaultsToPackageContents(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	fixture(t, filepath.Join(project, "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill for tests.\n---\n\n[broken](absent.md)\n")
	fixture(t, filepath.Join(project, "instructions", "AGENTS.md"), "[broken](absent.md)\n")
	fixture(t, filepath.Join(project, "docs", "guide.md"), "[broken](absent.md)\n")
	testConfig(t, "artifacts = [\"skills\"]\n[harness.claude.files]\n\"CLAUDE.md\" = { source = \"instructions/AGENTS.md\" }\n")
	cmd := newLintCommand()
	cmd.SilenceUsage = true
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var diagnostics []lint.Diagnostic
	if err := json.Unmarshal(out.Bytes(), &diagnostics); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, d := range diagnostics {
		paths = append(paths, filepath.Base(d.Path))
	}
	if got := strings.Join(paths, " "); got != "AGENTS.md SKILL.md" {
		t.Fatalf("linted %s", got)
	}
}

func TestBuildReusesDependencyHarnesses(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store := library.PackagesDir(library.ProjectKey(project))
	dep := filepath.Join(store, "tropos")
	fixture(t, filepath.Join(dep, "henia.toml"), "[harness.claude]\nartifacts = ['skills', 'agents']\nstrict = true\n[harness.claude.skills]\ndefault = 'dynamic'\nhybrid = ['entry']\nstatic = ['pinned']\n[harness.claude.variables]\nteam = 'tropos'\n[harness.claude.agents]\nprofile = 'terse-agent'\nlayout = 'flat'\n[harness.claude.files]\n'CLAUDE.md' = { source = 'instructions/AGENTS.md' }\n")
	fixture(t, filepath.Join(dep, ".henia", "harnesses", "terse-agent", "transform.toml"), "fields = ['name', 'description']\n")
	fixture(t, filepath.Join(dep, "instructions", "AGENTS.md"), "# Instructions\n")
	fixture(t, filepath.Join(dep, "skills", "entry", "SKILL.md"), "---\nname: entry\ndescription: Entry skill.\n---\n\n# Entry\n\nMore.\n")
	fixture(t, filepath.Join(dep, "skills", "pinned", "SKILL.md"), "---\nname: pinned\ndescription: Pinned skill.\n---\n\n# Pinned\n\nAsk {{.team}}.\n")
	fixture(t, filepath.Join(dep, "skills", "library", "SKILL.md"), "---\nname: library\ndescription: Library skill.\n---\n\n# Library\n")
	fixture(t, filepath.Join(dep, "agents", "helper.md"), "---\nname: helper\ndescription: Helper agent.\n---\n\nHelp.\n")
	fixture(t, filepath.Join(project, "henia.toml"), "[harness.claude.variables]\nteam = 'cmw'\n")
	fixture(t, filepath.Join(project, ".henia", "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill.\nunsupported-key: kept out\n---\n\n# Own\n")
	output := filepath.Join(t.TempDir(), "out")
	if err := executeBuild(t, "--output", output); err != nil {
		t.Fatal(err)
	}
	var files []string
	if err := filepath.WalkDir(output, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			rel, _ := filepath.Rel(output, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"claude/CLAUDE.md", "claude/agents/helper.md", "claude/skills/entry/SKILL.md", "claude/skills/henia/SKILL.md", "claude/skills/own/SKILL.md", "claude/skills/pinned/SKILL.md"}
	if strings.Join(files, " ") != strings.Join(want, " ") {
		t.Fatalf("files = %v, want %v", files, want)
	}
	entry, _ := os.ReadFile(filepath.Join(output, "claude/skills/entry/SKILL.md"))
	if !strings.Contains(string(entry), "henia show entry --head") {
		t.Errorf("entry is not a hybrid head:\n%s", entry)
	}
	if pinned, _ := os.ReadFile(filepath.Join(output, "claude/skills/pinned/SKILL.md")); !strings.Contains(string(pinned), "Ask cmw.") {
		t.Errorf("pinned misses the project's variables:\n%s", pinned)
	}
}

func TestBuildAddsProjectHarnessesToDependencyHarnesses(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dep := filepath.Join(library.PackagesDir(library.ProjectKey(project)), "tropos")
	fixture(t, filepath.Join(dep, "henia.toml"), "[harness.claude.skills]\ndefault = 'dynamic'\n")
	fixture(t, filepath.Join(dep, "skills", "library", "SKILL.md"), "---\nname: library\ndescription: Library skill.\n---\n\n# Library\n")
	fixture(t, filepath.Join(project, "henia.toml"), "[harness.codex]\n[harness.claude.skills]\ndynamic = ['quiet']\n")
	fixture(t, filepath.Join(project, ".henia", "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill.\n---\n\n# Own\n")
	fixture(t, filepath.Join(project, ".henia", "skills", "quiet", "SKILL.md"), "---\nname: quiet\ndescription: Quiet skill.\n---\n\n# Quiet\n")
	output := filepath.Join(t.TempDir(), "out")
	if err := executeBuild(t, "--output", output); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"claude/skills/own/SKILL.md":     true,
		"claude/skills/quiet/SKILL.md":   false,
		"claude/skills/library/SKILL.md": false,
		"codex/skills/library/SKILL.md":  true,
		"codex/skills/quiet/SKILL.md":    true,
		"gemini":                         false,
	} {
		if _, err := os.Stat(filepath.Join(output, path)); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", path, err == nil, want)
		}
	}
}

func dependencyHarnessFixture(t *testing.T, projectConfig string) (string, string) {
	t.Helper()
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store := library.PackagesDir(library.ProjectKey(project))
	tropos := filepath.Join(store, "tropos")
	fixture(t, filepath.Join(tropos, "henia.toml"), "[harness.claude]\nstrict = true\n[harness.claude.skills]\ndefault = 'dynamic'\nhybrid = ['entry']\nstatic = ['pinned']\n")
	fixture(t, filepath.Join(tropos, "skills", "entry", "SKILL.md"), "---\nname: entry\ndescription: Entry skill.\n---\n\n# Entry\n\nMore.\n")
	fixture(t, filepath.Join(tropos, "skills", "pinned", "SKILL.md"), "---\nname: pinned\ndescription: Pinned skill.\n---\n\n# Pinned\n")
	fixture(t, filepath.Join(tropos, "skills", "library", "SKILL.md"), "---\nname: library\ndescription: Library skill.\n---\n\n# Library\n")
	fixture(t, filepath.Join(store, "imp", "skills", "ext", "SKILL.md"), "---\nname: ext\ndescription: External skill.\nversion: 1.0.0\n---\n\n# Ext\n")
	fixture(t, filepath.Join(project, "henia.toml"), projectConfig)
	fixture(t, filepath.Join(project, ".henia", "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill.\nversion: 1.0.0\n---\n\n# Own\n")
	fixture(t, filepath.Join(project, ".henia", "skills", "quiet", "SKILL.md"), "---\nname: quiet\ndescription: Quiet skill.\n---\n\n# Quiet\n")
	return tropos, filepath.Join(t.TempDir(), "out")
}

func TestBuildAppliesProjectModesUnderDependencyHarness(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		want         map[string]bool
	}{
		{"listing a dependency skill keeps project skills static", "[harness.claude.skills]\nstatic = ['ext']\n",
			map[string]bool{"own": true, "quiet": true, "ext": true, "pinned": true, "library": false}},
		{"listing a project skill dynamic changes only it", "[harness.claude.skills]\ndynamic = ['quiet']\n",
			map[string]bool{"own": true, "quiet": false, "ext": false, "pinned": true, "library": false}},
		{"a project default covers the project's unlisted skills", "[harness.claude.skills]\ndefault = 'dynamic'\nstatic = ['library']\n",
			map[string]bool{"own": false, "quiet": false, "ext": false, "pinned": true, "library": true}},
		{"a project listing overrides the owner's", "[harness.claude.skills]\ndynamic = ['pinned']\n",
			map[string]bool{"own": true, "quiet": true, "pinned": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, output := dependencyHarnessFixture(t, tc.config)
			if err := executeBuild(t, "--output", output); err != nil {
				t.Fatal(err)
			}
			for skill, want := range tc.want {
				if _, err := os.Stat(filepath.Join(output, "claude", "skills", skill, "SKILL.md")); (err == nil) != want {
					t.Errorf("%s built = %v, want %v", skill, err == nil, want)
				}
			}
			if entry, _ := os.ReadFile(filepath.Join(output, "claude/skills/entry/SKILL.md")); !strings.Contains(string(entry), "henia show entry --head") {
				t.Errorf("entry is not a hybrid head:\n%s", entry)
			}
		})
	}
}

func TestBuildKeepsDependencyHarnessStrictnessToItsOwner(t *testing.T) {
	tropos, output := dependencyHarnessFixture(t, "[harness.claude.skills]\nstatic = ['ext']\n")
	if err := executeBuild(t, "--output", output); err != nil {
		t.Fatalf("other packages' unsupported frontmatter failed the owner's strict harness: %v", err)
	}
	fixture(t, filepath.Join(tropos, "skills", "pinned", "SKILL.md"), "---\nname: pinned\ndescription: Pinned skill.\nversion: 1.0.0\n---\n\n# Pinned\n")
	if err := executeBuild(t, "--output", output); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("the owner's unsupported frontmatter passed its strict harness: %v", err)
	}
}

func TestBuildAppliesProjectStrictnessToDependencies(t *testing.T) {
	_, output := dependencyHarnessFixture(t, "[harness.claude]\nstrict = true\n[harness.claude.skills]\nstatic = ['ext']\n")
	if err := executeBuild(t, "--output", output); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("unsupported frontmatter passed the project's strict table: %v", err)
	}
}

func TestBuildLayersUserAndProjectConfigOverADependencyHarness(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	dep := filepath.Join(library.PackagesDir(library.ProjectKey(project)), "tropos")
	fixture(t, filepath.Join(dep, "henia.toml"), "[harness.claude]\nartifacts = ['skills', 'config']\n"+packageHooks)
	fixture(t, filepath.Join(dep, "skills", "pinned", "SKILL.md"), "---\nname: pinned\ndescription: Pinned skill.\n---\n\n# Pinned\n")
	fixture(t, filepath.Join(config, "henia", "henia.toml"), userHooks)
	fixture(t, filepath.Join(project, "henia.toml"), `[harness.claude.config.native]
effortLevel = "low"
[harness.claude.config.native.hooks]
Stop = [{ hooks = [{ type = "command", command = "project" }] }]
`)
	output := filepath.Join(t.TempDir(), "out")
	if err := executeBuild(t, "--output", output); err != nil {
		t.Fatal(err)
	}
	settings := readJSON(t, filepath.Join(output, "claude", "settings.json"))
	if settings["model"] != "opus" || settings["effortLevel"] != "low" {
		t.Fatalf("scalars: %v", settings)
	}
	for event, want := range map[string][]string{
		"PreToolUse": {"fas", "peer", "limen"},
		"Stop":       {"limen", "project"},
	} {
		if got := hookCommands(t, settings, event); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s hooks = %v, want %v", event, got, want)
		}
	}
}
