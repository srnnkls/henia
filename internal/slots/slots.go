// Package slots resolves typed skill slots: skills declare slots in
// henia.slots, fill them in henia.provides and apply them with :slot[...].
package slots

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

// Type is a slot's composer.
type Type struct {
	Keyed  bool
	Unique bool
	Value  Value
}

type Value string

const (
	SkillValue   Value = ""
	CommandValue Value = "command"
	TextValue    Value = "text"
	PathValue    Value = "path"
)

var Untyped = Type{Keyed: true}

func (t Type) String() string {
	base := "list"
	if t.Unique {
		base = "unique"
	}
	if t.Value != SkillValue {
		base += "(" + string(t.Value) + ")"
	}
	if t.Keyed {
		return "keyed(" + base + ")"
	}
	return base
}

func ParseType(source string) (Type, error) {
	inner, keyed := strings.CutPrefix(source, "keyed(")
	if keyed {
		var closed bool
		if inner, closed = strings.CutSuffix(inner, ")"); !closed {
			return Type{}, fmt.Errorf("unclosed type %q", source)
		}
	}
	t := Type{Keyed: keyed}
	base, value, valued := strings.Cut(inner, "(")
	if valued {
		var closed bool
		if value, closed = strings.CutSuffix(value, ")"); !closed {
			return Type{}, fmt.Errorf("unclosed type %q", source)
		}
		switch Value(value) {
		case CommandValue, TextValue, PathValue:
			t.Value = Value(value)
		case "skill":
		default:
			return Type{}, fmt.Errorf("unknown value %q in type %q (use skill, command, text or path)", value, source)
		}
	}
	switch base {
	case "list":
		return t, nil
	case "unique":
		t.Unique = true
		return t, nil
	}
	return Type{}, fmt.Errorf("unknown type %q (use list, unique or keyed(...), each optionally of command, text or path)", source)
}

// Priority orders definitions of one slot; lower values win.
type Priority int

const (
	Force     Priority = 50
	Normal    Priority = 100
	inherited Priority = 500
	Fallback  Priority = 1000
)

var priorities = map[string]Priority{"force": Force, "normal": Normal, "fallback": Fallback}

func (t Type) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (p Priority) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

func (p Priority) String() string {
	if p == inherited {
		return "dependency"
	}
	for name, value := range priorities {
		if value == p {
			return name
		}
	}
	return fmt.Sprint(int(p))
}

type Declaration struct {
	Slot string
	Type Type
}

type Offer struct {
	Slot     string
	Priority Priority
	Explicit bool
	Section  string
	Value    string
	Kind     Value
}

var name = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

func validName(slot string) error {
	if !name.MatchString(slot) {
		return fmt.Errorf("invalid slot name %q", slot)
	}
	return nil
}

func Within(name, slot string) bool {
	return name == slot || strings.HasPrefix(name, slot+".")
}

func related(a, b string) bool { return Within(a, b) || Within(b, a) }

type Metadata struct {
	Declared []Declaration
	Offered  []Offer
	Applied  []string
	Problems []Problem
}

type Problem struct {
	Slot    string
	Role    string
	Message string
}

func Read(frontmatter map[string]any, body string, implied Priority) Metadata {
	var m Metadata
	henia, _ := frontmatter["henia"].(map[string]any)
	declared, err := table(henia, "slots")
	if err != nil {
		m.Problems = append(m.Problems, Problem{Role: "declare", Message: err.Error()})
	}
	for _, slot := range sortedKeys(declared) {
		d, err := declaration(slot, declared[slot])
		if err != nil {
			m.Problems = append(m.Problems, Problem{Slot: slot, Role: "declare", Message: err.Error()})
			continue
		}
		m.Declared = append(m.Declared, d)
	}
	provided, err := table(henia, "provides")
	if err != nil {
		m.Problems = append(m.Problems, Problem{Role: "provide", Message: err.Error()})
	}
	for _, slot := range sortedKeys(provided) {
		o, err := offer(slot, provided[slot], implied)
		if err != nil {
			m.Problems = append(m.Problems, Problem{Slot: slot, Role: "provide", Message: err.Error()})
			continue
		}
		m.Offered = append(m.Offered, o)
	}
	for _, application := range Applications(body) {
		for _, slot := range application.Slots {
			if err := validName(slot); err != nil {
				m.Problems = append(m.Problems, Problem{Slot: slot, Role: "apply", Message: err.Error()})
				continue
			}
			if !slices.Contains(m.Applied, slot) {
				m.Applied = append(m.Applied, slot)
			}
		}
	}
	return m
}

func table(henia map[string]any, key string) (map[string]any, error) {
	switch v := henia[key].(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return v, nil
	}
	return nil, fmt.Errorf("henia.%s must be a map of slot names", key)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func declaration(slot string, value any) (Declaration, error) {
	if err := validName(slot); err != nil {
		return Declaration{}, err
	}
	switch v := value.(type) {
	case nil:
		return Declaration{slot, Untyped}, nil
	case string:
		t, err := ParseType(v)
		if err != nil {
			return Declaration{}, fmt.Errorf("slot %s: %w", slot, err)
		}
		return Declaration{slot, t}, nil
	}
	return Declaration{}, fmt.Errorf("slot %s: the type is %T, expected a string such as keyed(list)", slot, value)
}

func offer(slot string, value any, implied Priority) (Offer, error) {
	if err := validName(slot); err != nil {
		return Offer{}, err
	}
	o := Offer{Slot: slot, Priority: implied}
	fields, ok := value.(map[string]any)
	if value != nil && !ok {
		return Offer{}, fmt.Errorf("slot %s: the offer is %T, expected a map with priority, section, command, text or path", slot, value)
	}
	for _, key := range sortedKeys(fields) {
		text, ok := fields[key].(string)
		if !ok {
			return Offer{}, fmt.Errorf("slot %s: %s is %T, expected a string", slot, key, fields[key])
		}
		switch key {
		case "priority":
			if o.Priority, ok = priorities[text]; !ok {
				return Offer{}, fmt.Errorf("slot %s: unknown priority %q (use force, normal or fallback)", slot, text)
			}
			o.Explicit = true
		case "section":
			o.Section = text
		case string(CommandValue), string(TextValue), string(PathValue):
			if o.Kind != SkillValue {
				return Offer{}, fmt.Errorf("slot %s: an offer takes one of command, text and path", slot)
			}
			o.Kind, o.Value = Value(key), text
		default:
			return Offer{}, fmt.Errorf("slot %s: unknown key %q (use priority, section, command, text or path)", slot, key)
		}
	}
	if o.Section != "" && o.Kind != SkillValue {
		return Offer{}, fmt.Errorf("slot %s: a section offers the skill, not a %s", slot, o.Kind)
	}
	return o, nil
}

type Application struct {
	Slots      []string
	Start, End int
}

var applicationText = regexp.MustCompile(`^:slot\[([^\]]*)\]`)

func Applications(body string) []Application {
	root, _ := markup.Tree([]byte(body))
	return ApplicationsIn(root)
}

func ApplicationsIn(root *markup.Element) []Application {
	var found []Application
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "directive" && e.Attrs["name"] == "slot" && e.Attrs["inline"] == "true" {
			if match := applicationText.FindStringSubmatch(e.Text); match != nil && !strings.Contains(match[1], "{{") {
				found = append(found, Application{Slots: strings.Fields(match[1]), Start: e.Start, End: e.End})
			}
		}
		return true
	})
	return found
}

func Preload(slots []string) string {
	return "!`henia slots " + strings.Join(slots, " ") + "`"
}
