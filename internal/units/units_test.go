package units

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func value(t *testing.T, format, data string) map[string]any {
	t.Helper()
	doc, err := Decode(format, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func contents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUnownedKeysSurvive(t *testing.T) {
	path := setup(t, "claude.json", `{"numStartups": 9007199254740993, "projects": {"/x": {"trust": true}}, "theme": "dark"}`)
	file := File{Path: path, Format: "json", Entries: []string{"mcpServers"}, Value: value(t, "json", `{"mcpServers": {"gh": {"command": "gh-mcp"}}}`)}
	record, _, err := Apply([]File{file}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	got := contents(t, path)
	for _, want := range []string{`"numStartups": 9007199254740993`, `"/x"`, `"theme": "dark"`, `"gh"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed to %v", info.Mode())
	}
	doc := value(t, "json", got)
	doc["numStartups"], doc["tipsHistory"] = 10, map[string]any{"a": 1}
	doc["mcpServers"].(map[string]any)["mine"] = map[string]any{"command": "mine"}
	rewritten, _ := encode("json", doc)
	os.WriteFile(path, rewritten, 0o600)
	if _, _, err := Apply([]File{file}, record, false); err != nil {
		t.Fatalf("runtime keys and a user server conflicted: %v", err)
	}
	if !strings.Contains(contents(t, path), `"mine"`) {
		t.Fatal("user server lost")
	}
}

func TestOwnedKeyEditedConflicts(t *testing.T) {
	path := setup(t, "settings.json", "")
	file := File{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus", "env": {"A": "1"}}`)}
	record, _, err := Apply([]File{file}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte(`{"model": "sonnet", "env": {"A": "1"}, "theme": "light"}`), 0o644)
	_, _, err = Apply([]File{file}, record, false)
	var conflict *Conflict
	if !errors.As(err, &conflict) || strings.Join(conflict.Units, ",") != "model" {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(contents(t, path), "sonnet") {
		t.Fatal("conflicting apply wrote")
	}
	if _, _, err := Apply([]File{file}, record, true); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, path); !strings.Contains(got, `"opus"`) || !strings.Contains(got, `"light"`) {
		t.Fatalf("force: %s", got)
	}
}

func TestPreexistingKeyConflictsUnlessEqual(t *testing.T) {
	path := setup(t, "settings.json", `{"model": "sonnet", "env": {"A": "1"}}`)
	file := File{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus", "env": {"A": "1"}}`)}
	if _, _, err := Apply([]File{file}, nil, false); err == nil || !strings.Contains(err.Error(), "model") || strings.Contains(err.Error(), "env") {
		t.Fatalf("got %v", err)
	}
}

func TestDroppedUnitsAreRemoved(t *testing.T) {
	path := setup(t, "settings.json", "")
	record, _, err := Apply([]File{{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus", "env": {"A": "1"}}`)}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if record, _, err = Apply([]File{{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus"}`)}}, record, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(contents(t, path), "env") {
		t.Fatal("dropped key kept")
	}
	os.WriteFile(path, []byte(`{"model": "haiku"}`), 0o644)
	if _, _, err := Apply(nil, record, false); err == nil {
		t.Fatal("removed an edited key")
	}
	os.WriteFile(path, []byte(`{"model": "opus", "theme": "x"}`), 0o644)
	if record, _, err = Apply(nil, record, false); err != nil || len(record) != 0 {
		t.Fatal(err, record)
	}
	if got := contents(t, path); strings.Contains(got, "model") || !strings.Contains(got, "theme") {
		t.Fatalf("uninstall: %s", got)
	}
}

func TestArrayEntries(t *testing.T) {
	path := setup(t, "settings.json", `{"permissions": {"allow": ["Bash(ls)"], "defaultMode": "plan"}}`)
	entries := []string{"permissions.allow"}
	file := File{Path: path, Format: "json", Entries: entries, Value: value(t, "json", `{"permissions": {"allow": ["Read", "Grep"]}}`)}
	record, _, err := Apply([]File{file}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	doc := value(t, "json", contents(t, path))
	permissions := doc["permissions"].(map[string]any)
	permissions["allow"] = append(permissions["allow"].([]any), "WebFetch")
	rewritten, _ := encode("json", doc)
	os.WriteFile(path, rewritten, 0o644)
	file.Value = value(t, "json", `{"permissions": {"allow": ["Read"]}}`)
	if _, _, err := Apply([]File{file}, record, false); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strings.Fields(contents(t, path)), "")
	if !strings.Contains(got, `"allow":["Bash(ls)","Read","WebFetch"]`) || !strings.Contains(got, `"defaultMode":"plan"`) {
		t.Fatalf("%s", got)
	}
}

func TestTomlCommentsBlockUnlessForced(t *testing.T) {
	path := setup(t, "config.toml", "# mine\nmodel = \"gpt\"\n\n[projects.\"/x\"]\ntrust_level = \"trusted\"\n")
	file := File{Path: path, Format: "toml", Entries: []string{"mcp_servers"}, Value: value(t, "toml", "[mcp_servers.gh]\ncommand = \"gh-mcp\"\n")}
	if _, _, err := Apply([]File{file}, nil, false); err == nil || !strings.Contains(err.Error(), "comments") {
		t.Fatalf("got %v", err)
	}
	if _, _, err := Apply([]File{file}, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, path); !strings.Contains(got, "trust_level = 'trusted'") || !strings.Contains(got, "gh-mcp") {
		t.Fatalf("%s", got)
	}
}

func TestUnchangedFileIsNotRewritten(t *testing.T) {
	path := setup(t, "settings.json", "{\"model\":\"opus\"}")
	file := File{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus"}`)}
	if _, _, err := Apply([]File{file}, nil, false); err != nil {
		t.Fatal(err)
	}
	if contents(t, path) != "{\"model\":\"opus\"}" {
		t.Fatal("an unchanged file was rewritten")
	}
}

func TestValidateWarnsOnMergedResult(t *testing.T) {
	path := setup(t, "settings.json", `{"bogus": true}`)
	file := File{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus"}`), Validate: func(doc map[string]any) error {
		if _, ok := doc["bogus"]; ok {
			return errors.New("bogus is not allowed")
		}
		return nil
	}}
	_, warnings, err := Apply([]File{file}, nil, false)
	if err != nil || len(warnings) != 1 {
		t.Fatal(err, warnings)
	}
}

func TestYamlMerge(t *testing.T) {
	path := setup(t, "config.yml", "theme: dark\nmodelRoles:\n  default: opus\n")
	file := File{Path: path, Format: "yaml", Value: value(t, "yaml", "retry:\n  maxRetries: 3\n")}
	record, _, err := Apply([]File{file}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	got := contents(t, path)
	if !strings.Contains(got, "theme: dark") || !strings.Contains(got, "maxRetries: 3") || !strings.Contains(got, "default: opus") {
		t.Fatalf("%s", got)
	}
	os.WriteFile(path, []byte("# mine\n"+got), 0o644)
	if _, _, err := Apply([]File{file}, record, false); err == nil || !strings.Contains(err.Error(), "comments") {
		t.Fatalf("yaml comments not protected: %v", err)
	}
}

func TestDefaultsFillOnlyMissingLeaves(t *testing.T) {
	path := setup(t, "settings.json", `{"effortLevel": "high", "modelSettings": {"opus": {"effortLevel": "high"}}}`)
	defaults := value(t, "json", `{"effortLevel": "medium", "theme": "auto", "modelSettings": {"opus": {"effortLevel": "medium"}, "fable": {"effortLevel": "low"}}}`)
	file := File{Path: path, Format: "json", Defaults: defaults}
	record, _, err := Apply([]File{file}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(record) != 0 {
		t.Fatalf("defaults were recorded: %v", record)
	}
	doc := value(t, "json", contents(t, path))
	if doc["effortLevel"] != "high" || doc["theme"] != "auto" {
		t.Fatalf("top-level defaults misapplied: %v", doc)
	}
	models := doc["modelSettings"].(map[string]any)
	if models["opus"].(map[string]any)["effortLevel"] != "high" || models["fable"].(map[string]any)["effortLevel"] != "low" {
		t.Fatalf("nested defaults misapplied: %v", models)
	}
	os.WriteFile(path, []byte(`{"theme": "dark"}`), 0o644)
	if _, _, err := Apply([]File{file}, record, false); err != nil {
		t.Fatalf("an edited default conflicted: %v", err)
	}
	if value(t, "json", contents(t, path))["theme"] != "dark" {
		t.Fatal("an edited default was overwritten")
	}
}

func TestForceOverwritesDefaults(t *testing.T) {
	path := setup(t, "settings.json", `{"theme": "dark", "modelSettings": {"opus": {"effortLevel": "high"}}, "verbose": true}`)
	file := File{Path: path, Format: "json", Defaults: value(t, "json", `{"theme": "auto", "modelSettings": {"opus": {"effortLevel": "medium"}}}`)}
	if _, _, err := Apply([]File{file}, nil, true); err != nil {
		t.Fatal(err)
	}
	doc := value(t, "json", contents(t, path))
	if doc["theme"] != "auto" || doc["modelSettings"].(map[string]any)["opus"].(map[string]any)["effortLevel"] != "medium" || doc["verbose"] != true {
		t.Fatalf("force did not reset defaults alone: %v", doc)
	}
}

func TestOwnedConfigWinsOverDefaults(t *testing.T) {
	path := setup(t, "settings.json", "")
	file := File{Path: path, Format: "json", Value: value(t, "json", `{"env": {"A": "1"}}`), Defaults: value(t, "json", `{"env": {"A": "2", "B": "2"}}`)}
	for _, force := range []bool{false, true} {
		if _, _, err := Apply([]File{file}, nil, force); err != nil {
			t.Fatal(err)
		}
		if got := value(t, "json", contents(t, path))["env"]; got.(map[string]any)["A"] != "1" {
			t.Fatalf("force=%v: default replaced an owned value: %v", force, got)
		}
	}
}

func TestUnitMovedToDefaultsIsReleased(t *testing.T) {
	path := setup(t, "settings.json", "")
	owned := File{Path: path, Format: "json", Value: value(t, "json", `{"model": "opus", "env": {"A": "1"}}`)}
	record, _, err := Apply([]File{owned}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte(`{"model": "sonnet", "env": {"A": "1"}}`), 0o644)
	moved := File{Path: path, Format: "json", Value: value(t, "json", `{"env": {"A": "1"}}`), Defaults: value(t, "json", `{"model": "opus"}`)}
	record, _, err = Apply([]File{moved}, record, false)
	if err != nil {
		t.Fatalf("a unit moved to defaults conflicted: %v", err)
	}
	if value(t, "json", contents(t, path))["model"] != "sonnet" {
		t.Fatal("a released unit was changed")
	}
	if _, ok := record[path][`{"path":["model"]}`]; ok {
		t.Fatal("a released unit stayed recorded")
	}
}

func TestDefaultsCreateMissingFile(t *testing.T) {
	path := setup(t, "settings.json", "")
	if _, _, err := Apply([]File{{Path: path, Format: "json", Defaults: map[string]any{"theme": "auto"}}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if value(t, "json", contents(t, path))["theme"] != "auto" {
		t.Fatal("defaults did not create the file")
	}
}
