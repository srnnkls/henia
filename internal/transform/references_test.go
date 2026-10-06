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

func TestReferenceOutputSyntaxes(t *testing.T) {
	art := &artifact.Artifact{Body: "Use `$review` and `@scout`.\n"}
	for _, output := range []string{"/skill:{{.Name}}", "/skill:{?name}"} {
		tr := Transformer{References: map[string]ReferenceConfig{"skill": {Output: output}, "agent": {Output: "{?kind} {@r.text}"}}}
		got, err := tr.Transform(art)
		if err != nil {
			t.Fatal(err)
		}
		if want := "Use `/skill:review` and `agent @scout`.\n"; got.Body != want {
			t.Errorf("%s: got %q, want %q", output, got.Body, want)
		}
	}
}

func TestReferenceOutputErrors(t *testing.T) {
	art := &artifact.Artifact{Body: "Use `$review`.\n"}
	for output, message := range map[string]string{
		"/{?skill}":    "{?skill} is not a reference field",
		"/{{.Nope}}":   "Nope",
		"/{{.Name":     "unclosed action",
		"/{{.Name}}{{": "unclosed action",
	} {
		tr := Transformer{References: map[string]ReferenceConfig{"skill": {Output: output}}}
		if _, err := tr.Transform(art); err == nil || !strings.Contains(err.Error(), message) {
			t.Errorf("%s: err = %v, want %q", output, err, message)
		}
	}
}
