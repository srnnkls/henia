package pattern

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func sortedCells(rows []Row) []string {
	var got []string
	for _, row := range rows {
		cells := strings.Fields(strings.Join(idents([]Row{row}), " "))
		slices.Sort(cells)
		got = append(got, strings.Join(cells, " "))
	}
	slices.Sort(got)
	return got
}

func TestTopLevelOptionalLeftJoins(t *testing.T) {
	cases := []struct {
		name   string
		corpus corpus
		query  string
		want   []string
	}{
		{"unmatched row kept once", chained(t), `(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)?`,
			[]string{"l= t=a", "l=b t=b", "l=c#usage t=c"}},
		{"each match joined", fixture(t), `(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)?`,
			[]string{"l=a t=a", "l=b t=b", "l=c t=c", "l=c#nope t=c", "l=c#usage t=c"}},
		{"self links excluded", selfLinked(t), `(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)?`,
			[]string{"l= t=b", "l=a t=a", "l=c#usage t=c"}},
		{"required members join first", chained(t), `(skill :id (not ?s) (link :target ?s) @l)? (skill :id ?s) @t`,
			[]string{"l= t=a", "l=b t=b", "l=c#usage t=c"}},
		{"negation after optional", chained(t), `(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)? (not (skill :id ?s (section :id "usage")))`,
			[]string{"l= t=a", "l=b t=b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sortedCells(run(t, tc.corpus, tc.query)); !slices.Equal(got, tc.want) {
				t.Errorf("%s: got %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestTopLevelOptionalReaderRules(t *testing.T) {
	rejected := []struct{ query, message string }{
		{`(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)*`, "quantifiers apply to patterns nested"},
		{`(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)+`, "quantifiers apply to patterns nested"},
		{`(skill :id ?t) @a (skill :id ?t (link :target ?s) @l)? (skill :id ?s)? @b`, "is bound in an optional pattern"},
		{`(skill :id ?s) @u (skill (link :target ?s)? @l) @t`, "is bound in an optional pattern"},
		{`(skill :id ?s (link :target ?s) @l) (skill (heading) @h)?`, "shares no variable with the first"},
	}
	for _, tc := range rejected {
		_, err := Read(tc.query)
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message, tc.message) {
			t.Errorf("%s: err = %v, want %q", tc.query, err, tc.message)
		}
	}
	for _, query := range []string{
		`(skill)? @s`,
		`(skill :id ?t) @a (skill :id ?s (link :target ?t) @l)? (not (skill :id ?s (heading)))`,
	} {
		_, err := Read(query)
		var pe *Error
		if !errors.As(err, &pe) || strings.Contains(pe.Message, "quantifiers apply") {
			t.Errorf("%s: err = %v, want a required-member error", query, err)
		}
	}
	for _, query := range []string{
		`(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)?`,
		`(skill :id ?t) @a (skill (link :target ?t) @l)? (not (skill :id ?t (heading :text "X")))`,
	} {
		if _, err := Read(query); err != nil {
			t.Errorf("%s: err = %v, want nil", query, err)
		}
	}
}

func TestTopLevelOptionalBoundedByMaxBindings(t *testing.T) {
	var text strings.Builder
	text.WriteString("# A\n")
	for range 400 {
		text.WriteString("\npara.\n")
	}
	file, err := markup.Tree([]byte(text.String()))
	if err != nil {
		t.Fatal(err)
	}
	root := &markup.Element{Type: "corpus", Attrs: map[string]string{}}
	skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": "a"}, Parent: root}
	file.Attrs["path"], file.Attrs["main"], file.Parent = "SKILL.md", "true", skill
	skill.Children = append(skill.Children, file)
	root.Children = append(root.Children, skill)
	c := corpus{root: root, skills: map[string]*markup.Element{"a": skill}}

	query := `(skill :id ?s (paragraph) @p) (skill :id ?s (paragraph) @q)?`
	q, err := Read(query)
	if err != nil {
		t.Fatalf("read %q: %v", query, err)
	}
	if _, err := q.Run(c.root, Environment{Resolve: c}); !errors.Is(err, ErrTooMany) {
		t.Errorf("%s: err = %v, want ErrTooMany", query, err)
	}
}

func TestImpossibleIgnoresOptionalMembers(t *testing.T) {
	env := Environment{Params: map[string]Value{"th": Number(0)}}
	for query, want := range map[string]bool{
		`(skill :id ?s (paragraph :text ?t) @a) (skill :id ?s (paragraph :text (near ?t $th)) @b)?`: false,
		`(skill :id ?s (paragraph :text ?t) @a) (skill :id ?s (paragraph :text (near ?t $th)) @b)`:  true,
	} {
		q, err := Read(query)
		if err != nil {
			t.Fatalf("read %q: %v", query, err)
		}
		if got := q.Impossible(env); got != want {
			t.Errorf("%s: Impossible = %v, want %v", query, got, want)
		}
	}
}

func TestScoreIsNotAVariable(t *testing.T) {
	query := `(paragraph :text ?t) @a (paragraph :text (near ?t 0.6 @score)) @b (paragraph :text ?score)? @c`
	var pe *Error
	if _, err := Read(query); !errors.As(err, &pe) || !strings.Contains(pe.Message, "?score appears only once") {
		t.Errorf("%s: err = %v, want ?score to stay a separate, single-use variable", query, err)
	}
}
