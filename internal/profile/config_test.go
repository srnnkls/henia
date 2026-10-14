package profile

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func renderConfig(t *testing.T, config Config, shared, native map[string]any) *Result {
	t.Helper()
	compiler, err := NewConfigCompiler(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := compiler.Render(shared, nil, native, Context{Name: "test"}, &Schemas{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestConfigNativeArrays(t *testing.T) {
	native := map[string]any{"list": []any{"native"}}
	for arrays, want := range map[string]string{"": `["native"]`, "replace": `["native"]`, "concat": `["generated","native"]`} {
		config := Config{Files: map[string]ConfigFile{MainFile: {Path: "c.json", Format: "json", Arrays: arrays, Value: `{list: ["generated"]}`}}}
		got := strings.Join(strings.Fields(string(renderConfig(t, config, nil, native).Files["c.json"])), "")
		if !strings.Contains(got, `"list":`+want) {
			t.Errorf("arrays %q: %s", arrays, got)
		}
	}
}

func TestConfigOmitsEmptyFiles(t *testing.T) {
	config := Config{Files: map[string]ConfigFile{
		"mcp":    {Path: "mcp.json", Format: "json", Value: `{mcpServers: shared.mcp}`},
		MainFile: {Path: "settings.json", Format: "json"},
	}}
	if files := renderConfig(t, config, nil, nil).Files; len(files) != 0 {
		t.Fatalf("emitted %v", files)
	}
	files := renderConfig(t, config, nil, map[string]any{"model": "opus"}).Files
	if _, ok := files["settings.json"]; !ok || len(files) != 1 {
		t.Fatalf("native data did not fill the main file: %v", files)
	}
}

func TestConfigNativeAddressesFilesByID(t *testing.T) {
	config := Config{Files: map[string]ConfigFile{
		MainFile: {Path: "main.yaml", Format: "yaml"},
		"models": {Path: "models.yaml", Format: "yaml"},
	}}
	files := renderConfig(t, config, nil, map[string]any{"theme": "dark", "models": map[string]any{"fast": "small"}}).Files
	if string(files["main.yaml"]) != "theme: dark\n" || string(files["models.yaml"]) != "fast: small\n" {
		t.Fatalf("%q", files)
	}
	compiler, err := NewConfigCompiler(Config{Files: map[string]ConfigFile{"mcp": {Path: "mcp.json", Format: "json"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.Render(nil, nil, map[string]any{"theme": "dark"}, Context{}, nil); err == nil {
		t.Fatal("accepted native keys without a main file")
	}
}

func TestConfigFormatsAreDeterministic(t *testing.T) {
	value := `{zeta: 1, alpha: {beta: true, aleph: "x"}, list: [2, 1]}`
	want := map[string]string{
		"json": "{\n  \"$schema\": \"https://example.com/s.json\",\n  \"alpha\": {\n    \"aleph\": \"x\",\n    \"beta\": true\n  },\n  \"list\": [\n    2,\n    1\n  ],\n  \"zeta\": 1\n}\n",
		"yaml": "# yaml-language-server: $schema=https://example.com/s.json\nalpha:\n    aleph: x\n    beta: true\nlist:\n    - 2\n    - 1\nzeta: 1\n",
		"toml": "#:schema https://example.com/s.json\n\nlist = [2, 1]\nzeta = 1\n\n[alpha]\naleph = 'x'\nbeta = true\n",
	}
	for format, expected := range want {
		file := ConfigFile{Path: "c." + format, Format: format, Value: value, Schema: "https://example.com/s.json"}
		for range 5 {
			compiler, err := NewConfigCompiler(Config{Files: map[string]ConfigFile{MainFile: file}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := compiler.Render(nil, nil, nil, Context{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(result.Files[file.Path]); got != expected {
				t.Fatalf("%s:\n%s", format, got)
			}
		}
	}
}

func TestConfigRejectsInvalidDefinitions(t *testing.T) {
	for _, files := range []map[string]ConfigFile{
		{"a": {Path: "c.ini", Format: "ini"}},
		{"a": {Path: "c.json", Format: "json", Arrays: "merge"}},
		{"a": {Path: "../c.json", Format: "json"}},
		{"a": {Path: "c.json", Format: "json"}, "b": {Path: "c.json", Format: "json"}},
		{"a": {Path: "c.json", Format: "json", Value: "input.name"}},
	} {
		if _, err := NewConfigCompiler(Config{Files: files}); err == nil {
			t.Errorf("accepted %v", files)
		}
	}
	compiler, err := NewConfigCompiler(Config{Files: map[string]ConfigFile{MainFile: {Path: "c.txt", Format: "text", Value: `"x"`}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiler.Render(nil, nil, map[string]any{"a": 1}, Context{}, nil); err == nil {
		t.Error("text file accepted native data")
	}
}

func TestBundledConfigSections(t *testing.T) {
	shared := map[string]any{"mcp": map[string]any{
		"github": map[string]any{"command": "gh-mcp", "env": map[string]any{"GITHUB_TOKEN": "$GITHUB_TOKEN", "LOG_LEVEL": "debug"}},
		"docs":   map[string]any{"url": "https://docs.example.com/mcp"},
	}}
	render := func(name string, shared, native map[string]any) (*Result, error) {
		h, err := Load(name, t.TempDir(), "")
		if err != nil {
			t.Fatal(err)
		}
		compiler, err := NewConfigCompiler(h.Config)
		if err != nil {
			t.Fatal(err)
		}
		return compiler.Render(shared, nil, native, Context{Name: name, Profile: name}, &Schemas{})
	}
	claude, err := render("claude", shared, map[string]any{"permissions": map[string]any{"allow": []any{"Read"}}})
	if err != nil {
		t.Fatal(err)
	}
	mcp := string(claude.Files[".mcp.json"])
	if !strings.Contains(mcp, `"GITHUB_TOKEN": "${GITHUB_TOKEN}"`) || !strings.Contains(mcp, `"LOG_LEVEL": "debug"`) || !strings.Contains(mcp, `"type": "http"`) || len(claude.Warnings) != 0 {
		t.Fatalf("%s %v", mcp, claude.Warnings)
	}
	codex, err := render("codex", shared, map[string]any{"approval_policy": "never"})
	if err != nil {
		t.Fatal(err)
	}
	if toml := string(codex.Files["config.toml"]); !strings.Contains(toml, "env_vars = ['GITHUB_TOKEN']") || !strings.Contains(toml, "LOG_LEVEL = 'debug'") || len(codex.Warnings) != 0 {
		t.Fatalf("%s %v", toml, codex.Warnings)
	}
	invalid, err := render("codex", nil, map[string]any{"approval_policy": "sometimes"})
	if err != nil || len(invalid.Warnings) != 1 {
		t.Fatalf("schema violation not reported: %v %v", err, invalid)
	}
	renamed := map[string]any{"mcp": map[string]any{"x": map[string]any{"command": "x", "env": map[string]any{"TOKEN": "$OTHER"}}}}
	if _, err := render("codex", renamed, nil); err == nil || !strings.Contains(err.Error(), "by name only") {
		t.Fatalf("codex accepted a renamed env reference: %v", err)
	}
}

func TestSchemaUnavailableIsReported(t *testing.T) {
	err := (&Schemas{}).Validate("https://example.com/unknown.json", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatal(err)
	}
}

func TestClaudeStatusLine(t *testing.T) {
	h, err := Load("claude", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := NewConfigCompiler(h.Config)
	if err != nil {
		t.Fatal(err)
	}
	render := func(statusline map[string]any) (string, *Result, error) {
		result, err := compiler.Render(nil, map[string]any{"statusline": statusline}, nil, Context{}, &Schemas{})
		if err != nil {
			return "", nil, err
		}
		return strings.Join(strings.Fields(string(result.Files["settings.json"])), " "), result, nil
	}
	for _, c := range []struct {
		statusline map[string]any
		want       string
	}{
		{map[string]any{"command": "starship statusline", "padding": 1}, `"command": "starship statusline", "padding": 1,`},
		{map[string]any{"script": "echo hi", "content": "#!/bin/sh\necho hi", "refresh_interval": 5}, `"command": "\"${CLAUDE_CONFIG_DIR:-$HOME/.claude}/statusline.sh\"", "refreshInterval": 5,`},
		{map[string]any{"file": "/src/line.sh", "content": "#!/bin/sh\necho hi"}, `"command": "\"${CLAUDE_CONFIG_DIR:-$HOME/.claude}/statusline.sh\"",`},
	} {
		got, result, err := render(c.statusline)
		if err != nil || !strings.Contains(got, c.want) || !strings.Contains(got, `"type": "command"`) || len(result.Warnings) > 0 {
			t.Errorf("%v: %v %s %v", c.statusline, err, got, result)
			continue
		}
		script, deployed := result.Files["statusline.sh"]
		if deployed != (c.statusline["content"] != nil) || deployed && (string(script) != c.statusline["content"] || result.Modes["statusline.sh"] != 0o755) {
			t.Errorf("%v: deployed %q %v", c.statusline, script, result.Modes)
		}
	}
	for _, bad := range []map[string]any{{}, {"command": "a", "script": "b"}} {
		if _, _, err := render(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

func TestSchemaCacheNamesKeepURLsApart(t *testing.T) {
	seen := map[string]string{}
	for _, url := range []string{"https://example.com/a/b.json", "https://example.com/a_b.json", "http://example.com/a/b.json"} {
		name := cacheName(url)
		if other, ok := seen[name]; ok {
			t.Fatalf("%s and %s share %s", url, other, name)
		}
		seen[name] = url
	}
}

func TestConfigDropsNilArrayItems(t *testing.T) {
	config := Config{Files: map[string]ConfigFile{MainFile: {Path: "c.toml", Format: "toml", Value: `{args: [shared.x, "kept"]}`}}}
	if got := string(renderConfig(t, config, nil, nil).Files["c.toml"]); got != "args = ['kept']\n" {
		t.Fatalf("%q", got)
	}
}

func TestSchemaCacheRefreshesAfterTTL(t *testing.T) {
	url := "https://example.com/schema.json"
	now := time.Now()
	fetched := 0
	body := `{"type": "object", "required": ["a"]}`
	s := &Schemas{Cache: t.TempDir(), TTL: time.Hour, Now: func() time.Time { return now }, Fetch: func(string) ([]byte, error) {
		fetched++
		return []byte(body), nil
	}}
	load := func(url string) string {
		t.Helper()
		data, err := s.load(url)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	load(url)
	load(url)
	if fetched != 1 {
		t.Fatalf("fresh cache refetched: %d", fetched)
	}
	now = now.Add(2 * time.Hour)
	body = `{"type": "object"}`
	if got := load(url); got != body || fetched != 2 {
		t.Fatalf("expired cache not refreshed: %s %d", got, fetched)
	}
	now = now.Add(2 * time.Hour)
	s.Fetch = func(string) ([]byte, error) { return nil, errors.New("offline") }
	if got := load(url); got != body {
		t.Fatalf("offline did not fall back to the stale copy: %s", got)
	}
	s.Cache = t.TempDir()
	if _, err := s.load(url); err == nil {
		t.Fatal("an unknown schema loaded offline")
	}
	claude := "https://www.schemastore.org/claude-code-settings.json"
	if !strings.Contains(load(claude), "statusLine") {
		t.Fatal("offline did not fall back to the pinned copy")
	}
	s.Fetch = func(string) ([]byte, error) { return []byte("<html>"), nil }
	if !strings.Contains(load(claude), "statusLine") {
		t.Fatal("a non-JSON response replaced the schema")
	}
}
