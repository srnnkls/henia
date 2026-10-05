package slots

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseType(t *testing.T) {
	for source, want := range map[string]Type{
		"list":                   {},
		"unique":                 {Unique: true},
		"keyed(list)":            {Keyed: true},
		"keyed(unique)":          {Keyed: true, Unique: true},
		"keyed(list(command))":   {Keyed: true, Value: CommandValue},
		"unique(path)":           {Unique: true, Value: PathValue},
		"list(text)":             {Value: TextValue},
		"list(skill)":            {},
		"keyed(unique(command))": {Keyed: true, Unique: true, Value: CommandValue},
	} {
		got, err := ParseType(source)
		if err != nil || got != want {
			t.Errorf("ParseType(%q) = %+v, %v; want %+v", source, got, err, want)
		}
		if reparsed, err := ParseType(got.String()); err != nil || reparsed != got {
			t.Errorf("round trip of %q via %q = %+v, %v", source, got.String(), reparsed, err)
		}
	}
	for _, source := range []string{"", "map", "keyed(list", "keyed(keyed(list))", "list(shell)", "list(command", "keyed(list(command)"} {
		if got, err := ParseType(source); err == nil {
			t.Errorf("ParseType(%q) = %+v; want an error", source, got)
		}
	}
}

func TestRead(t *testing.T) {
	frontmatter := map[string]any{"henia": map[string]any{
		"slots": map[string]any{"code.style": nil, "code.check": "keyed(list(command))", "bad": "map", "bad..name": nil},
		"provides": map[string]any{
			"code.style.go":     nil,
			"code.style.python": map[string]any{"section": "python", "priority": "fallback"},
			"code.check.go":     map[string]any{"command": "go vet ./..."},
			"code.check.rust":   map[string]any{"command": "cargo check", "text": "x"},
			"code.check.zig":    map[string]any{"priority": "urgent"},
		},
	}}
	body := "# Code\n\n:slot[code.style code.check]\n\n```md\n:slot[ignored]\n```\n\nUse :slot[code.style] again.\n"
	m := Read(frontmatter, body, Normal)
	if want := []Declaration{{"code.check", Type{Keyed: true, Value: CommandValue}}, {"code.style", Untyped}}; !reflect.DeepEqual(m.Declared, want) {
		t.Errorf("declared = %+v", m.Declared)
	}
	want := []Offer{
		{Slot: "code.check.go", Priority: Normal, Kind: CommandValue, Value: "go vet ./..."},
		{Slot: "code.style.go", Priority: Normal},
		{Slot: "code.style.python", Priority: Fallback, Explicit: true, Section: "python"},
	}
	if !reflect.DeepEqual(m.Offered, want) {
		t.Errorf("offered = %+v", m.Offered)
	}
	if want := []string{"code.style", "code.check"}; !reflect.DeepEqual(m.Applied, want) {
		t.Errorf("applied = %v", m.Applied)
	}
	var problems []string
	for _, p := range m.Problems {
		problems = append(problems, p.Role+" "+p.Slot)
	}
	if got := strings.Join(problems, ", "); got != "declare bad, declare bad..name, provide code.check.rust, provide code.check.zig" {
		t.Errorf("problems = %s", got)
	}
}

