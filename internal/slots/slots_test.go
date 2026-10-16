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
	if got := Expand(body, Preload); got != "Providers:\n\n!`henia slots --markdown code.style code.check`\n\n`:slot[literal]`\n" {
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
	if len(rows) != 1 || rows[0].Line() != "git.commits\tproject\thenia show project:mine" {
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
	want := "code.style.python\tglobal\thenia show 'tropos:loqui#python'\ncode.style.go\tglobal\thenia show tropos:other"
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

func lines(rows []Row) string {
	var got []string
	for _, r := range rows {
		got = append(got, r.Line())
	}
	return strings.Join(got, "\n")
}

func TestUniqueMostSpecificWins(t *testing.T) {
	for _, typ := range []Type{{Unique: true, Keyed: true}, {Keyed: true}} {
		owner := Skill{Path: "owner", Ref: "tropos:test", Tier: Global, Metadata: Metadata{Declared: []Declaration{{"test.approach", typ}}}}
		r := Evaluate([]Skill{
			owner,
			provider("tropos:root", Global, Offer{Slot: "test.approach", Priority: Fallback}),
			provider("tropos:frontend", Global, Offer{Slot: "test.approach.frontend", Priority: Fallback}),
		}, nil)
		root, frontend := "test.approach\tglobal\thenia show tropos:root", "test.approach.frontend\tglobal\thenia show tropos:frontend"
		want := map[string]string{
			"test.approach.frontend": frontend,
			"test.approach.backend":  root,
			"test.approach":          root + "\n" + frontend,
		}
		if !typ.Unique {
			want["test.approach.frontend"] = root + "\n" + frontend
		}
		for request, rows := range want {
			if got := lines(r.Rows([]string{request}, false)); got != rows {
				t.Errorf("%s: %s:\n%s\nwant:\n%s", typ, request, got, rows)
			}
		}
		if got := lines(r.Rows(nil, true)); got != "" {
			t.Errorf("%s: problems = %s", typ, got)
		}
	}
}

func TestUniqueSupersededExplained(t *testing.T) {
	owner := Skill{Path: "owner", Ref: "tropos:test", Tier: Global, Metadata: Metadata{Declared: []Declaration{{"test.approach", Type{Keyed: true, Unique: true}}}}}
	r := Evaluate([]Skill{
		owner,
		provider("tropos:root", Global, Offer{Slot: "test.approach", Priority: Fallback}),
		provider("tropos:frontend", Global, Offer{Slot: "test.approach.frontend", Priority: Fallback}),
	}, nil)
	var b strings.Builder
	if err := r.Explain(&b, []string{"test.approach.frontend"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "shadowed tropos:root [global, fallback from tier] tropos:root/SKILL.md\n      by tropos:frontend [global, fallback from tier] for test.approach.frontend, more specific,") {
		t.Errorf("explain:\n%s", b.String())
	}
	selected := r.Select([]string{"test.approach.frontend"})
	var statuses []string
	for _, p := range selected.Providers {
		status := p.Ref + " " + p.Status
		if p.ShadowedBy != nil {
			status += " by " + p.ShadowedBy.Ref
		}
		statuses = append(statuses, status)
	}
	if got := strings.Join(statuses, ", "); got != "tropos:root shadowed by tropos:frontend, tropos:frontend selected" {
		t.Errorf("select = %s", got)
	}
	if r.Providers[1].Status != Selected {
		t.Errorf("Select changed the resolution: %s", r.Providers[1].Status)
	}
	if got := r.Select([]string{"test.approach.frontend", "test.approach"}).Providers[0].Status; got != Selected {
		t.Errorf("root requested too: %s", got)
	}
}

func TestProvide(t *testing.T) {
	owner := Skill{Path: "owner", Ref: "tropos:implement", Tier: Global, Metadata: Metadata{Declared: []Declaration{{"implement.approach", Untyped}}}}
	skills := []Skill{
		owner,
		provider("ponytail:ponytail", Dependency),
		provider("tropos:loqui", Global),
	}
	provided := Provide(skills, map[string][]string{
		"implement.approach": {"ponytail:ponytail", "tropos:loqui#tdd", "ghost:skill"},
		"bad..slot":          {"tropos:loqui"},
	}, "henia.toml")
	r := Evaluate(append(skills, provided...), nil)
	want := "implement.approach\tdependency\thenia show ponytail:ponytail\n" +
		"bad..slot\tinvalid\thenia.toml\n" +
		"implement.approach\tinvalid\thenia.toml\n" +
		"bad..slot\tundeclared\t-"
	if got := lines(r.Rows([]string{"implement.approach", "bad..slot"}, false)); got != want {
		t.Errorf("rows:\n%s\nwant:\n%s", got, want)
	}
	for _, p := range r.Providers {
		if p.Ref == "tropos:loqui" && (p.Section != "tdd" || p.Priority != Fallback || p.Status != Shadowed) {
			t.Errorf("loqui = %+v", p)
		}
	}
	if got := lines(Evaluate(append(skills, provided...), map[string][]string{"implement": {"ponytail:ponytail"}}).Rows([]string{"implement.approach"}, false)); !strings.HasPrefix(got, "implement.approach\tglobal\thenia show 'tropos:loqui#tdd'") {
		t.Errorf("disabled ponytail:\n%s", got)
	}
}

func TestProvideOnceAcrossConfigLayers(t *testing.T) {
	owner := Skill{Path: "owner", Ref: "tropos:code", Tier: Global, Metadata: Metadata{Declared: []Declaration{{"code.orientation", Untyped}}}}
	skills := []Skill{owner, provider("gestalt:gestalt", Dependency)}
	provided := Provide(skills, map[string][]string{"code.orientation": {"gestalt:gestalt#orient", "gestalt:gestalt#orient"}}, "henia.toml")
	want := "code.orientation\tdependency\thenia show 'gestalt:gestalt#orient'"
	if got := lines(Evaluate(append(skills, provided...), nil).Rows([]string{"code.orientation"}, false)); got != want {
		t.Errorf("rows:\n%s\nwant:\n%s", got, want)
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

func TestMarkdown(t *testing.T) {
	r := Evaluate([]Skill{
		{Path: "/lib/house/SKILL.md", Ref: "project:house", Tier: Project, Metadata: Metadata{
			Declared: []Declaration{{Slot: "code.validation", Type: Type{Keyed: true, Value: CommandValue}}, {Slot: "git.commits"}, {Slot: "review.criteria"}},
			Offered:  []Offer{{Slot: "code.validation.go", Priority: Normal, Value: "go test ./...", Kind: CommandValue}, {Slot: "git.commits", Priority: Normal, Section: "commits"}},
		}},
	}, nil)
	want := "- `code.validation.go`: `go test ./...`\n- `git.commits`: `henia show 'project:house#commits'`\n- `review.criteria`: no provider\n- `lint.rules`: undeclared\n"
	if got := r.Markdown([]string{"code.validation", "git.commits", "review.criteria", "lint.rules"}); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	inline := r.Inline([]string{"git.commits", "code.validation", "review.criteria"}, func(p *Provider, kind Value) string {
		return string(kind) + ":" + p.address() + ":" + p.Value
	})
	if inline != ":project:house#commits:\n\ncommand:project:house:go test ./...\n" {
		t.Fatalf("inline: %q", inline)
	}
}

func TestApplicationsInline(t *testing.T) {
	got := Expand("A :slot[a b]\n\n:slot[c]{.inline}\n", Preload)
	if got != "A !`henia slots --markdown a b`\n\n!`henia slots --inline c`\n" {
		t.Fatalf("expand: %q", got)
	}
}

func TestExpandNestedApplication(t *testing.T) {
	if got := Expand(":note[:slot[a]] :slot[b]\n", Preload); got != ":note[!`henia slots --markdown a`] !`henia slots --markdown b`\n" {
		t.Fatalf("Expand = %q", got)
	}
}

func TestInlineDedupesProviders(t *testing.T) {
	r := Evaluate([]Skill{
		{Path: "/lib/code/SKILL.md", Ref: "tropos:code", Tier: Global, Metadata: Metadata{Declared: []Declaration{{Slot: "code.style", Type: Type{Keyed: true}}}}},
		provider("loqui:loqui", Dependency,
			Offer{Slot: "code.style.go", Priority: Dependency.priority()},
			Offer{Slot: "code.style.python", Priority: Dependency.priority()},
			Offer{Slot: "code.style.rust", Priority: Dependency.priority()}),
		provider("project:effect", Project, Offer{Slot: "code.style.typescript", Priority: Project.priority()}),
	}, nil)
	got := r.Inline([]string{"code.style"}, func(p *Provider, _ Value) string { return "# " + p.Ref + "\n" })
	want := "# project:effect\n\n`code.style.go`, `code.style.python`, `code.style.rust`:\n\n# loqui:loqui\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
