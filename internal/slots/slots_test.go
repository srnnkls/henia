package slots

import "testing"

func TestParseDeclaration(t *testing.T) {
	for entry, want := range map[string]Declaration{
		"code.style":               {"code.style", Untyped},
		"code.style:list":          {"code.style", Type{}},
		"review.criteria:unique":   {"review.criteria", Type{Unique: true}},
		"code.style:keyed(list)":   {"code.style", Type{Keyed: true}},
		"code.style:keyed(unique)": {"code.style", Type{Keyed: true, Unique: true}},
	} {
		got, err := ParseDeclaration(entry)
		if err != nil || got != want {
			t.Errorf("ParseDeclaration(%q) = %+v, %v; want %+v", entry, got, err, want)
		}
		typed := got.Slot + ":" + got.Type.String()
		if reparsed, err := ParseDeclaration(typed); err != nil || reparsed != got {
			t.Errorf("round trip of %q via %q = %+v, %v", entry, typed, reparsed, err)
		}
	}
	for _, entry := range []string{"", "code..style", ".code", "code.style:", "code.style:map", "code.style:keyed(list", "code.style:keyed(keyed(list))", "code style"} {
		if got, err := ParseDeclaration(entry); err == nil {
			t.Errorf("ParseDeclaration(%q) = %+v; want an error", entry, got)
		}
	}
}

func TestParseDefinition(t *testing.T) {
	for entry, want := range map[string]Definition{
		"code.style.python":          {"code.style.python", Normal},
		"code.style.python@fallback": {"code.style.python", Fallback},
		"review.criteria@force":      {"review.criteria", Force},
		"git.commits@normal":         {"git.commits", Normal},
	} {
		if got, err := ParseDefinition(entry, Normal); err != nil || got != want {
			t.Errorf("ParseDefinition(%q) = %+v, %v; want %+v", entry, got, err, want)
		}
	}
	if got, _ := ParseDefinition("code.style", Fallback); got.Priority != Fallback {
		t.Errorf("implied priority = %v; want fallback", got.Priority)
	}
	for _, entry := range []string{"code.style@", "code.style@urgent", "code.style@default", "@force", "code.style:list"} {
		if got, err := ParseDefinition(entry, Normal); err == nil {
			t.Errorf("ParseDefinition(%q) = %+v; want an error", entry, got)
		}
	}
}

func TestWithin(t *testing.T) {
	for _, c := range []struct {
		name, slot string
		want       bool
	}{
		{"code.style", "code.style", true},
		{"code.style.python", "code.style", true},
		{"code.styles", "code.style", false},
		{"code.style-guide", "code.style", false},
		{"code", "code.style", false},
	} {
		if got := Within(c.name, c.slot); got != c.want {
			t.Errorf("Within(%q, %q) = %v", c.name, c.slot, got)
		}
	}
}

func TestResolveRanksPriorityBeforeTier(t *testing.T) {
	skills := []Skill{
		{Path: "p", Tier: Project, Provided: []string{"git.commits"}},
		{Path: "g", Tier: Global, Provided: []string{"git.commits@force"}},
	}
	rows := Evaluate(skills).Rows([]string{"git.commits"}, false)
	if len(rows) != 1 || rows[0] != (Row{"git.commits", "global", "g"}) {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestResolveUniquePerKey(t *testing.T) {
	skills := []Skill{
		{Path: "owner", Tier: Global, Declared: []string{"code.style:keyed(unique)"}},
		{Path: "go", Tier: Global, Provided: []string{"code.style.go"}},
		{Path: "python", Tier: Global, Provided: []string{"code.style.python"}},
		{Path: "python2", Tier: Global, Provided: []string{"code.style.python"}},
	}
	conflicts := Evaluate(skills).Rows(nil, true)
	if len(conflicts) != 1 || conflicts[0] != (Row{"code.style.python", Conflict, "-"}) {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}
