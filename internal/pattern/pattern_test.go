package pattern

import (
	"errors"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

type corpus struct {
	root   *markup.Element
	skills map[string]*markup.Element
}

func (c corpus) Skill(ref string) *markup.Element { return c.skills[ref] }

func newCorpus(t *testing.T, skills map[string]map[string]string, links map[string][]string) corpus {
	t.Helper()
	c := corpus{root: &markup.Element{Type: "corpus", Attrs: map[string]string{}}, skills: map[string]*markup.Element{}}
	for _, name := range []string{"a", "b", "c"} {
		files, ok := skills[name]
		if !ok {
			continue
		}
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": name}, Parent: c.root}
		c.root.Children = append(c.root.Children, skill)
		c.skills[name] = skill
		for _, path := range []string{"SKILL.md", "refs/more.md"} {
			text, ok := files[path]
			if !ok {
				continue
			}
			file, err := markup.Tree([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			file.Attrs["path"], file.Attrs["main"] = path, map[bool]string{true: "true", false: "false"}[path == "SKILL.md"]
			file.Parent, file.Index = skill, len(skill.Children)
			skill.Children = append(skill.Children, file)
		}
		for _, spec := range links[name] {
			target, anchor, _ := strings.Cut(spec, "#")
			attrs := map[string]string{"target": target, "path": "SKILL.md"}
			if anchor != "" {
				attrs["anchor"] = anchor
			}
			main := skill.Children[0]
			main.Children = append(main.Children, &markup.Element{Type: "link", Attrs: attrs, Text: spec, Parent: main, Index: len(main.Children)})
		}
	}
	return c
}

func fixture(t *testing.T) corpus {
	return newCorpus(t, map[string]map[string]string{
		"a": {
			"SKILL.md":     "# A\n\nIntro.\n\n## Usage\n\nFirst para.\n\nSecond has Term here.\n\nThird para.\n\n```bash\necho top\n```\n\n### Deeper\n\n```bash\necho deep\n```\n\n```go\nfunc x() {}\n```\n\n## Other\n\nTerm leads this section.\n\n- item one\n- item two\n",
			"refs/more.md": "# More\n\nResource para with term.\n",
		},
		"b": {"SKILL.md": "# B\n\nPlain.\n"},
		"c": {"SKILL.md": "# C\n\nNo links.\n\n## Usage\n\nC usage.\n"},
	}, map[string][]string{"a": {"b", "c#usage", "c#nope"}, "b": {"a", "c"}})
}

func run(t *testing.T, c corpus, query string) []Row {
	t.Helper()
	q, err := Read(query)
	if err != nil {
		var pe *Error
		if errors.As(err, &pe) {
			t.Fatalf("read %q: %s", query, pe.Explain(query))
		}
		t.Fatal(err)
	}
	rows, err := q.Run(c.root, c)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func texts(rows []Row) []string {
	var out []string
	for _, row := range rows {
		var cells []string
		for _, cell := range row {
			var parts []string
			for _, e := range cell.Elements {
				parts = append(parts, e.Text)
			}
			cells = append(cells, cell.Name+"="+strings.Join(parts, "+"))
		}
		out = append(out, strings.Join(cells, " "))
	}
	return out
}

func TestRun(t *testing.T) {
	c := fixture(t)
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"headings including resources", `(skill :id "a" (heading) @h)`, []string{"h=A", "h=Usage", "h=Deeper", "h=Other", "h=More"}},
		{"neighbours of a match", `(skill :id "a" (paragraph)? @prev . (paragraph :contains "term") @hit . (paragraph)? @next)`, []string{
			"prev=First para. hit=Second has Term here. next=Third para.",
			"prev= hit=Term leads this section. next=",
			"prev= hit=Resource para with term. next=",
		}},
		{"code at any depth", `(section :id "usage" (code :lang "bash") @c)`, []string{"c=echo top\n", "c=echo deep\n"}},
		{"direct children only", `(section :id "usage" > (code :lang "bash") @c)`, []string{"c=echo top\n"}},
		{"cross-skill links", `(skill @s (link :target "a"))`, []string{"s=" + c.skills["b"].Text}},
		{"inbound", `(skill :id "c" (inbound (skill) @from))`, []string{"from=", "from="}},
		{"join on shared text across skills", `(join (skill :id ?x (heading :text ?t) @h1) (skill :id (not ?x) (heading :text ?t) @h2))`, []string{"h1=Usage h2=Usage", "h1=Usage h2=Usage"}},
		{"anti-join finds broken anchors", `(join (skill (link :target ?s :path ?p :anchor ?a) @l) (not (skill :id ?s (file :path ?p (section :id ?a)))))`, []string{"l=c#nope"}},
		{"correlated negation", `(skill :id "a" (code :lang ?l) @c (not (section :id "usage" > (code :lang ?l))))`, []string{"c=func x() {}\n"}},
		{"literal inequality", `(section :level 2 :title (not "Usage") > (heading) @h)`, []string{"h=Other"}},
		{"orphans", `(skill (not (inbound (skill)))) @s`, []string{}},
		{"skills reaching nothing", `(skill (not (reaches (skill)))) @s`, []string{"s="}},
		{"transitive reach with a cycle", `(skill :id "a" (reaches (skill) @t))`, []string{"t=", "t=", "t="}},
		{"negation", `(section :level 2 (not (code)) (heading) @h)`, []string{"h=Other", "h=Usage"}},
		{"pairs in document order", `(section :level 3 (heading) @title (code) @c)`, []string{"title=Deeper c=echo deep\n", "title=Deeper c=func x() {}\n"}},
		{"implicit capture", `(heading :level 1..1 :matches "^[BC]$")`, []string{"=B", "=C"}},
		{"alternation", `(section :id "deeper" [(code :lang "go") (code :lang "rust")] @c)`, []string{"c=func x() {}\n"}},
		{"first child anchor", `(item . (paragraph) @p)`, []string{"p=item one", "p=item two"}},
		{"file scope", `(file :main "false" (heading) @h)`, []string{"h=More"}},
		{"regexp value", `(section :title /^D/ > (heading) @h)`, []string{"h=Deeper"}},
		{"own heading only", `(skill :id "a" (section :id "usage" > (heading) @h))`, []string{"h=Usage"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := texts(run(t, c, tc.query))
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestReachIDs(t *testing.T) {
	c := fixture(t)
	var ids []string
	for _, row := range run(t, c, `(skill :id "a" (reaches (skill) @t))`) {
		ids = append(ids, row[0].Elements[0].Attrs["id"])
	}
	if strings.Join(ids, ",") != "a,b,c" {
		t.Errorf("reach = %v, want a,b,c", ids)
	}
}

func TestReadErrors(t *testing.T) {
	cases := []struct {
		query, message, hint string
		offset               int
	}{
		{`(skill (headng) @h)`, `unknown type "headng"`, "did you mean heading?", 8},
		{`(skill :id "x" (heading)`, "unclosed (", "add the matching )", 0},
		{`(code :language "go")`, "code has no key :language", "did you mean lang?", 6},
		{`(skill :id 'x')`, `unexpected '\''`, `quote strings with "`, 11},
		{`(heading :level two)`, ":level takes a number or a range", ":level 2", 16},
		{`(paragraph :matches "(")`, "invalid regular expression", "", 20},
		{`(not (code))`, "(not ...) belongs inside a pattern", "", 1},
		{`(heading)?`, "quantifiers apply to patterns nested", "", 9},
		{`(section . )`, "anchor . needs a pattern after it", "", 11},
		{`(link :url /^https)`, "unterminated /regexp/", "", 11},
		{`(join (heading :text ?t) (skill))`, "a join prints its captures", "", 0},
		{`(heading :contains ?t)`, ":contains takes a string, not a variable", "bind the text with :text ?x", 19},
	}
	for _, tc := range cases {
		_, err := Read(tc.query)
		var pe *Error
		if !errors.As(err, &pe) {
			t.Fatalf("%s: err = %v, want *Error", tc.query, err)
		}
		if !strings.Contains(pe.Message, tc.message) || !strings.Contains(pe.Hint, tc.hint) || pe.Offset != tc.offset {
			t.Errorf("%s: got %q / %q at %d, want %q / %q at %d", tc.query, pe.Message, pe.Hint, pe.Offset, tc.message, tc.hint, tc.offset)
		}
	}
}

func TestExplainPointsAtOffset(t *testing.T) {
	query := `(skill (headng))`
	_, err := Read(query)
	var pe *Error
	errors.As(err, &pe)
	want := "henia query: unknown type \"headng\"\n  (skill (headng))\n          ^\n  did you mean heading?\n"
	if got := pe.Explain(query); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
