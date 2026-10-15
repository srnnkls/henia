package lint_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/profile"
)

func configProject(t *testing.T, toml string, harnesses map[string]henia.Harness, extra ...func(*lint.Options)) []lint.Diagnostic {
	t.Helper()
	root := t.TempDir()
	write(t, root, "skills/a/SKILL.md", skillFile("a", "", "# A\n"))
	path := write(t, root, "henia.toml", toml)
	options := lint.Options{Project: root, Configured: harnesses, Artifacts: []string{"skills", "config"}, ConfigFiles: []string{path}, Schemas: &profile.Schemas{}}
	for _, f := range extra {
		f(&options)
	}
	return withRule(lintProject(t, options, filepath.Join(root, "skills")), lint.MCPTransport, lint.MCPHeader, lint.ConfigInvalid, lint.ConfigSchema, lint.ConfigUnused)
}

func mcp(servers map[string]any) map[string]any { return map[string]any{"mcp": servers} }

func TestMCPTransportNeedsExactlyOneOfCommandAndURL(t *testing.T) {
	toml := "[shared.mcp.docs]\nargs = [\"x\"]\n\n[shared.mcp.both]\ncommand = \"srv\"\nurl = \"https://x.example/mcp\"\n\n[shared.mcp.ok]\ncommand = \"srv\"\n"
	shared := mcp(map[string]any{
		"docs": map[string]any{"args": []any{"x"}},
		"both": map[string]any{"command": "srv", "url": "https://x.example/mcp"},
		"ok":   map[string]any{"command": "srv"},
	})
	found := configProject(t, toml, map[string]henia.Harness{
		"claude": {Profile: "claude", Shared: shared},
		"codex":  {Profile: "codex", Shared: shared},
	})
	var got []string
	for _, d := range found {
		if d.Harness != "" || d.Stage != lint.StageConfig || d.Severity != "error" || filepath.Base(d.Path) != "henia.toml" {
			t.Errorf("diagnostic = %+v, want one harness-independent config-stage error in henia.toml", d)
		}
		got = append(got, fmt.Sprintf("%s@%d:%s", d.Rule, d.Line, d.Message))
	}
	want := []string{"mcp-transport@1:MCP server docs sets neither command nor url", "mcp-transport@4:MCP server both sets both command and url"}
	if !slices.Equal(got, want) {
		t.Fatalf("diagnostics = %q, want %q", got, want)
	}
}

func TestConfigChecksOnlyHarnessesThatBuildConfig(t *testing.T) {
	shared := mcp(map[string]any{"docs": map[string]any{}})
	found := configProject(t, "[shared.mcp.docs]\n", map[string]henia.Harness{"claude": {Profile: "claude", Shared: shared, Artifacts: []string{"skills"}}})
	if len(found) != 0 {
		t.Fatalf("diagnostics = %+v, want none: claude builds no config", found)
	}
}

func TestConfigRulesCanBeDisabled(t *testing.T) {
	shared := mcp(map[string]any{"docs": map[string]any{}})
	found := configProject(t, "[shared.mcp.docs]\n", map[string]henia.Harness{"claude": {Profile: "claude", Shared: shared}},
		func(o *lint.Options) { o.Disable = []string{lint.MCPTransport} })
	if len(found) != 0 {
		t.Fatalf("diagnostics = %+v, want none: mcp-transport is disabled", found)
	}
}

func TestConfigSchemaAndUnusedInputsArePerHarness(t *testing.T) {
	toml := "[harness.codex.config.statusline]\ncommand = \"echo hi\"\n\n[harness.codex.config.native.main]\napproval_policy = \"sometimes\"\n"
	found := configProject(t, toml, map[string]henia.Harness{
		"claude": {Profile: "claude"},
		"codex": {Profile: "codex", Config: henia.HarnessConfig{
			StatusLine: map[string]any{"command": "echo hi"},
			Native:     map[string]any{"main": map[string]any{"approval_policy": "sometimes"}},
		}},
	})
	lines := map[string]int{}
	for _, d := range found {
		if d.Harness != "codex" || d.Severity != "warning" {
			t.Errorf("diagnostic = %+v, want a codex warning", d)
		}
		lines[d.Rule] = d.Line
	}
	if lines[lint.ConfigSchema] != 1 || lines[lint.ConfigUnused] != 1 {
		t.Fatalf("diagnostics = %+v, want config-schema and config-unused at the codex tables", found)
	}
}

func TestConfigRenderFailureIsInvalid(t *testing.T) {
	shared := mcp(map[string]any{"docs": map[string]any{"command": "srv", "env": map[string]any{"TOKEN": "$OTHER"}}})
	found := configProject(t, "[shared.mcp.docs]\ncommand = \"srv\"\n", map[string]henia.Harness{"codex": {Profile: "codex", Shared: shared}})
	if len(found) != 1 || found[0].Rule != lint.ConfigInvalid || found[0].Severity != "error" || found[0].Harness != "" {
		t.Fatalf("diagnostics = %+v, want one config-invalid error", found)
	}
}

func TestConfigKeysLocateInPrecedenceOrder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/a/SKILL.md", skillFile("a", "", "# A\n"))
	project := write(t, root, "henia.toml", "artifacts = [\"config\"]\n")
	user := write(t, root, "user/henia.toml", "[shared]\nmcp.docs.args = [\"x\"]\n")
	options := lint.Options{Project: root, Artifacts: []string{"config"}, ConfigFiles: []string{filepath.Join(root, "henia.local.toml"), project, user}, Schemas: &profile.Schemas{},
		Configured: map[string]henia.Harness{"claude": {Profile: "claude", Shared: mcp(map[string]any{"docs": map[string]any{"args": []any{"x"}}})}}}
	found := withRule(lintProject(t, options, filepath.Join(root, "skills")), lint.MCPTransport)
	if len(found) != 1 || found[0].Path != user || found[0].Line != 2 || !strings.Contains(found[0].Message, "docs") {
		t.Fatalf("diagnostics = %+v, want docs located at %s:2", found, user)
	}
}

func TestMCPHeadersTakeOnlyFormsEveryHarnessCarries(t *testing.T) {
	toml := "[shared.mcp.docs]\nurl = \"https://docs.example/mcp\"\n\n[shared.mcp.docs.headers]\nAuthorization = \"Bearer $TOKEN\"\nX-Team = \"$TEAM\"\nX-Client = \"henia\"\nX-Key = \"key-$KEY\"\nX-Auth = \"Bearer $TOKEN\"\n\n[shared.mcp.local]\ncommand = \"srv\"\nheaders = { X-Team = \"$TEAM\" }\n"
	shared := mcp(map[string]any{
		"docs":  map[string]any{"url": "https://docs.example/mcp", "headers": map[string]any{"Authorization": "Bearer $TOKEN", "X-Team": "$TEAM", "X-Client": "henia", "X-Key": "key-$KEY", "X-Auth": "Bearer $TOKEN"}},
		"local": map[string]any{"command": "srv", "headers": map[string]any{"X-Team": "$TEAM"}},
	})
	var got []string
	for _, d := range configProject(t, toml, map[string]henia.Harness{"claude": {Profile: "claude", Shared: shared}}) {
		if d.Rule == lint.MCPHeader {
			got = append(got, fmt.Sprintf("%d:%s", d.Line, d.Message))
		}
	}
	want := []string{
		"8:MCP server docs header X-Key references a variable as neither $NAME nor Bearer $NAME",
		"9:MCP server docs header X-Auth uses Bearer $NAME outside Authorization",
		"13:MCP server local sets headers without url",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("diagnostics = %q, want %q", got, want)
	}
}
