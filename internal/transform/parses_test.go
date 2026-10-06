package transform

import (
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
)

func TestTransformParsesOnce(t *testing.T) {
	tr := Transformer{
		OutputFormat: "xml",
		References:   map[string]ReferenceConfig{"skill": {Output: "/{{.Name}}"}},
		LibraryLinks: true,
	}
	document := "# Demo\n\n:::static\nUse `$review`.\n:::\n\nSee [the guide](guide.md) and :slot[code.style].\n\n:::note\nA note.\n:::\n"
	for body, want := range map[string]int64{
		document: 1,
		document + "\nInline :term[`$git`] here.\n": 2,
	} {
		art := &artifact.Artifact{Name: "demo", Type: artifact.TypeSkill, Body: body}
		if _, err := tr.Transform(art); err != nil {
			t.Fatal(err)
		}
		before := markup.Parses()
		if _, err := tr.Transform(art); err != nil {
			t.Fatal(err)
		}
		if got := markup.Parses() - before; got != want {
			t.Errorf("render ran %d parses, want %d: one for the document and one per inline directive whose content holds inline syntax", got, want)
		}
	}
}
