package markup_test

import (
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestApply(t *testing.T) {
	source := "aaa bbb ccc ddd"
	applied := markup.Apply(source, []markup.Edit{
		{Start: 0, End: 7, Text: "X", Rule: "outer"},
		{Start: 4, End: 7, Text: "inner", Rule: "contained"},
		{Start: 8, End: 11, Text: "CC", Rule: "first"},
		{Start: 8, End: 11, Text: "lost", Rule: "same-span"},
		{Start: 10, End: 13, Text: "partial", Rule: "overlap"},
	})
	if applied.Text != "X CC ddd" {
		t.Errorf("text = %q", applied.Text)
	}
	if len(applied.Dropped) != 1 || applied.Dropped[0].Rule != "overlap" {
		t.Errorf("dropped = %+v", applied.Dropped)
	}
	if from, to, rule, ok := applied.Span(8, 11); !ok || applied.Text[from:to] != "CC" || rule != "first" {
		t.Errorf("span of the kept edit = %d-%d %q %v", from, to, rule, ok)
	}
	if from, to, _, ok := applied.Span(12, 15); !ok || applied.Text[from:to] != "ddd" {
		t.Errorf("span after edits = %d-%d %v", from, to, ok)
	}
	if _, _, _, ok := applied.Span(4, 7); ok {
		t.Error("a span inside a replaced edit mapped into the output")
	}
}
