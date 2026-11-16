package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
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
	q, err := pattern.Read(`(code :lang ?l) @c (group ?l (count @c @n))`)
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
		counts[record["?l"]] = record["n"]
	}
	if want := map[any]any{"bash": 3.0, "go": 2.0}; !maps.Equal(counts, want) {
		t.Errorf("--json ?l -> n = %v, want %v", counts, want)
	}

	text := render(queryOutput{})
	for _, line := range []string{"?l=bash", "?l=go", "@n  3", "@n  2"} {
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
		{[]string{"(skill (code :lang ?l) @c) @s (group @s (count @c @n))", "--sort", "?l"}, "?l"},
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
	grouped := runQuery(t, project, "(skill (code :lang ?l) @c) @s (group ?l (count @s @n))", "--sort", "-@n")
	for line := range strings.SplitSeq(grouped, "\n") {
		if strings.HasPrefix(line, "?l=") || strings.HasPrefix(line, "@n") {
			langs = append(langs, line)
		}
	}
	if got, want := strings.Join(langs, " | "), "?l=bash | @n  3 | ?l=go | @n  1 | ?l=zsh | @n  1"; got != want {
		t.Errorf("grouped --sort -@n = %q, want %q", got, want)
	}
	if got := runQuery(t, project, "(skill (code :lang ?l) @c) @s (group ?l (count @s @n))", "--sort", "-?n"); !strings.Contains(got, "aggregates are captures") {
		t.Errorf("--sort -?n on an aggregate should point at -@n:\n%s", got)
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
	lines := strings.Split(runQuery(t, project, "(code :lang ?l) @c (group ?l (count @c @n))"), "\n")
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

func pathWorkspace(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func runPathQuery(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newQueryCommand()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Errorf("query %v: %v", args, err)
	}
	return out.String()
}

func addresses(output, capture string) []string {
	var got []string
	for _, line := range captureLines(output, capture) {
		got = append(got, strings.Fields(line)[0])
	}
	slices.Sort(got)
	return got
}

func TestQueryPathCollects(t *testing.T) {
	dir := pathWorkspace(t)
	fixture(t, filepath.Join(dir, "notes", "a.md"), "# Alpha\n")
	fixture(t, filepath.Join(dir, "notes", ".hidden", "b.md"), "# Beta\n")
	fixture(t, filepath.Join(dir, ".henia", "skills", "lib", "SKILL.md"), "---\nname: lib\ndescription: lib.\n---\n# Library\n")
	fixture(t, filepath.Join(dir, "docs", "guide.md"), "# Guide\n\n## Usage\n\n```bash\nrun\n```\n")
	fixture(t, filepath.Join(dir, "misc", "log.txt"), "first paragraph\n\nsecond paragraph\n")
	fixture(t, filepath.Join(dir, "big.md"), "# Big\n\n"+strings.Repeat("a", 1<<20)+"\n")

	if out := runPathQuery(t, "(heading) @h", "--path", "notes"); !slices.Equal(addresses(out, "h"), []string{"notes/a.md#alpha"}) || !strings.Contains(out, "Alpha") {
		t.Errorf("--path notes: want one Alpha row at notes/a.md#alpha, got:\n%s", out)
	}
	if out := runPathQuery(t, "(heading :level 1) @h", "--path", "."); !strings.Contains(out, "Alpha") || strings.Contains(out, "Beta") || strings.Contains(out, "Library") {
		t.Errorf("--path .: want Alpha without dot entries or library skills, got:\n%s", out)
	}
	if out := runPathQuery(t, "(code :lang \"bash\") @c", "--path", "./docs"); !slices.Equal(addresses(out, "c"), []string{"docs/guide.md#usage"}) {
		t.Errorf("--path ./docs: want address docs/guide.md#usage, got:\n%s", out)
	}
	if out := runPathQuery(t, "(paragraph) @p", "--path", "misc/log.txt"); len(captureLines(out, "p")) != 2 {
		t.Errorf("--path misc/log.txt: want two paragraph rows, got:\n%s", out)
	}
	out := runPathQuery(t, "(heading) @h", "--path", "big.md", "--path", "notes")
	if !strings.Contains(out, "henia query: big.md: omitted: over 1 MiB, invalid UTF-8 or contains NUL") || !slices.Equal(addresses(out, "h"), []string{"notes/a.md#alpha"}) {
		t.Errorf("oversized named file: want the omitted message and the notes row, got:\n%s", out)
	}
	if out := runPathQuery(t, "(file) @f", "--path", "./docs", "--path", "docs/guide.md"); !slices.Equal(addresses(out, "f"), []string{"docs/guide.md"}) {
		t.Errorf("overlapping paths: want docs/guide.md once, got:\n%s", out)
	}

	fixture(t, filepath.Join(dir, "heavy", "a.md"), "# Light\n")
	fixture(t, filepath.Join(dir, "heavy", "big.md"), "# Heavy\n\n"+strings.Repeat("a", 1<<20)+"\n")
	out = runPathQuery(t, "(heading) @h", "--path", "heavy", "--path", "heavy/big.md")
	if !strings.Contains(out, "henia query: heavy/big.md: omitted: over 1 MiB, invalid UTF-8 or contains NUL") || !slices.Equal(addresses(out, "h"), []string{"heavy/a.md#light"}) {
		t.Errorf("oversized file named after its directory: want the omitted message for heavy/big.md and the heavy/a.md row, got:\n%s", out)
	}
}

func TestQueryPathSkillGrouping(t *testing.T) {
	dir := pathWorkspace(t)
	skill := "---\nname: tool\ndescription: tool.\n---\n# Tool\n"
	fixture(t, filepath.Join(dir, "tree", "tool", "SKILL.md"), skill)
	fixture(t, filepath.Join(dir, "tree", "tool", "ref", "x.md"), "# X\n")
	fixture(t, filepath.Join(dir, "twin", "a", "tool", "SKILL.md"), skill)
	fixture(t, filepath.Join(dir, "twin", "b", "tool", "SKILL.md"), skill)

	owned := "(skill :id \"tool\" (file :path \"ref/x.md\")) @s"
	if out := runPathQuery(t, owned, "--path", "tree"); len(captureLines(out, "s")) != 1 {
		t.Errorf("--path tree: want the skill owning ref/x.md, got:\n%s", out)
	}
	if out := runPathQuery(t, "(skill) @s", "--path", "twin"); !slices.Equal(addresses(out, "s"), []string{"twin/a/tool", "twin/b/tool"}) {
		t.Errorf("colliding skills: want refs twin/a/tool and twin/b/tool, got:\n%s", out)
	}
	if out := runPathQuery(t, "(skill :id \"twin/a/tool\") @s", "--path", "twin"); len(captureLines(out, "s")) != 1 {
		t.Errorf("colliding skills: want id twin/a/tool, got:\n%s", out)
	}
	if out := runPathQuery(t, "(skill :id \"tool\") @s", "--path", "twin"); len(captureLines(out, "s")) != 0 {
		t.Errorf("colliding skills: bare id must match nothing, got:\n%s", out)
	}

	t.Chdir(filepath.Join(dir, "tree", "tool"))
	if out := runPathQuery(t, owned, "--path", "."); len(captureLines(out, "s")) != 1 {
		t.Errorf("--path . inside the skill: want the skill named after the root, got:\n%s", out)
	}

	fixture(t, filepath.Join(dir, "bench", "tool", "SKILL.md"), "---\nname: hammer\ndescription: hammer.\n---\n# Hammer\n")
	fixture(t, filepath.Join(dir, "bench", "other", "SKILL.md"), "---\nname: other\ndescription: other.\n---\n# Other\n\nUse `$hammer`.\n")
	t.Chdir(filepath.Join(dir, "bench", "tool"))
	if out := runPathQuery(t, "(skill :id \"other\" (reaches (skill) @t))", "--path", ".", "--path", "../other"); len(captureLines(out, "t")) != 1 {
		t.Errorf("root skill: want its frontmatter name hammer to resolve the link from other, got:\n%s", out)
	}
}

func TestQueryPathMessages(t *testing.T) {
	dir := pathWorkspace(t)
	fixture(t, filepath.Join(dir, "notes", "a.md"), "# Alpha\n")
	if err := os.MkdirAll(filepath.Join(dir, "project"), 0755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--path", "notes", "--path", "missing"}, "henia query: missing: no such file or directory"},
		{[]string{"--path", "notes", "--project", "project"}, "henia query: --path replaces the library; drop --project"},
		{[]string{"--path", "notes", "--package", "project"}, "henia query: --path replaces the library; drop --package"},
		{[]string{"--path", "notes", "--harness", "claude"}, "henia query: --path replaces the library; drop --harness"},
		{[]string{"--path", "notes", "--canonical"}, "henia query: --path replaces the library; drop --canonical"},
	}
	for _, c := range cases {
		out := runPathQuery(t, append([]string{"(heading) @h"}, c.args...)...)
		if !strings.Contains(out, c.want) || len(captureLines(out, "h")) != 0 {
			t.Errorf("%v: want %q and no rows, got:\n%s", c.args, c.want, out)
		}
	}
}
