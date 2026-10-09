package pattern

import (
	"slices"
	"strconv"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

type typedCorpus struct {
	corpus
	declared map[string]bool
}

func (c typedCorpus) Typed(e *markup.Element, key string) (any, bool) {
	if e.Type != "directive" || !c.declared[e.Attrs["name"]] || key != "level" {
		return nil, false
	}
	n, err := strconv.ParseFloat(e.Attrs[key], 64)
	return n, err == nil
}

func directiveLines(t *testing.T, c typedCorpus, query string) []int {
	t.Helper()
	q, err := Read(query)
	if err != nil {
		t.Fatalf("read %q: %v", query, err)
	}
	rows, err := q.Run(c.root, Environment{Resolve: c})
	if err != nil {
		t.Fatalf("run %q: %v", query, err)
	}
	var lines []int
	for _, row := range rows {
		for _, cell := range row.Cells {
			for _, e := range cell.Elements {
				lines = append(lines, e.Line)
			}
		}
	}
	return lines
}

func TestDeclaredNumberLevelComparesByValue(t *testing.T) {
	c := typedCorpus{
		corpus: newCorpus(t, map[string]map[string]string{
			"a": {"SKILL.md": "# A\n\n:::routes{level=3.0}\none\n:::\n\n:::routes{level=3}\ntwo\n:::\n\n:::routes{level=2.5}\nthree\n:::\n\n:::other{level=3.0}\nfour\n:::\n"},
		}, nil),
		declared: map[string]bool{"routes": true},
	}
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{`(directive :level 3) @d`, []int{3, 7}},
		{`(directive :level 3.0) @d`, []int{3, 7}},
		{`(directive :level 2.5) @d`, []int{11}},
		{`(directive :level 2..3) @d`, []int{3, 7, 11}},
		{`(directive :level 3..) @d`, []int{3, 7}},
		{`(directive :level (>= 3)) @d`, []int{3, 7, 15}},
	} {
		if got := directiveLines(t, c, tc.query); !slices.Equal(got, tc.want) {
			t.Errorf("%s matched lines %v, want %v", tc.query, got, tc.want)
		}
	}
}

func TestUndeclaredLevelKeepsStringSemantics(t *testing.T) {
	c := typedCorpus{corpus: newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\n:::other{level=3.0}\none\n:::\n\n:::other{level=3}\ntwo\n:::\n"},
	}, nil)}
	if got := directiveLines(t, c, `(directive :level 3) @d`); !slices.Equal(got, []int{7}) {
		t.Errorf(":level 3 matched lines %v, want [7]", got)
	}
	if got := directiveLines(t, c, `(directive :level 3.0) @d`); !slices.Equal(got, []int{7}) {
		t.Errorf(":level 3.0 matched lines %v, want [7]", got)
	}
}

func TestHeadingLevelAcceptsIntegralDecimal(t *testing.T) {
	c := newCorpus(t, map[string]map[string]string{"a": {"SKILL.md": "# A\n\n## B\n"}}, nil)
	if got := texts(run(t, c, `(heading :level 2.0) @h`)); !slices.Equal(got, []string{"h=B"}) {
		t.Errorf(":level 2.0 matched %v, want [h=B]", got)
	}
}
