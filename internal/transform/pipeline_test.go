package transform

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
)

func TestMarkupPipeline(t *testing.T) {
	art := &artifact.Artifact{Frontmatter: map[string]any{"priority": "{{.priority}}"}, Body: ":::instruction{priority=high}\n- one\n- two\nUse `!read` and `$review`.\n:::\n"}
	tr := Transformer{OutputFormat: "xml", Variables: map[string]string{"priority": "high"}, Tools: map[string]string{"read": "Read"}, References: map[string]ReferenceConfig{"skill": {Output: "/{{.Name}}"}}}
	got, err := tr.Transform(art)
	if err != nil {
		t.Fatal(err)
	}
	want := "<instruction priority=\"high\">\n- one\n- two\nUse `Read` and `/review`.\n</instruction>\n"
	if got.Body != want {
		t.Fatalf("got %q, want %q", got.Body, want)
	}
	if got.Frontmatter["priority"] != "high" || art.Frontmatter["priority"] != "{{.priority}}" {
		t.Fatalf("frontmatter: got %v, input %v", got.Frontmatter["priority"], art.Frontmatter["priority"])
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

func TestNestedFrontmatterTemplates(t *testing.T) {
	art := &artifact.Artifact{
		Frontmatter: map[string]any{
			"metadata": map[string]any{"audience": "{{.audience}}"},
			"henia":    map[string]any{"variables": map[string]any{"audience": "author"}},
		},
		Body: ":::instruction{audience=reviewer}\n- correctness\n- coverage\nUse `!read`.\n:::\n",
	}
	tr := Transformer{OutputFormat: "xml", Variables: map[string]string{"audience": "reviewer"}, Tools: map[string]string{"read": "Read"}}
	got, err := tr.Transform(art)
	if err != nil {
		t.Fatal(err)
	}
	want := "<instruction audience=\"reviewer\">\n- correctness\n- coverage\nUse `Read`.\n</instruction>\n"
	if got.Body != want {
		t.Fatalf("got %q, want %q", got.Body, want)
	}
	if got.Frontmatter["metadata"].(map[string]any)["audience"] != "reviewer" {
		t.Fatal("nested frontmatter template was not expanded")
	}
	if art.Frontmatter["metadata"].(map[string]any)["audience"] != "{{.audience}}" {
		t.Fatal("input was mutated")
	}
	delete(tr.Variables, "audience")
	if got, err = tr.Transform(art); err != nil || got.Frontmatter["metadata"].(map[string]any)["audience"] != "author" {
		t.Fatalf("henia.variables fallback: %+v, %v", got, err)
	}
}

func BenchmarkPipeline(b *testing.B) {
	art := &artifact.Artifact{Frontmatter: map[string]any{"priority": "critical"}, Body: strings.Repeat(":::instruction{priority=critical}\nUse :term[TTL]{abbr=\"time to live\"}.\n:::\n\n", 140)}
	tr := Transformer{OutputFormat: "xml"}
	for b.Loop() {
		if _, err := tr.Transform(art); err != nil {
			b.Fatal(err)
		}
	}
}
