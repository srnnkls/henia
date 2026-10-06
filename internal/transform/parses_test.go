package transform

import (
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
)

func TestTransformParseCount(t *testing.T) {
	const parsesPerSkill = 8
	tr := Transformer{
		OutputFormat: "xml",
		References:   map[string]ReferenceConfig{"skill": {Output: "/{{.Name}}"}},
		LibraryLinks: true,
	}
	art := &artifact.Artifact{Name: "demo", Type: artifact.TypeSkill, Body: "# Demo\n\n:::static\nUse `$review`.\n:::\n\nSee [the guide](guide.md) and :slot[code.style].\n\n:::note\nInline :term[`$git`] here.\n:::\n"}
	if _, err := tr.Transform(art); err != nil {
		t.Fatal(err)
	}
	before := markup.Parses()
	if _, err := tr.Transform(art); err != nil {
		t.Fatal(err)
	}
	if got := markup.Parses() - before; got != parsesPerSkill {
		t.Fatalf("one skill render ran %d parses, want %d", got, parsesPerSkill)
	}
}
