package config

import (
	"os"
	"path/filepath"
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
	if len(cfg.Harness) != 9 {
		t.Fatalf("build setting disabled profiles: %+v", cfg.Harness)
	}
}

func TestRejectDeploymentSettings(t *testing.T) {
	for _, input := range []string{"[sources.team]\nrepo='owner/repo'\n", "[harness.claude]\npath='.claude'\n"} {
		_, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte(input))
		if err == nil || !strings.Contains(err.Error(), "Phora") {
			t.Fatalf("want actionable migration error: %v", err)
		}
	}
}
