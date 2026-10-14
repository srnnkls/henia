package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/library"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func TestProjectInstallConfig(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	fixture(t, filepath.Join(project, ".henia", "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill.\n---\n\n# Own\n")
	fixture(t, filepath.Join(project, "line.sh"), "echo hi\n")
	fixture(t, filepath.Join(project, "henia.toml"), `artifacts = ["skills", "config"]
[shared.mcp.github]
command = "gh-mcp"
[harness.claude.config.statusline]
file = "line.sh"
[harness.claude.config.native]
permissions = { allow = ["Read"] }
`)
	settings := filepath.Join(project, ".claude", "settings.json")
	fixture(t, settings, `{"theme": "dark", "permissions": {"allow": ["Bash(ls)"]}}`)
	install := func(args ...string) error {
		cmd, out := installCommand(t, append([]string{"install", "--harness", "claude"}, args...)...)
		err := cmd.Execute()
		if err != nil {
			t.Logf("%s", out)
		}
		return err
	}
	if err := install(); err != nil {
		t.Fatal(err)
	}
	doc := readJSON(t, settings)
	allow := doc["permissions"].(map[string]any)["allow"].([]any)
	command := doc["statusLine"].(map[string]any)["command"].(string)
	if doc["theme"] != "dark" || len(allow) != 2 || !strings.Contains(command, "CLAUDE_PROJECT_DIR") || !strings.Contains(command, "/.claude/statusline.sh") {
		t.Fatalf("settings: %v", doc)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); err != nil {
		t.Fatalf("project MCP not at the project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("MCP installed into .claude")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude.json")); !os.IsNotExist(err) {
		t.Fatal("a project install wrote the home config")
	}

	doc["statusLine"].(map[string]any)["padding"] = 3
	data, _ := json.Marshal(doc)
	os.WriteFile(settings, data, 0o644)
	fixture(t, filepath.Join(project, ".henia", "skills", "second", "SKILL.md"), "---\nname: second\ndescription: Second skill.\n---\n\n# Second\n")
	if err := install(); err == nil || !strings.Contains(err.Error(), "statusLine") {
		t.Fatalf("an edited owned key installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "second")); !os.IsNotExist(err) {
		t.Fatal("a conflict left a partial install")
	}
	if err := install("--force"); err != nil {
		t.Fatal(err)
	}
	if _, ok := readJSON(t, settings)["statusLine"].(map[string]any)["padding"]; ok {
		t.Fatal("--force kept the edited value")
	}

	fixture(t, filepath.Join(project, "henia.toml"), "[install.claude]\nenabled = false\n")
	if err := install(); err != nil {
		t.Fatal(err)
	}
	doc = readJSON(t, settings)
	if _, ok := doc["statusLine"]; ok || doc["theme"] != "dark" {
		t.Fatalf("uninstall: %v", doc)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("uninstall kept the project MCP file")
	}
}

func TestGlobalInstallConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	home := t.TempDir()
	t.Setenv("HOME", home)
	claude := filepath.Join(home, ".claude")
	global := library.PackagesDir("global")
	fixture(t, filepath.Join(global, "tropos", "henia.toml"), `artifacts = ["skills", "config"]
[shared.mcp.github]
command = "gh-mcp"
[harness.claude.config.statusline]
command = "starship statusline"
`)
	fixture(t, filepath.Join(global, "tropos", "skills", "pinned", "SKILL.md"), "---\nname: pinned\ndescription: Pinned skill.\n---\n\n# Pinned\n")
	fixture(t, filepath.Join(config, "henia", "henia.toml"), "install_henia_skill = false\n[install.claude]\npath = '"+claude+"'\n")
	fixture(t, filepath.Join(home, ".claude.json"), `{"numStartups": 9007199254740993, "mcpServers": {"mine": {"command": "mine"}}}`)
	fixture(t, filepath.Join(claude, "settings.json"), `{"model": "opus"}`)
	cmd, out := installCommand(t, "install", "--global", "--harness", "claude")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	state, _ := os.ReadFile(filepath.Join(home, ".claude.json"))
	if !strings.Contains(string(state), "9007199254740993") || !strings.Contains(string(state), `"mine"`) || !strings.Contains(string(state), `"github"`) {
		t.Fatalf("home config: %s", state)
	}
	settings := readJSON(t, filepath.Join(claude, "settings.json"))
	if settings["model"] != "opus" || settings["statusLine"].(map[string]any)["command"] != "starship statusline" {
		t.Fatalf("settings: %v", settings)
	}
	if _, err := os.Stat(filepath.Join(claude, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("a global install wrote a project MCP file")
	}
}
