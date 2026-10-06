package pattern

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCaptureRows(t *testing.T) {
	c := grouped(t)
	env := Environment{Resolve: c}
	cases := []struct {
		name, query string
		want        []string
	}{
		{"after a pattern, the node", `(code :lang "go") @c`,
			[]string{"c=w", "c=x"}},
		{"after a variable, its value", `(code :lang ?l @lang) @c`,
			[]string{"?l=bash @lang=bash c=y", "?l=bash @lang=bash c=z", "?l=go @lang=go c=w", "?l=go @lang=go c=x"}},
		{"a variable and a capture of one name", `(skill :id ?s @s (code) @c) (group ?s (count @c @n))`,
			[]string{"?s=a @n=3 @s=a c=x+y+z", "?s=b @n=1 @s=b c=w"}},
		{"after a literal, the value", `(code :lang "bash" @l) @c`,
			[]string{"@l=bash c=y", "@l=bash c=z"}},
		{"a literal on a skill", `(skill :id "b" @s)`,
			[]string{"@s=b"}},
		{"a value keys a group", `(code :lang ?l @l) @c (group @l (count @c @n))`,
			[]string{"@l=bash @n=2 c=y+z", "@l=go @n=2 c=x+w"}},
		{"count reads distinct values", `(code :lang ?l @l) @c (group (count @l @k))`,
			[]string{"@k=2 @l=go+bash c=x+y+z+w"}},
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

func TestScoreCapture(t *testing.T) {
	c := newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\nKeep the branch short and rebase it often.\n"},
		"b": {"SKILL.md": "# B\n\nKeep the branch short and rebase it daily.\n"},
	}, nil)
	q, err := Read(`(skill :id ?x (paragraph :text ?t) @a) (skill :id (after ?x) (paragraph :text (near ?t 0.3 @score)) @b)`)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.Run(c.root, Environment{Resolve: c})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	score, ok := rows[0].Value("score")
	if !ok || score == "" || score == "0" {
		t.Errorf("@score = %q, %v; want the shingle similarity", score, ok)
	}
	if _, leaked := rows[0].Vars["score"]; leaked {
		t.Error("@score leaked into the ?variables")
	}
}

func TestCaptureRejected(t *testing.T) {
	for _, tc := range []struct{ query, mentions string }{
		{`(skill @s (code))`, "captures what precedes it"},
		{`(skill (code) @c @d)`, "captures what precedes it"},
		{`(code :lang ?l @c) @c`, "already names a value"},
		{`(code :lang ?l @l :text ?t @l) @c`, "already captured"},
		{`(skill (code :lang ?l @l)+ @c) @s`, "under * or +"},
		{`(skill :id ?s (not (code :lang ?l @l)) (link :target ?s)) @k`, "inside (not ...)"},
		{`(paragraph :text ?t) @a (paragraph :text (near ?t 0.6 ?score)) @b`, "@score, not ?score"},
		{`(code :lang ?l) @c (group ?l (count @c ?n))`, "@n, not ?n"},
		{`(code :lang ?l) @c (group ?l (count @c @c))`, "already captured"},
		{`(code :lang ?l) @c (group @n (count @c @n))`, "cannot key it"},
		{`(code :lang ?l @l) @c (group (sum :words @l @w))`, "captures a value"},
	} {
		_, err := Read(tc.query)
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message+" "+pe.Hint, tc.mentions) {
			t.Errorf("%s: err = %v, want an *Error mentioning %q", tc.query, err, tc.mentions)
		}
	}
}

func TestSortByValueCapture(t *testing.T) {
	for _, tc := range []struct {
		query, sort, mentions string
	}{
		{`(code :lang ?l) @c (group ?l (count @c @n))`, "-@n", ""},
		{`(code :lang ?l @l) @c`, "@l", ""},
		{`(code :lang ?l) @c`, "@c", "captures nodes"},
		{`(code :lang ?l) @c (group ?l (count @c @n))`, "@n.words", "captures a value"},
		{`(code :lang ?l) @c (group ?l (count @c @n))`, "-?n", "aggregates are captures"},
	} {
		_, err := Read(tc.query, tc.sort)
		if tc.mentions == "" {
			if err != nil {
				t.Errorf("%s --sort %s: err = %v, want nil", tc.query, tc.sort, err)
			}
			continue
		}
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message+" "+pe.Hint, tc.mentions) {
			t.Errorf("%s --sort %s: err = %v, want an *Error mentioning %q", tc.query, tc.sort, err, tc.mentions)
		}
	}
}
