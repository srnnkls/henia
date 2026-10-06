package pattern

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func grouped(t *testing.T) corpus {
	return newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\nOne two three.\n\nSeven eight.\n\n```go\nx\n```\n\n```bash\ny\n```\n\n```bash\nz\n```\n"},
		"b": {"SKILL.md": "# B\n\nFour five.\n\n```go\nw\n```\n"},
		"c": {"SKILL.md": "# C\n\nSix.\n"},
	}, nil)
}

func groups(rows []Row) []string {
	var out []string
	for _, row := range rows {
		var parts []string
		for _, cell := range row.Cells {
			var nodes []string
			for _, e := range cell.Elements {
				if e.Type == "skill" {
					nodes = append(nodes, e.Attrs["id"])
				} else {
					nodes = append(nodes, strings.TrimSpace(e.Text))
				}
			}
			if cell.Value {
				if len(cell.Values) == 0 {
					continue
				}
				parts = append(parts, "@"+cell.Name+"="+strings.Join(cell.Values, "+"))
				continue
			}
			parts = append(parts, cell.Name+"="+strings.Join(nodes, "+"))
		}
		for name, value := range row.Vars {
			if !strings.HasPrefix(name, "@") {
				parts = append(parts, "?"+name+"="+value)
			}
		}
		slices.Sort(parts)
		out = append(out, strings.Join(parts, " "))
	}
	slices.Sort(out)
	return out
}

func TestGroupReader(t *testing.T) {
	for _, query := range []string{
		`(code :lang ?l) @c (group ?l)`,
		`(code :lang ?l) @c (group ?l (count @c @n))`,
		`(skill (code) @c) @s (group @s (count @c (< $max)))`,
		`(code :lang ?l) @c (group (count @c @n (>= 3)) (count ?l @k))`,
		`(skill (paragraph) @p) @s (group @s (sum :words @p @w) (min :lines @p @m) (max :level @p @x))`,
	} {
		if _, err := Read(query); err != nil {
			t.Errorf("%s: err = %v, want nil", query, err)
		}
	}
	rejected := []struct{ query, mentions string }{
		{`(code :lang ?l) @c (group ?n (count @c @n))`, "?n"},
		{`(code :lang ?l) @c (group ?l (count @c @n) (count ?l @n))`, "@n"},
		{`(code :lang ?l :text ?n) @c (group ?l (count @c @n))`, "?n"},
		{`(code :lang ?l) @c (group (sum ?l @n))`, "sum"},
		{`(skill (paragraph) @p) @s (group @s (sum :bogus @p @n))`, "bogus"},
		{`(skill (paragraph) @p) @s (group @s (sum :level @p @n))`, "level"},
		{`(skill (paragraph) @p) @s (group @s (count @p @n (older 30)))`, "older"},
		{`(skill (paragraph)* @p) @s (group @p (count @s @n))`, "@p"},
	}
	for _, tc := range rejected {
		_, err := Read(tc.query)
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message+" "+pe.Hint, tc.mentions) {
			t.Errorf("%s: err = %v, want an *Error naming %s", tc.query, err, tc.mentions)
		}
	}
}

func TestGroupEvaluation(t *testing.T) {
	c := grouped(t)
	env := Environment{Resolve: c, Params: map[string]Value{"max": Number(2)}}
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"distinct keys collect their nodes", `(code :lang ?l) @c (group ?l)`,
			[]string{"?l=bash c=y+z", "?l=go c=x+w"}},
		{"uncaptured keys group by value", `(code :lang ?l) (group ?l)`,
			[]string{"=x+w ?l=go", "=y+z ?l=bash"}},
		{"keys and counts without node captures", `(code :lang ?l) (group ?l (count ?l @n))`,
			[]string{"?l=bash @n=1", "?l=go @n=1"}},
		{"count nodes versus values", `(code :lang ?l) @c (group (count @c @n) (count ?l @k))`,
			[]string{"@k=2 @n=4 c=x+y+z+w"}},
		{"grouping precedes capture dedup", `(skill (code :lang ?l)) @s (group ?l (count @s @n))`,
			[]string{"?l=bash @n=1 s=a", "?l=go @n=2 s=a+b"}},
		{"keyless group over nothing", `(code :lang "rust") @c (group (count @c @n) (sum :words @c @w))`,
			[]string{"@n=0 c="}},
		{"keyed group over nothing", `(code :lang ?l :text "nothing") @c (group ?l (count @c @n))`,
			nil},
		{"zero groups through optional", `(skill (code :lang "bash")? @c) @s (group @s (count @c @n))`,
			[]string{"@n=0 c= s=b", "@n=0 c= s=c", "@n=2 c=y+z s=a"}},
		{"empty key forms one group", `(skill (paragraph :words 3..)? @p) @s (group @p (count @s @n))`,
			[]string{"@n=1 p=One two three. s=a", "@n=2 p= s=b+c"}},
		{"sum min max over words", `(skill (paragraph) @p) @s (group @s (sum :words @p @w) (min :words @p @m) (max :words @p @x))`,
			[]string{"@m=1 @w=1 @x=1 p=Six. s=c", "@m=2 @w=2 @x=2 p=Four five. s=b", "@m=2 @w=5 @x=3 p=One two three.+Seven eight. s=a"}},
		{"sum over only empty captures is unbound", `(skill (paragraph :words 3..)? @p) @s (group @s (sum :words @p @w))`,
			[]string{"@w=3 p=One two three. s=a", "p= s=b", "p= s=c"}},
		{"min over non-numeric values is unbound", `(code :lang ?l) @c (group (min ?l @n))`,
			[]string{"c=x+y+z+w"}},
		{"count filter drops groups", `(skill (code) @c) @s (group @s (count @c @n (>= 2)))`,
			[]string{"@n=3 c=x+y+z s=a"}},
		{"param filter without output", `(skill (code) @c) @s (group @s (count @c (< $max)))`,
			[]string{"c=w s=b"}},
		{"filter on unbound result fails", `(skill (paragraph :words 3..)? @p) @s (group @s (sum :words @p @w (>= 0)))`,
			[]string{"@w=3 p=One two three. s=a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Read(tc.query)
			if err != nil {
				t.Fatalf("read %q: %v", tc.query, err)
			}
			rows, err := q.Run(c.root, env)
			if err != nil {
				t.Fatal(err)
			}
			if got := groups(rows); !slices.Equal(got, tc.want) {
				t.Errorf("%s:\ngot  %q\nwant %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestGroupSkipsNonFiniteOperands(t *testing.T) {
	c := newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\n```NaN\nq\n```\n"},
	}, nil)
	q, err := Read(`(code :lang ?l) @c (group (min ?l @n))`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.Run(c.root, Environment{Resolve: c})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := groups(rows), []string{"c=q"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