func TestExpand(t *testing.T) {
	body := "Providers:\n\n:slot[code.style code.check]\n\n`:slot[literal]`\n"
	if got := Expand(body, Preload); got != "Providers:\n\n!`henia slots code.style code.check`\n\n`:slot[literal]`\n" {
		t.Fatalf("Expand = %q", got)
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

func provider(ref string, tier Tier, offers ...Offer) Skill {
	return Skill{Path: ref + "/SKILL.md", Ref: ref, Tier: tier, Metadata: Metadata{Offered: offers}}
}

func TestTiersRank(t *testing.T) {
	skills := []Skill{
		provider("project:mine", Project, Offer{Slot: "git.commits", Priority: Project.priority()}),
		provider("acme:theirs", Dependency, Offer{Slot: "git.commits", Priority: Dependency.priority()}),
		provider("tropos:git", Global, Offer{Slot: "git.commits", Priority: Global.priority()}),
	}
	rows := Evaluate(skills, nil).Rows([]string{"git.commits"}, false)
	if len(rows) != 1 || rows[0] != (Row{Slot: "git.commits", Kind: "project", Path: "henia show project:mine"}) {
		t.Fatalf("rows = %+v", rows)
	}
	rows = Evaluate(skills[1:], nil).Rows([]string{"git.commits"}, false)
	if len(rows) != 1 || rows[0].Path != "henia show acme:theirs" {
		t.Fatalf("dependency over global: %+v", rows)
	}
}

func TestExplicitPriorityBeatsTier(t *testing.T) {
	skills := []Skill{
		provider("project:mine", Project, Offer{Slot: "git.commits", Priority: Normal}),
		provider("tropos:git", Global, Offer{Slot: "git.commits", Priority: Force, Explicit: true}),
	}
	rows := Evaluate(skills, nil).Rows([]string{"git.commits"}, false)
	if len(rows) != 1 || rows[0].Path != "henia show tropos:git" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDisabledProviders(t *testing.T) {
	skills := []Skill{
		provider("tropos:loqui", Global, Offer{Slot: "code.style.go", Priority: Fallback}, Offer{Slot: "code.style.python", Priority: Fallback, Section: "python"}),
		provider("tropos:other", Global, Offer{Slot: "code.style.go", Priority: Fallback}),
	}
	rows := Evaluate(skills, map[string][]string{"code.style.go": {"tropos:loqui"}}).Rows([]string{"code.style"}, false)
	var got []string
	for _, r := range rows {
		got = append(got, r.Line())
	}
	want := "code.style.python\tglobal\thenia show tropos:loqui#python\ncode.style.go\tglobal\thenia show tropos:other"
	if strings.Join(got, "\n") != want {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
	}
}

func TestResolveUniquePerKey(t *testing.T) {
	owner := Skill{Path: "owner", Ref: "tropos:owner", Tier: Global, Metadata: Metadata{Declared: []Declaration{{"code.style", Type{Keyed: true, Unique: true}}}}}
	skills := []Skill{
		owner,
		provider("a:go", Global, Offer{Slot: "code.style.go", Priority: Fallback}),
		provider("a:python", Global, Offer{Slot: "code.style.python", Priority: Fallback}),
		provider("b:python", Global, Offer{Slot: "code.style.python", Priority: Fallback}),
	}
	conflicts := Evaluate(skills, nil).Rows(nil, true)
	if len(conflicts) != 1 || conflicts[0] != (Row{Slot: "code.style.python", Kind: Conflict, Path: "-"}) {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestValueOffers(t *testing.T) {
	owner := Skill{Path: "owner", Ref: "tropos:code", Tier: Global, Metadata: Metadata{Declared: []Declaration{{"code.check", Type{Keyed: true, Value: CommandValue}}, {"code.style", Untyped}}}}
	skills := []Skill{
		owner,
		provider("tropos:loqui", Global, Offer{Slot: "code.check.go", Priority: Fallback, Kind: CommandValue, Value: "go vet ./..."}),
		provider("tropos:bad", Global, Offer{Slot: "code.check.rust", Priority: Fallback}, Offer{Slot: "code.style.go", Priority: Fallback, Kind: TextValue, Value: "x"}),
	}
	r := Evaluate(skills, nil)
	rows := r.Rows([]string{"code.check"}, false)
	if len(rows) < 1 || rows[0].Line() != "code.check.go\tglobal\thenia show tropos:loqui\tgo vet ./..." {
		t.Fatalf("rows = %+v", rows)
	}
	var invalid []string
	for _, p := range r.Providers {
		if p.Status == Invalid {
			invalid = append(invalid, p.Slot+": "+p.Reason)
		}
	}
	if got := strings.Join(invalid, "; "); got != "code.check.rust: code.check needs a command; code.style.go: code.style takes a skill, not a text" {
		t.Fatalf("invalid = %s", got)
	}
}
