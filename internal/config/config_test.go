package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "henia.toml")
	if err := os.WriteFile(path, []byte("[build]\noutput='dist'\n[harness.claude]\nprofile='claude'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Harness) != 1 || cfg.Harness["claude"].Profile != "claude" || cfg.Build.Output != filepath.Join(root, "dist") {
		t.Fatalf("%+v", cfg)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("accepted missing configuration")
	}
}

func TestBuildOnlyConfigKeepsProfiles(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte("[build]\noutput='dist'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Harness) != 11 {
		t.Fatalf("build setting disabled profiles: %+v", cfg.Harness)
	}
}

func TestFindRejectsTwoProjectConfigs(t *testing.T) {
	dir := t.TempDir()
	if _, err := Find(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Find without config = %v; want ErrNotExist", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".henia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".henia", "henia.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Find(dir); err != nil || got != filepath.Join(dir, ".henia", "henia.toml") {
		t.Fatalf("Find = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "henia.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(dir); err == nil || !strings.Contains(err.Error(), "keep one") {
		t.Fatalf("Find with both configs = %v; want an error", err)
	}
}

func TestSlotOptions(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), []byte("[slots.\"code.style\"]\ndisable = [\"loqui:loqui\"]\n"), []byte("[slots.\"implement.approach\"]\nprovide = [\"ponytail:ponytail\", \"tropos:loqui#tdd\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.DisabledProviders(); !reflect.DeepEqual(got, map[string][]string{"code.style": {"loqui:loqui"}}) {
		t.Errorf("disabled = %v", got)
	}
	if got := cfg.ProvidedSkills(); !reflect.DeepEqual(got, map[string][]string{"implement.approach": {"ponytail:ponytail", "tropos:loqui#tdd"}}) {
		t.Errorf("provided = %v", got)
	}
}

func TestLinksResourcesLayersTheLocalFileOverThePackage(t *testing.T) {
	user := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", user)
	if err := os.MkdirAll(filepath.Join(user, "henia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, "henia", "henia.toml"), []byte("[resources]\nlink = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "henia.toml")
	for _, tc := range []struct {
		committed, local string
		want             bool
	}{
		{"", "", false},
		{"[resources]\nlink = true\n", "", true},
		{"[resources]\nlink = true\n", "[resources]\nlink = false\n", false},
		{"", "[resources]\nlink = true\n", true},
	} {
		for path, content := range map[string]string{manifest: tc.committed, filepath.Join(dir, "henia.local.toml"): tc.local} {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := LinksResources(manifest); err != nil || got != tc.want {
			t.Errorf("committed %q, local %q: link = %v (%v), want %v", tc.committed, tc.local, got, err, tc.want)
		}
	}
}
