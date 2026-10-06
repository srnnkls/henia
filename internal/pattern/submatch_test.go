package pattern

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestSubmatchesBindVariables(t *testing.T) {
	root, err := markup.Tree([]byte("Use `$review`, `@scout`, `plain` and `$git`.\n"))
	if err != nil {
		t.Fatal(err)
	}
	src := `
(define (sigiled @r ?sigil ?name) (code :ticks 1 :matches /^(?<sigil>[$@])(?<name>[a-z]+)$/) @r)
(rule all :message "{?s}{?n}" (sigiled @r ?s ?n))
(rule skills :message "{?n}" (sigiled @r "$" ?n))
(rule pair :message "{?n}" (code :matches /^\$(?<n>[a-z]+)$/) @a (paragraph :text (contains ?n)) @p)`
	m, err := ReadModule(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"$review @scout $git", "review git", "review git"}
	for i, rule := range m.Rules {
		rows, err := rule.Query.Run(root, Environment{})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, row := range rows {
			got = append(got, row.Vars["s"]+row.Vars["n"])
		}
		if strings.Join(got, " ") != want[i] {
			t.Errorf("rule %s: got %q, want %q", rule.ID, strings.Join(got, " "), want[i])
		}
	}
}

func TestSubmatchErrors(t *testing.T) {
	cases := []struct{ src, message string }{
		{`(rule r :message "m" (code :matches /(?<x>a)/) @c)`, "?x appears only once"},
		{`(rule r :message "m" (code :text (not /(?<x>a)/) :lang ?x) @c)`, "cannot bind ?x"},
		{`(define (d @c ?x) (code :matches /(?<x>a)/) @c) (rule r :message "m" (d @c (heading)))`, "takes a ?variable or a value"},
	}
	for _, tc := range cases {
		_, err := ReadModule(tc.src, nil)
		if err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Errorf("%s: err = %v, want %q", tc.src, err, tc.message)
		}
	}
}
