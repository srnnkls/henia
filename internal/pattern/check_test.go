package pattern

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestOptionalBindingJoinsRequiredBinder(t *testing.T) {
	query := `(skill :id ?s (link :target ?s)? @l) @t`
	rows := run(t, selfLinked(t), query)
	var got []string
	for _, row := range rows {
		cells := strings.Fields(strings.Join(idents([]Row{row}), " "))
		slices.Sort(cells)
		got = append(got, strings.Join(cells, " "))
	}
	slices.Sort(got)
	want := []string{"l= t=b", "l= t=c", "l=a t=a"}
	if !slices.Equal(got, want) {
		t.Errorf("%s: got %q, want %q", query, got, want)
	}
}

func TestOptionalBindingWithoutRequiredBinderFails(t *testing.T) {
	for _, query := range []string{
		`(skill (code :lang ?l)? @c (paragraph :text ?l)? @p) @s`,
		`(skill (link :target ?s)? @l (not (link :anchor ?s))) @t`,
		`(skill :id ?s (file (link :target ?s)? @l)) @t`,
		`(skill (link :target ?s)? @l (inbound (skill :id ?s) @f)) @t`,
		`(skill (link :target ?s)? @l) @t (skill :id ?s) @u`,
	} {
		_, err := Read(query)
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message, "is bound in an optional pattern") {
			t.Errorf("%s: err = %v, want optional-binding error", query, err)
		}
	}
}

func TestOptionalBindingAfterRequiredSiblingParses(t *testing.T) {
	query := `(skill (link :target ?s) @m (link :target ?s)? @l) @t`
	if _, err := Read(query); err != nil {
		t.Errorf("%s: err = %v, want nil", query, err)
	}
}

const missingAxis = `(row :table "axes" :value ?axis)
  (file :path ?p (section :title "Conventions") @s)
  (not (file :path ?p (section :title "Conventions" (table (row (cell :column "Axis" :text ?axis))))))`

func TestNegationConnectsPatterns(t *testing.T) {
	c := newCorpus(t, map[string]map[string]string{
		"a": {"refs/more.md": "# More\n\n## Conventions\n\n| Axis | Rule |\n| --- | --- |\n| Errors | Wrap them. |\n"},
		"b": {"SKILL.md": "# B\n\n## Conventions\n\n| Axis | Rule |\n| --- | --- |\n| Errors | Wrap them. |\n| Effects | Name them. |\n"},
	}, nil)
	q, err := Read(missingAxis)
	if err != nil {
		var pe *Error
		errors.As(err, &pe)
		t.Fatalf("read: %s", pe.Explain(missingAxis))
	}
	rows, err := q.Run(c.root, Environment{Resolve: c, Data: axes()})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, row := range rows {
		got = append(got, row.Vars["p"]+" "+row.Vars["axis"]+" L"+strconv.Itoa(row.Cells[0].Elements[0].Line))
	}
	if want := []string{"refs/more.md Effects L3"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNegationJoinsBothMembers(t *testing.T) {
	query := `(skill :id ?a) @x (skill :id ?b) @y (not (skill :id ?a (link :target ?b)))`
	var got []string
	for _, row := range run(t, fixture(t), query) {
		got = append(got, row.Vars["a"]+">"+row.Vars["b"])
	}
	slices.Sort(got)
	if want := []string{"a>a", "b>b", "c>a", "c>b", "c>c"}; !slices.Equal(got, want) {
		t.Errorf("%s: got %q, want %q", query, got, want)
	}
}

func TestNegationThatConnectsNothingFails(t *testing.T) {
	for _, tc := range []struct {
		query  string
		offset int
	}{
		{`(skill :id ?a) @x (code) @c (not (skill :id ?a))`, 18},
		{`(skill :id ?a) @x (code :lang ?l) @c (not (skill :id ?a (not (code :lang ?l))))`, 18},
		{`(skill :id ?a) @x (code :lang ?l) @c (paragraph) @p (not (skill :id ?a (code :lang ?l)))`, 37},
	} {
		_, err := Read(tc.query)
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message, "shares no variable with the first") || pe.Offset != tc.offset {
			t.Errorf("%s: err = %v, want disconnected-pattern error at %d", tc.query, err, tc.offset)
		}
	}
}
