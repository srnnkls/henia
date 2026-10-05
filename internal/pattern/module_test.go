package pattern

import (
	"errors"
	"strings"
	"testing"
)

func TestReadModule(t *testing.T) {
	shared := `(define (titled ?t) (section > (heading :text ?t)))`
	imported := func(name string) (map[string]Define, error) {
		if name != "std" {
			return nil, errors.New("no module std2")
		}
		m, err := ReadModule(shared, nil)
		return m.Defines, err
	}
	src := `
(import std)
(define (usage-code ?lang) (section :id "usage" (code :lang ?lang)))
(rule bash-usage
  :severity error
  :message "{@c} is {?l}; use {$want}"
  (skill :id "a" (usage-code ?l) (code :lang ?l :text /top/) @c))
(rule usage :message "{@s}" (titled "Usage") @s)`
	m, err := ReadModule(src, imported)
	if err != nil {
		var pe *Error
		if errors.As(err, &pe) {
			t.Fatal(pe.Explain(src))
		}
		t.Fatal(err)
	}
	if len(m.Rules) != 2 || m.Rules[0].ID != "bash-usage" || m.Rules[0].Severity != "error" || strings.Join(m.Rules[0].Params(), ",") != "want" {
		t.Fatalf("rule = %+v", m.Rules[0])
	}
	c := fixture(t)
	rows, err := m.Rules[0].Query.Run(c.root, Environment{Resolve: c, Params: map[string]string{"want": "go"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(rows), "|"); got != "c=echo top\n" {
		t.Errorf("rows = %q", got)
	}
	rows, err = m.Rules[1].Query.Run(c.root, Environment{Resolve: c})
	if err != nil || len(rows) != 2 {
		t.Errorf("imported define: %d rows, %v", len(rows), err)
	}
}

func TestReadModuleErrors(t *testing.T) {
	cases := []struct{ src, message string }{
		{`(rule Bad :message "m" (heading))`, "lowercase letters"},
		{`(rule r (heading))`, "needs a :message"},
		{`(rule r :message "{@x}" (heading) @h)`, "uses @x"},
		{`(rule r :message "m" :at @x (heading) @h)`, "points at @x"},
		{`(define (loop ?a) (loop ?a)) (rule r :message "m" (loop "x") @h)`, "expands without end"},
		{`(define (one ?a) (heading :text ?a)) (rule r :message "m" (one) @h)`, "takes 1 arguments, got 0"},
		{`(define (heading) (code)) `, "cannot name a pattern"},
		{`(query (heading))`, "unknown module form"},
	}
	for _, tc := range cases {
		_, err := ReadModule(tc.src, nil)
		if err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Errorf("%s: err = %v, want %q", tc.src, err, tc.message)
		}
	}
}
