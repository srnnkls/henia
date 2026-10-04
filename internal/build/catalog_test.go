package build

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
)

func TestCatalog(t *testing.T) {
	skill := func(name, description string) *artifact.Artifact {
		return &artifact.Artifact{Name: name, Type: artifact.TypeSkill, Frontmatter: map[string]any{"description": description}}
	}
	code, debate, loqui := skill("code", "Code."), skill("debate", "Debates\n  across lines."), skill("loqui", "Guides.")
	agent := &artifact.Artifact{Name: "reviewer", Type: artifact.TypeAgent}
	all := []*artifact.Artifact{code, debate, loqui, agent}
	off := false

	index, err := catalog("/src", all, []*artifact.Artifact{code, agent}, henia.Harness{Profile: "claude"})
	if err != nil || index == nil {
		t.Fatalf("index=%v err=%v", index, err)
	}
	if !strings.Contains(index.Body, "- `debate`: Debates across lines.\n- `loqui`: Guides.\n") || strings.Contains(index.Body, "`code`") || strings.Contains(index.Body, "reviewer") {
		t.Fatalf("body:\n%s", index.Body)
	}
	if index.Name != "henia" || index.Frontmatter["allowed-tools"] != "Bash(henia ls *), Bash(henia show *)" {
		t.Fatalf("frontmatter: %v", index.Frontmatter)
	}
	if index, _ := catalog("/src", all, []*artifact.Artifact{code}, henia.Harness{Profile: "codex"}); index.Frontmatter["allowed-tools"] != nil {
		t.Fatal("allowed-tools set for a profile without the field")
	}
	if index, _ := catalog("/src", all, all, henia.Harness{}); index != nil {
		t.Fatal("catalog built without dynamic skills")
	}
	if index, _ := catalog("/src", all, []*artifact.Artifact{code}, henia.Harness{Skills: henia.Artifacts{Catalog: &off}}); index != nil {
		t.Fatal("catalog built while turned off")
	}
	if index, _ := catalog("/src", all, []*artifact.Artifact{code}, henia.Harness{Artifacts: []string{"agents"}}); index != nil {
		t.Fatal("catalog built for a harness without skills")
	}
	clash := skill("henia", "Mine.")
	if _, err := catalog("/src", append(all, clash), []*artifact.Artifact{code, clash}, henia.Harness{}); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("collision: %v", err)
	}
}
