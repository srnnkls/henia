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

func TestTransitivePassesThrough(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "henia.toml")
	if err := Add(manifest, "tropos", Dependency{Git: "https://example.test/tropos.git", Transitive: true}); err != nil {
		t.Fatal(err)
	}
	declared, err := Read(manifest)
	if err != nil || !declared["tropos"].Transitive {
		t.Fatalf("tropos = %+v (%v)", declared["tropos"], err)
	}
	generated, err := phoraManifest(declared, Scope{State: dir, Store: filepath.Join(dir, "store")})
	if err != nil || !strings.Contains(string(generated), "[sources.tropos]\ngit = \"https://example.test/tropos.git\"\ntransitive = true\n") {
		t.Fatalf("manifest:\n%s (%v)", generated, err)
	}
	for _, d := range []Dependency{{Git: "https://example.test/tropos.git", Transitive: true, Skills: []string{"git"}}, {Git: "https://example.test/tropos.git", Transitive: true, Root: "pkg"}} {
		if err := d.validate("tropos"); err == nil {
			t.Errorf("%+v validates", d)
		}
	}
}

func TestForgeShorthands(t *testing.T) {
	for shorthand, want := range map[string][3]string{
		"srnnkls/loqui":                  {"", "srnnkls/loqui", ""},
		"github:srnnkls/tropos":          {"github", "srnnkls/tropos", ""},
		"gitlab:group/repo/sub/pkg":      {"gitlab", "group/repo", "sub/pkg"},
		"codeberg.org/me/dotfiles":       {"codeberg", "me/dotfiles", ""},
		"github.com/srnnkls/loqui/skill": {"github", "srnnkls/loqui", "skill"},
	} {
		host, repo, root, err := Forge(shorthand)
		if err != nil || [3]string{host, repo, root} != want {
			t.Errorf("Forge(%q) = %q %q %q (%v), want %q", shorthand, host, repo, root, err, want)
		}
	}
	for _, bad := range []string{"https://github.com/me/x.git", "git@github.com:me/x.git", "loqui", ":me/x", "github.com/me"} {
		if _, _, _, err := Forge(bad); err == nil {
			t.Errorf("Forge(%q) accepted", bad)
		}
	}
}

func TestForgeDependencies(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "henia.toml")
	if err := Add(manifest, "loqui", Dependency{Host: "github", Repo: "srnnkls/loqui", Branch: "main", Skills: []string{"loqui"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(manifest)
	if want := "[dependencies.loqui]\nhost = \"github\"\nrepo = \"srnnkls/loqui\"\nbranch = \"main\"\nskills = [\"loqui\"]\n"; err != nil || string(data) != want {
		t.Fatalf("manifest:\n%s (%v)\nwant:\n%s", data, err, want)
	}
	declared, err := Read(manifest)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := phoraManifest(declared, Scope{State: dir, Store: filepath.Join(dir, "store")})
	if err != nil || !strings.Contains(string(generated), "[sources.loqui]\nhost = \"github\"\nrepo = \"srnnkls/loqui\"\nbranch = \"main\"\n") {
		t.Fatalf("phora.toml:\n%s (%v)", generated, err)
	}
	for _, d := range []Dependency{
		{Repo: "srnnkls/loqui", Git: "https://github.com/srnnkls/loqui.git"},
		{Repo: "srnnkls/loqui", Path: "/tmp/loqui"},
		{Host: "gitlab", Git: "https://gitlab.com/x/y.git"},
		{Repo: "https://github.com/srnnkls/loqui"},
		{Repo: "loqui"},
	} {
		if err := d.validate("loqui"); err == nil {
			t.Errorf("%+v validates", d)
		}
	}
}
