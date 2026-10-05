package pattern

import (
	"errors"
	"slices"
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
