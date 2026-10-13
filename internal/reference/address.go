package reference

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/pattern"
)

var Sigils = map[string]Type{"$": TypeSkill, "/": TypeCommand, "@": TypeAgent, "#": TypeFile, "!": TypeTool}

var addressable = []Type{TypeSkill, TypeAgent, TypeCommand}

func Reserved(pkg string) bool { return slices.Contains(addressable, Type(pkg)) }

func SigilTable() pattern.Table {
	var t pattern.Table
	for _, sigil := range slices.Sorted(maps.Keys(Sigils)) {
		t.Keys, t.Values = append(t.Keys, sigil), append(t.Values, string(Sigils[sigil]))
	}
	return t
}

type Address struct {
	Kind                    Type
	Package, Name, Resource string
	Anchor                  string
}

func ParseAddress(s string) (Address, error) {
	a := Address{Kind: TypeSkill}
	rest, anchor, _ := strings.Cut(s, "#")
	a.Anchor = anchor
	if kind, ok := Sigils[rest[:min(1, len(rest))]]; ok {
		if !slices.Contains(addressable, kind) {
			return Address{}, fmt.Errorf("%s names a %s, which has no document", s, kind)
		}
		a.Kind, rest = kind, rest[1:]
	} else if kind, name, ok := strings.Cut(rest, ":"); ok && Reserved(kind) {
		a.Kind, rest = Type(kind), name
	}
	if pkg, name, ok := strings.Cut(rest, ":"); ok {
		a.Package, rest = pkg, name
	}
	a.Name = rest
	if a.Kind == TypeSkill {
		if i := strings.IndexAny(rest, "/."); i >= 0 {
			a.Name, a.Resource = rest[:i], rest[i:]
		}
	}
	if !artifactName.MatchString(a.Name) || strings.Contains(a.Package, ":") {
		return Address{}, fmt.Errorf("%s is not an address", s)
	}
	return a, nil
}

func (a Address) String() string {
	s := string(a.Kind) + ":"
	if a.Package != "" {
		s += a.Package + ":"
	}
	s += a.Name + a.Resource
	if a.Anchor != "" {
		s += "#" + a.Anchor
	}
	return s
}

func (a Address) Modules() []string {
	if module, ok := strings.CutPrefix(a.Resource, "."); ok {
		return strings.Split(module, ".")
	}
	return nil
}

func (a Address) Path() string {
	if path, ok := strings.CutPrefix(a.Resource, "/"); ok {
		return path
	}
	return ""
}

func (a Address) Ref() string {
	if a.Package != "" {
		return a.Package + ":" + a.Name
	}
	return a.Name
}
