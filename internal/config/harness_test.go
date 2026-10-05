package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarnessConfigErrors(t *testing.T) {
	for _, c := range []struct{ name, config, want string }{
		{"retired key", "[harness.claude]\nexclude = [\"a\"]\n", "exclude"},
		{"retired table", "[harness.claude.artifact_mappings.agents]\nprofile = \"x\"\n", "artifact_mappings"},
		{"retired sources", "[sources]\nx = 1\n", "sources"},
		{"two modes", "[harness.claude.skills]\nstatic = [\"a\"]\nhybrid = [\"a\"]\n", `skills "a" is listed as both static and hybrid`},
		{"unknown default", "[harness.claude.skills]\ndefault = \"lazy\"\n", `skills.default must be static, dynamic or hybrid, got "lazy"`},
		{"hybrid agents", "[harness.claude.agents]\nhybrid = [\"a\"]\n", "hybrid applies to skills only"},
		{"unknown directives", "[harness.claude]\ndirectives = \"html\"\n", "directives must be xml, markdown, md or keep"},
		{"unknown layout", "[harness.claude.agents]\nlayout = \"deep\"\n", "agents.layout must be flat or nested"},
		{"catalog outside skills", "[harness.claude.agents]\ncatalog = false\n", "catalog applies to skills only"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte(c.config))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestExplicitHarnessKeepsItsPreset(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte("[harness.claude]\nstrict = true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if h := cfg.Harness["claude"]; h.Profile != "claude" || h.Directives != "xml" || !h.Strict {
		t.Fatalf("%+v", h)
	}
}

func TestHarnessesRejectsUnknownKeys(t *testing.T) {
	if _, err := Harnesses([]byte("[build]\noutput = \"x\"\n[harness.pi]\ninclude = [\"a\"]\n")); err == nil {
		t.Fatal("accepted a retired key")
	}
	harnesses, err := Harnesses([]byte("[build]\noutput = \"x\"\n[harness.pi.skills]\nstatic = [\"a\"]\n"))
	if err != nil || len(harnesses["pi"].Skills.Static) != 1 {
		t.Fatalf("%v %v", harnesses, err)
	}
}
func TestInstallHeniaSkill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "henia.toml")
	for _, c := range []struct {
		toml string
		want bool
	}{{"[install.claude]\npath = \"~/.claude\"\n", true}, {"install_henia_skill = false\n", false}} {
		if err := os.WriteFile(path, []byte(c.toml), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil || cfg.HeniaSkill() != c.want {
			t.Fatalf("%q: %v, %v", c.toml, cfg.HeniaSkill(), err)
		}
	}
}
