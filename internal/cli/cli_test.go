package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"path/filepath"
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

func sortFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	project := t.TempDir()
	skills := filepath.Join(project, ".henia", "skills")
	for id, body := range map[string]string{
		"a": "# A\n\none two three four five six seven eight nine ten eleven\n\n```go\nx\n```\n\n```bash\ny\n```\n",
		"b": "# B\n\none\n\n```bash\ny\n```\n",
		"c": "# C\n\none two three four five six seven eight\n",
		"d": "# D\n\none two\n\n```bash\ny\n```\n\n```zsh\nz\n```\n",
	} {
		fixture(t, filepath.Join(skills, id, "SKILL.md"), "---\nname: "+id+"\ndescription: "+id+".\n---\n"+body)
	}
	return project
}

func runQuery(t *testing.T, project string, args ...string) string {
	t.Helper()
	cmd := newQueryCommand()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"--project", project, "--canonical"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("query %v: %v", args, err)
	}
	return out.String()
}

func captureLines(output, capture string) []string {
	var got []string
	for line := range strings.SplitSeq(output, "\n") {
		if rest, ok := strings.CutPrefix(line, "@"+capture+"  "); ok {
			got = append(got, rest)
		}
	}
	return got
}

func TestQuerySortReader(t *testing.T) {
	project := sortFixture(t)
	for _, args := range [][]string{
		{"(skill :words ?w) @s", "--sort", "?w"},
		{"(skill) @s", "--sort", "@s.words"},
	} {
		out := runQuery(t, project, args...)
		if got := len(captureLines(out, "s")); got != 4 {
			t.Errorf("%v: %d @s rows, want 4 with the sort name counted as used:\n%s", args, got, out)
		}
	}
	for _, c := range []struct {
		args []string
		name string
	}{
		{[]string{"(skill :id ?i (frontmatter :name ?i)) @s", "--sort", "?zz"}, "?zz"},
		{[]string{"(skill) @s", "--sort", "@x.words"}, "@x"},
		{[]string{"(skill (code :lang ?l) @c) @s (group @s (count @c ?n))", "--sort", "?l"}, "?l"},
	} {
		out := runQuery(t, project, c.args...)
		if !strings.HasPrefix(out, "henia query:") || !strings.Contains(out, c.name) || len(captureLines(out, "s")) > 0 {
			t.Errorf("%v: want a reader error naming %s, got:\n%s", c.args, c.name, out)
		}
	}
}

func TestQuerySortOrder(t *testing.T) {
	project := sortFixture(t)
	order := func(args ...string) string {
		var ids []string
		for _, line := range captureLines(runQuery(t, project, args...), "s") {
			ids = append(ids, strings.Fields(line)[0])
		}
		return strings.Join(ids, " ")
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"(skill :words ?w) @s", "--sort", "-?w"}, "a c d b"},
		{[]string{"(skill :words ?w) @s", "--sort", "?w"}, "b d c a"},
		{[]string{"(skill) @s", "--sort", "-@s.words"}, "a c d b"},
		{[]string{"(skill (code :lang ?l)? @c) @s", "--sort", "?l"}, "a b d a d c"},
		{[]string{"(skill (code :lang ?l)? @c) @s", "--sort", "-?l"}, "d a a b d c"},
		{[]string{"(skill (code) @c) @s (group @s)", "--sort", "@c.lang"}, "b d a"},
		{[]string{"(skill (code) @c) @s (group @s)", "--sort", "-@c.lang"}, "a b d"},
		{[]string{"(skill :words ?w) @s", "--sort", "-?w", "--limit", "2"}, "a c"},
	} {
		if got := order(c.args...); got != c.want {
			t.Errorf("%v: @s order = %q, want %q", c.args, got, c.want)
		}
	}

	var langs []string
	grouped := runQuery(t, project, "(skill (code :lang ?l) @c) @s (group ?l (count @s ?n))", "--sort", "-?n")
	for line := range strings.SplitSeq(grouped, "\n") {
		if strings.HasPrefix(line, "?l=") {
			langs = append(langs, line)
		}
	}
	if got, want := strings.Join(langs, " | "), "?l=bash  ?n=3 | ?l=go  ?n=1 | ?l=zsh  ?n=1"; got != want {
		t.Errorf("grouped --sort -?n = %q, want %q", got, want)
	}

	ungrouped := runQuery(t, project, "(skill :id ?i :words ?w (frontmatter :name ?i)) @s", "--sort", "-?w")
	var vars []string
	for line := range strings.SplitSeq(ungrouped, "\n") {
		if strings.HasPrefix(line, "?") {
			vars = append(vars, line)
		}
	}
	if got, want := strings.Join(vars, " "), "?w=16 ?w=9 ?w=7 ?w=4"; got != want {
		t.Errorf("ungrouped variable lines = %q, want %q, the sorted variable only:\n%s", got, want, ungrouped)
	}
}

func TestQueryGroupCellLabels(t *testing.T) {
	project := sortFixture(t)
	lines := strings.Split(runQuery(t, project, "(code :lang ?l) @c (group ?l (count @c ?n))"), "\n")
	if !slices.Contains(lines, "@c  1 node") || slices.Contains(lines, "@c  1 nodes") {
		t.Errorf("single-node cell should read \"@c  1 node\":\n%s", strings.Join(lines, "\n"))
	}
	lines = strings.Split(runQuery(t, project, "(code :lang ?l) (group ?l)"), "\n")
	if !slices.Contains(lines, "3 matches") || !slices.Contains(lines, "1 match") || slices.Contains(lines, "3 nodes") {
		t.Errorf("implicit collected cell should read \"N matches\":\n%s", strings.Join(lines, "\n"))
	}
}

func TestQuerySortKeepsRowsDifferingInSortVariable(t *testing.T) {
	project := sortFixture(t)
	out := runQuery(t, project, "(skill :id \"a\" (code :lang ?l)) @s", "--sort", "?l")
	var vars []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "?l=") {
			vars = append(vars, line)
		}
	}
	if got, want := strings.Join(vars, " "), "?l=bash ?l=go"; got != want {
		t.Errorf("--sort ?l variable lines = %q, want %q:\n%s", got, want, out)
	}
}
