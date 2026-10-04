package config

import (
	"slices"
	"strings"
	"testing"
)

func TestDeprecatedKeysMigrate(t *testing.T) {
	cfg, err := decodeLayers(t.TempDir(), t.TempDir(), nil, []byte(`[harness.claude]
profile = "claude"
format = "directives"
exclude = ["debate"]
keys = { model = "model_preference" }
values = { model_preference = { strong = "opus" } }

[harness.claude.references.skill]
output = "/{{.Name}}"

[harness.claude.artifact_mappings.agents]
profile = "claude-agent"
structure = "flat"
`))
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Harness["claude"]
	switch {
	case h.Directives != "keep":
		t.Errorf("directives = %q", h.Directives)
	case !slices.Equal(h.Skills.Dynamic, []string{"debate"}) || !slices.Equal(h.Commands.Dynamic, []string{"debate"}):
		t.Errorf("dynamic = %v %v", h.Skills.Dynamic, h.Commands.Dynamic)
	case h.Frontmatter.Rename["model"] != "model_preference" || h.Frontmatter.Values["model_preference"]["strong"] != "opus":
		t.Errorf("frontmatter = %+v", h.Frontmatter)
	case h.References["skill"] != "/{{.Name}}":
		t.Errorf("references = %v", h.References)
	case h.Agents.Profile != "claude-agent" || h.Agents.Layout != "flat":
		t.Errorf("agents = %+v", h.Agents)
	}
	for _, want := range []string{
		"harness.claude: format is deprecated; use directives",
		"harness.claude: exclude is deprecated; use skills.dynamic (and agents.dynamic or commands.dynamic)",
		"harness.claude: keys is deprecated; use frontmatter.rename",
		"harness.claude: references.skill.output is deprecated; use references.skill",
		"harness.claude: artifact_mappings.agents.structure is deprecated; use agents.layout",
	} {
		if !slices.Contains(cfg.Warnings, want) {
			t.Errorf("missing warning %q in %q", want, cfg.Warnings)
		}
	}
}

func TestHarnessConfigErrors(t *testing.T) {
	for _, c := range []struct{ name, config, want string }{
		{"old and new key", "[harness.claude]\nexclude = [\"a\"]\n[harness.claude.skills]\ndynamic = [\"b\"]\n", "exclude and its replacement skills.dynamic are both set"},
		{"static and dynamic", "[harness.claude.skills]\nstatic = [\"a\"]\ndynamic = [\"b\"]\n", "skills.static and skills.dynamic are both set"},
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

func TestHarnessesDecodesSourceConfig(t *testing.T) {
	harnesses, warnings, err := Harnesses([]byte("[harness.pi]\ninclude = [\"code\"]\n[harness.pi.references]\nskill = \"/skill:{{.Name}}\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if h := harnesses["pi"]; !slices.Equal(h.Skills.Static, []string{"code"}) || h.References["skill"] != "/skill:{{.Name}}" || len(warnings) != 1 {
		t.Fatalf("%+v %q", h, warnings)
	}
}
