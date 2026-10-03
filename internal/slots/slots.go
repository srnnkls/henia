// Package slots resolves typed skill slots: skills declare slots in
// metadata.slots and fill them in metadata.provides.
package slots

import (
	"fmt"
	"regexp"
	"strings"
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
	Force    Priority = 50
	Normal   Priority = 100
	Fallback Priority = 1000
)

var priorities = map[string]Priority{"force": Force, "normal": Normal, "fallback": Fallback}

func (t Type) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

func (p Priority) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

func (p Priority) String() string {
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

type Definition struct {
	Slot     string
	Priority Priority
}

var name = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

func validName(slot string) error {
	if !name.MatchString(slot) {
		return fmt.Errorf("invalid slot name %q", slot)
	}
	return nil
}

// ParseDeclaration parses a metadata.slots entry, name[:type].
func ParseDeclaration(entry string) (Declaration, error) {
	slot, source, typed := strings.Cut(entry, ":")
	if err := validName(slot); err != nil {
		return Declaration{}, err
	}
	d := Declaration{Slot: slot, Type: Untyped}
	if typed {
		var err error
		if d.Type, err = ParseType(source); err != nil {
			return Declaration{}, fmt.Errorf("slot %s: %w", slot, err)
		}
	}
	return d, nil
}

// ParseApplication parses a metadata.applies entry, a slot name.
func ParseApplication(entry string) (string, error) {
	return entry, validName(entry)
}

// ParseDefinition parses a metadata.provides entry, name[@priority].
func ParseDefinition(entry string, implied Priority) (Definition, error) {
	slot, source, explicit := strings.Cut(entry, "@")
	if err := validName(slot); err != nil {
		return Definition{}, err
	}
	d := Definition{Slot: slot, Priority: implied}
	if explicit {
		var ok bool
		if d.Priority, ok = priorities[source]; !ok {
			return Definition{}, fmt.Errorf("slot %s: unknown priority %q (use force, normal or fallback)", slot, source)
		}
	}
	return d, nil
}

func Within(name, slot string) bool {
	return name == slot || strings.HasPrefix(name, slot+".")
}

func related(a, b string) bool { return Within(a, b) || Within(b, a) }

func Names(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		return strings.Fields(v), nil
	case []string:
		var names []string
		for _, s := range v {
			names = append(names, strings.Fields(s)...)
		}
		return names, nil
	case []any:
		var names []string
		for _, item := range v {
			switch s := item.(type) {
			case nil:
			case string:
				names = append(names, strings.Fields(s)...)
			default:
				return nil, fmt.Errorf("list item is %T, expected string", item)
			}
		}
		return names, nil
	}
	return nil, fmt.Errorf("value is %T, expected a string or a list of strings", value)
}

type Metadata struct {
	Declared []string
	Provided []string
	Applied  []string
	Values   map[string]string
}

func Entries(frontmatter map[string]any) (Metadata, error) {
	metadata, _ := frontmatter["metadata"].(map[string]any)
	var m Metadata
	for _, field := range []struct {
		key     string
		entries *[]string
	}{{"slots", &m.Declared}, {"provides", &m.Provided}, {"applies", &m.Applied}} {
		names, err := Names(metadata[field.key])
		if err != nil {
			return Metadata{}, fmt.Errorf("metadata.%s: %w", field.key, err)
		}
		*field.entries = names
	}
	for key, value := range metadata {
		if text, ok := value.(string); ok && strings.Contains(key, ".") {
			if m.Values == nil {
				m.Values = make(map[string]string)
			}
			m.Values[key] = text
		}
	}
	return m, nil
}
