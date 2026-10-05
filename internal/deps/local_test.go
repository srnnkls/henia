package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalOverridesAndLink(t *testing.T) {
	if got := Local("/p/henia.toml"); got != "/p/henia.local.toml" {
		t.Fatalf("Local = %s", got)
	}
	if got := Local("/p/henia.lock"); got != "/p/henia.local.lock" {
		t.Fatalf("Local = %s", got)
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	manifest := filepath.Join(dir, "henia.local.toml")
	if err := os.WriteFile(manifest, []byte("[dependencies.loqui]\npath = \"~/projects/loqui\"\nlink = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	declared, err := Read(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if d := declared["loqui"]; d.Path != filepath.Join(dir, "projects", "loqui") || !d.Link {
		t.Fatalf("loqui = %+v", d)
	}
	manifest2 := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(manifest2, []byte("[dependencies.x]\ngit = \"https://example.test/x.git\"\nlink = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(manifest2); err == nil || !strings.Contains(err.Error(), "needs path") {
		t.Fatalf("link without path: %v", err)
	}
	generated, err := phoraManifest(declared, Scope{State: dir, Store: filepath.Join(dir, "store")})
	if err != nil || !strings.Contains(string(generated), "deploy = \"link\"\n") {
		t.Fatalf("manifest:\n%s (%v)", generated, err)
	}
}

func TestMergeLockKeepsOverriddenEntries(t *testing.T) {
	dir := t.TempDir()
	fresh, tracked := filepath.Join(dir, "phora.lock"), filepath.Join(dir, "henia.lock")
	original := "version = 1\n\n[[sources]]\nname = \"a\"\ncommit = \"1\"\n\n[[sources]]\nname = \"loqui\"\ncommit = \"2\"\n"
	if err := os.WriteFile(tracked, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte("version = 1\n\n[[sources]]\nname = \"a\"\ncommit = \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overrides := map[string]Dependency{"loqui": {Path: "/live", Link: true}}
	if err := mergeLock(fresh, tracked, overrides); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(tracked); string(data) != original {
		t.Fatalf("unchanged lock rewritten:\n%s", data)
	}
	if err := os.WriteFile(fresh, []byte("version = 1\n\n[[sources]]\nname = \"a\"\ncommit = \"3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := mergeLock(fresh, tracked, overrides); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(tracked)
	if !strings.Contains(string(data), "commit = '3'") || !strings.Contains(string(data), "name = 'loqui'") {
		t.Fatalf("merged lock:\n%s", data)
	}
}
