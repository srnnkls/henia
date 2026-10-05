package markup

import (
	"reflect"
	"testing"
)

func TestUnwrap(t *testing.T) {
	source := "# Skill\n\n:::static\n## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n:::\n\n## Detail\n\nMore.\n\n```md\n:::static\nliteral\n:::\n```\n\n:::static\nLast.\n:::\n"
	full, inner := Unwrap(source, "static")
	want := "# Skill\n\n## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n\n## Detail\n\nMore.\n\n```md\n:::static\nliteral\n:::\n```\n\nLast.\n"
	if full != want {
		t.Fatalf("full:\n%q\nwant:\n%q", full, want)
	}
	if !reflect.DeepEqual(inner, []string{"## Routes\n\n| a | b |\n\n:::instruction{priority=high}\nRule.\n:::\n", "Last.\n"}) {
		t.Fatalf("inner = %q", inner)
	}
}
