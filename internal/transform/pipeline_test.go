package transform

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
)

func TestMarkupPipeline(t *testing.T) {
	art := &artifact.Artifact{Frontmatter: map[string]any{"enabled": true, "items": []string{"one", "two"}, "priority": "critical"}, Body: "{{if .enabled}}:::instruction{priority={{.priority}}}\n{{range .items}}- {{.}}\n{{end}}Use `!read` and `$review`.\n:::\n{{end}}"}
	tr := Transformer{OutputFormat: "xml", Variables: map[string]string{"priority": "high"}, Tools: map[string]string{"read": "Read"}, References: map[string]ReferenceConfig{"skill": {Output: "/{{.Name}}"}}}
	got, err := tr.Transform(art)
	if err != nil {
		t.Fatal(err)
	}
	want := "<instruction priority=\"high\">\n- one\n- two\nUse `Read` and `/review`.\n</instruction>\n"
	if got.Body != want {
		t.Fatalf("got %q, want %q", got.Body, want)
	}
	if art.Frontmatter["priority"] != "critical" {
		t.Fatal("mutated input frontmatter")
	}
	tr.OutputFormat = "directives"
	got, err = tr.Transform(art)
	if err != nil || !strings.HasPrefix(got.Body, ":::instruction{priority=\"high\"}") {
		t.Fatalf("directive output: %+v (%v)", got, err)
	}
}

func TestUnknownFormat(t *testing.T) {
	tr := Transformer{OutputFormat: "html"}
	if _, err := tr.Transform(&artifact.Artifact{Body: "ordinary markdown"}); err == nil {
		t.Fatal("expected unsupported format error")
	}
}

func BenchmarkPipeline(b *testing.B) {
	art := &artifact.Artifact{Frontmatter: map[string]any{"priority": "critical"}, Body: strings.Repeat(":::instruction{priority={{.priority}}}\nUse :term[TTL]{abbr=\"time to live\"}.\n:::\n\n", 140)}
	tr := Transformer{OutputFormat: "xml"}
	for b.Loop() {
		if _, err := tr.Transform(art); err != nil {
			b.Fatal(err)
		}
	}
}
