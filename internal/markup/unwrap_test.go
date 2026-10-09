package markup

import (
	"reflect"
	"testing"
)

func TestUnwrap(t *testing.T) {
	source := "# Skill\n\n:::head\n## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n:::\n\n## Detail\n\nMore.\n\n```md\n:::head\nliteral\n:::\n```\n\n:::head\nLast.\n:::\n"
	full, inner := Unwrap(source, "head")
	want := "# Skill\n\n## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n\n## Detail\n\nMore.\n\n```md\n:::head\nliteral\n:::\n```\n\nLast.\n"
	if full != want {
		t.Fatalf("full:\n%q\nwant:\n%q", full, want)
	}
	if !reflect.DeepEqual(inner, []string{"## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n", "Last.\n"}) {
		t.Fatalf("inner = %q", inner)
	}
}

func TestUnwrapDocument(t *testing.T) {
	source := "# Skill\n\n:::head\n## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n:::\n\n## Detail\n\nMore.\n\n```md\n:::head\nliteral\n:::\n```\n\n:::head\nLast.\n:::\n"
	doc, err := Parse("skill.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	head := UnwrapDocument(doc, "head")
	if got, want := string(Write(doc)), "# Skill\n\n## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n\n## Detail\n\nMore.\n\n```md\n:::head\nliteral\n:::\n```\n\nLast.\n"; got != want {
		t.Fatalf("full:\n%q\nwant:\n%q", got, want)
	}
	if head == nil {
		t.Fatal("no head document")
	}
	if got, want := string(Write(head)), "## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n\nLast.\n"; got != want {
		t.Fatalf("head:\n%q\nwant:\n%q", got, want)
	}
}
