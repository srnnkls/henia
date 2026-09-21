package transform

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
)

func TestReferencesAfterCodeBlocks(t *testing.T) {
	prefix := "```bash\nprintf '`$review`'\n```\n\n"
	suffix := "\n~~~md\n`$review`\n~~~\n\n    `$review`\n\nAfter `$review`.\n\nLiteral `` `$review` `` and \\`$review\\`.\n"
	for _, format := range []string{"xml", "directives"} {
		t.Run(format, func(t *testing.T) {
			tr := Transformer{
				OutputFormat: format,
				References:   map[string]ReferenceConfig{"skill": {Output: "/skill:{{.Name}}"}},
				Tools:        map[string]string{"read": "read_file"},
			}
			art := &artifact.Artifact{Body: prefix + ":::instruction\nUse `$review` with `!read`.\n:::\n" + suffix}
			got, err := tr.Transform(art)
			if err != nil {
				t.Fatal(err)
			}
			instruction := ":::instruction\nUse `/skill:review` with `read_file`.\n:::\n"
			if format == "xml" {
				instruction = "<instruction>\nUse `/skill:review` with `read_file`.\n</instruction>\n"
			}
			want := prefix + instruction + strings.Replace(suffix, "After `$review`.", "After `/skill:review`.", 1)
			if got.Body != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got.Body, want)
			}
		})
	}
}
