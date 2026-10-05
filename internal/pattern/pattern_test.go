package pattern

import (
	"errors"
	"strings"
	"testing"
	"time"

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
			attrs := map[string]string{"target": target}
			if anchor != "" {
				attrs["path"], attrs["anchor"] = "SKILL.md", anchor
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
		"b": {"SKILL.md": "# B\n\nPlain.\n\nSecond has Term here too.\n"},
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
	rows, err := q.Run(c.root, Environment{Resolve: c})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func texts(rows []Row) []string {
	var out []string
	for _, row := range rows {
		var cells []string
		for _, cell := range row.Cells {
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
		{"join on shared text across skills", `(skill :id ?x (heading :text ?t) @h1) (skill :id (not ?x) (heading :text ?t) @h2)`, []string{"h1=Usage h2=Usage", "h1=Usage h2=Usage"}},
		{"anti-join finds broken anchors", `(skill (link :target ?s :path ?p :anchor ?a) @l) (not (skill :id ?s (file :path ?p (section :id ?a))))`, []string{"l=c#nope"}},
		{"correlated negation", `(skill :id "a" (code :lang ?l) @c (not (section :id "usage" > (code :lang ?l))))`, []string{"c=func x() {}\n"}},
		{"word counts with an open range", `(paragraph :words 4..)`, []string{"=Second has Term here.", "=Term leads this section.", "=Resource para with term.", "=Second has Term here too."}},
		{"near duplicates across skills, each pair once", `(skill :id ?x (paragraph :text ?t) @a) (skill :id (after ?x) (paragraph :text (near ?t 0.6)) @b)`, []string{"a=Second has Term here. b=Second has Term here too."}},
		{"to resolves a section", `(link (to (section) @s)) @l`, []string{"s=## Usage\n\nC usage. l=c#usage"}},
		{"to resolves a whole skill", `(link :target "c" (to (skill) @s)) @l`, []string{"s= l=c"}},
		{"dangling links resolve to nothing", `(link :anchor /./ (not (to (section)))) @l`, []string{"l=c#nope"}},
		{"from finds the links into a section", `(section :id "usage" (from (link) @l)) @s`, []string{"l=c#usage s=## Usage\n\nC usage."}},
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
		ids = append(ids, row.Cells[0].Elements[0].Attrs["id"])
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
		{`(not (code))`, "a query needs a pattern outside (not ...)", "", 0},
		{`(heading)?`, "quantifiers apply to patterns nested", "", 9},
		{`(section . )`, "anchor . needs a pattern after it", "", 11},
		{`(link :url /^https)`, "unterminated /regexp/", "", 11},
		{`(heading :text ?t) (skill)`, "a query of several patterns prints its captures", "", 0},
		{`(paragraph :text (near ?t 2))`, "(near ?x T) takes a threshold above 0 and at most 1", "e.g. (near ?t 0.7)", 26},
		{`(paragraph :text (near "x" 0.5))`, "expected a ?variable", "", 23},
		{`(paragraph :text (twin ?t))`, "expected not, after, before, contains", "", 18},
		{`(skill :id ?x)`, "?x appears only once", "use ?x again", 11},
		{`(skill :id "a" (heading :text (not ?t)) @h) (skill :id "b" (heading :text (not ?t)))`, "?t is only compared, never bound", "bind it with :key ?t", 35},
		{`(skill (paragraph :text ?t)? @p (heading :text ?t)) @s`, "?t is bound in an optional pattern", "", 24},
		{`(skill :id ?s (not (section :id ?a @x)) (link :anchor ?a :target ?s)) @k`, "@x is inside (not ...), so it never prints", "", 19},
		{`(skill (heading :text ?t)) @a (skill (heading :text ?t)) @b (code) @c`, "shares no variable with the first", "share a ?variable", 60},
		{`(skill :id ?s (link :target ?s) @l) (not (section :id ?a (heading :text ?a)))`, "(not ...) shares no variable", "", 41},
		{`(skill [(heading :text ?t) (code :lang ?l)] (paragraph :text ?t)) @s`, "alternatives bind different variables: ?t and ?l", "", 27},
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

func TestSimilar(t *testing.T) {
	c := fixture(t)
	q, err := Read(`(skill :id "a" (paragraph :text ?t) @a) (skill :id "b" (paragraph :text (similar ?t 0.9)) @b)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Run(c.root, Environment{Resolve: c}); !errors.Is(err, ErrNoModel) {
		t.Fatalf("err = %v, want ErrNoModel", err)
	}
	embed := func(text string) []float32 {
		if strings.Contains(strings.ToLower(text), "term") {
			return []float32{1, 0}
		}
		return []float32{0, 0}
	}
	rows, err := q.Run(c.root, Environment{Resolve: c, Embed: embed})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"a=Second has Term here. b=Second has Term here too.",
		"a=Term leads this section. b=Second has Term here too.",
		"a=Resource para with term. b=Second has Term here too.",
	}
	if got := texts(rows); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestEnvironment(t *testing.T) {
	c := fixture(t)
	data := &markup.Element{Type: "data", Attrs: map[string]string{}}
	for _, row := range []map[string]string{{"table": "outdated", "key": "Term", "value": "Concept"}, {"table": "dates", "value": "2020-01-01"}, {"table": "dates", "value": "2026-09-30"}} {
		data.Children = append(data.Children, &markup.Element{Type: "row", Attrs: row, Parent: data})
	}
	env := Environment{Resolve: c, Params: map[string]string{"max": "10", "days": "30"}, Data: data, Now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	cases := []struct{ name, query, want string }{
		{"param comparison", `(file :lines (> $max)) @f`, "f="},
		{"first occurrence", `(heading :norm ?t :node ?n) @dup (heading :norm ?t :node (before ?n)) @first`, "dup=Usage first=Usage"},
		{"data rows joined by contains", `(row :table "outdated" :key ?old) @r (paragraph :text (contains ?old)) @p`, "r= p=Second has Term here.|r= p=Term leads this section.|r= p=Resource para with term.|r= p=Second has Term here too."},
		{"older dates", `(row :table "dates" :value (older $days)) @r`, "r="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Read(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := q.Run(c.root, env)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(texts(rows), "|"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	q, _ := Read(`(file :lines (> $missing)) @f`)
	if _, err := q.Run(c.root, Environment{Resolve: c}); err == nil || !strings.Contains(err.Error(), "$missing has no value") {
		t.Errorf("missing param: %v", err)
	}
}
