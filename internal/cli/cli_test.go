package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
)

func TestHelpDescribesCompilerBoundary(t *testing.T) {
	out := &bytes.Buffer{}
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"build", "lint"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("missing %s: %s", word, out)
		}
	}
	for _, command := range rootCmd.Commands() {
		switch command.Name() {
		case "deploy":
			t.Fatalf("deployment command still registered: %s", command.Name())
		}
	}
	if rootCmd.PersistentFlags().Lookup("data-dir") != nil {
		t.Fatal("compiler still exposes fetched-source cache")
	}
}

func TestQueryGroupOutput(t *testing.T) {
	root := &markup.Element{Type: "corpus", Attrs: map[string]string{}}
	for _, spec := range []struct{ id, text string }{
		{"a", "# A\n\n```go\nx\n```\n\n```bash\ny\n```\n\n```bash\nz\n```\n"},
		{"b", "# B\n\n```go\nw\n```\n\n```bash\nv\n```\n"},
	} {
		file, err := markup.Tree([]byte(spec.text))
		if err != nil {
			t.Fatal(err)
		}
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": spec.id, "ref": spec.id}, Parent: root}
		file.Attrs["path"], file.Attrs["main"], file.Parent = "SKILL.md", "true", skill
		skill.Children = append(skill.Children, file)
		root.Children = append(root.Children, skill)
	}
	q, err := pattern.Read(`(code :lang ?l) @c (group ?l (count @c ?n))`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.Run(root, pattern.Environment{})
	if err != nil {
		t.Fatal(err)
	}
	render := func(o queryOutput) string {
		out := &bytes.Buffer{}
		o.print(out, rows)
		return out.String()
	}

	if got := render(queryOutput{count: true}); got != "2\n" {
		t.Errorf("--count = %q, want 2 groups", got)
	}

	var records []map[string]any
	if err := json.Unmarshal([]byte(render(queryOutput{json: true})), &records); err != nil {
		t.Fatal(err)
	}
	counts := map[any]any{}
	for _, record := range records {
		counts[record["?l"]] = record["?n"]
	}
	if want := map[any]any{"bash": 3.0, "go": 2.0}; !maps.Equal(counts, want) {
		t.Errorf("--json ?l -> ?n = %v, want %v", counts, want)
	}

	text := render(queryOutput{})
	for _, line := range []string{"?l=bash  ?n=3", "?l=go  ?n=2"} {
		if !slices.Contains(strings.Split(text, "\n"), line) {
			t.Errorf("text output lacks %q:\n%s", line, text)
		}
	}
	for _, line := range []string{"@c  3 nodes", "@c  2 nodes"} {
		if !slices.Contains(strings.Split(text, "\n"), line) {
			t.Errorf("text output lacks %q:\n%s", line, text)
		}
	}
	if got := strings.Count(text, "@c  "); got != 2 {
		t.Errorf("text output has %d capture lines, want 2:\n%s", got, text)
	}
	if got := strings.Count(render(queryOutput{text: true}), "@c  "); got != 5 {
		t.Errorf("--text output has %d capture lines, want 5", got)
	}

	q, err = pattern.Read(`(code :lang ?l :lang ?l) @c`)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err = q.Run(root, pattern.Environment{}); err != nil {
		t.Fatal(err)
	}
	records = nil
	if err := json.Unmarshal([]byte(render(queryOutput{json: true})), &records); err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if _, ok := record["?l"].(string); !ok {
			t.Errorf("ungrouped --json record lacks string ?l: %v", record)
		}
	}
}
