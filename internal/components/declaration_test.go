package components

import (
	"slices"
	"testing"
)

func TestListOfSlotCoercesWhitespaceSeparatedItems(t *testing.T) {
	source := "---\nform: container\nattributes:\n  slots:\n    type: list\n    of: slot\n---\n::body\n"
	decl, err := Parse("head", []byte(source))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	attr := decl.Attributes["slots"]
	if attr.Type != "list" || attr.Of != "slot" {
		t.Fatalf("slots attribute = %+v, want list of slot", attr)
	}

	value, err := attr.Coerce("git.commits  nope")
	if err != nil {
		t.Fatalf("Coerce() error = %v", err)
	}
	items, ok := value.([]string)
	if !ok || !slices.Equal(items, []string{"git.commits", "nope"}) {
		t.Fatalf("Coerce() = %#v, want []string{\"git.commits\", \"nope\"}", value)
	}
}

func TestScalarCoercionAppliesMatches(t *testing.T) {
	source := "---\nform: leaf\nattributes:\n  count:\n    type: number\n    matches: '[1-9][0-9]*'\n    default: 0\n---\n"
	if _, err := Parse("counter", []byte(source)); err == nil {
		t.Fatal("Parse() accepted default 0 that does not match [1-9][0-9]*")
	}
	attr, err := newAttribute(attributeSchema{Type: "number", Matches: "[1-9][0-9]*"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := attr.Coerce("0"); err == nil {
		t.Error("Coerce(\"0\") accepted a number that does not match [1-9][0-9]*")
	}
}
