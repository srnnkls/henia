package markup_test

import (
	"testing"

	"github.com/srnnkls/henia/internal/benchdata"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/srnnkls/henia/internal/slots"
)

func BenchmarkParse(b *testing.B) {
	body := benchdata.LargestSkill(b).Body
	source := []byte(body)
	cases := []struct {
		name  string
		parse func()
	}{
		{"tree", func() { _, _ = markup.Tree(source) }},
		{"inspect", func() { _, _ = markup.Inspect(source) }},
		{"sections", func() { _, _ = markup.Sections(source) }},
		{"references", func() { tree, _ := markup.Tree(source); _, _ = reference.Recognize(tree) }},
		{"preloads", func() { preload.Find(source) }},
		{"slots", func() { slots.Applications(body) }},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			for b.Loop() {
				c.parse()
			}
		})
	}
}
