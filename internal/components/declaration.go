package components

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"github.com/srnnkls/henia/internal/reference"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/srnnkls/henia/internal/artifact"
)

const (
	Container = "container"
	Leaf      = "leaf"
	Text      = "text"
)

const (
	Expand = "expand"
	Keep   = "keep"
)

var (
	forms       = []string{Container, Leaf, Text}
	types       = []string{"string", "boolean", "number", "list", "enum", "slot", "skill", "ref"}
	elements    = []string{"slot", "skill", "ref", "string"}
	bodyKinds   = []string{"markdown", "text", "none"}
	renderModes = []string{Expand, Keep}
)

type Declaration struct {
	Name       string
	Path       string
	Forms      []string
	Doc        string
	Attributes map[string]Attribute
	Body       Body
	Render     string
	Template   string
	Builtin    bool
	Open       bool
}

func (d Declaration) Allows(form string) bool { return slices.Contains(d.Forms, form) }

type Body struct {
	Kind     string
	Children []string
	Required bool
}

type Attribute struct {
	Type     string
	Of       string
	Required bool
	Default  any
	Values   []string
	Matches  string
	Doc      string
	pattern  *regexp.Regexp
}

type schema struct {
	Form       string                     `yaml:"form"`
	Doc        string                     `yaml:"doc"`
	Attributes map[string]attributeSchema `yaml:"attributes"`
	Body       bodySchema                 `yaml:"body"`
	Render     string                     `yaml:"render"`
}

type attributeSchema struct {
	Type     string   `yaml:"type"`
	Of       string   `yaml:"of"`
	Required bool     `yaml:"required"`
	Default  any      `yaml:"default"`
	Values   []string `yaml:"values"`
	Matches  string   `yaml:"matches"`
	Doc      string   `yaml:"doc"`
}

type bodySchema struct {
	Kind     string   `yaml:"kind"`
	Children []string `yaml:"children"`
	Required bool     `yaml:"required"`
}

func Parse(name string, source []byte) (Declaration, error) {
	header, body, err := artifact.SplitFrontmatter(string(source))
	if err != nil {
		return Declaration{}, err
	}
	var s schema
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(header)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return Declaration{}, fmt.Errorf("schema: %w", err)
	}
	if !slices.Contains(forms, s.Form) {
		return Declaration{}, fmt.Errorf("form %q: want one of %s", s.Form, strings.Join(forms, ", "))
	}
	decl := Declaration{
		Name:       name,
		Forms:      []string{s.Form},
		Doc:        s.Doc,
		Attributes: make(map[string]Attribute, len(s.Attributes)),
		Body:       Body{Kind: cmp.Or(s.Body.Kind, "markdown"), Children: s.Body.Children, Required: s.Body.Required},
		Render:     cmp.Or(s.Render, Expand),
		Template:   body,
	}
	if !slices.Contains(bodyKinds, decl.Body.Kind) {
		return Declaration{}, fmt.Errorf("body.kind %q: want one of %s", decl.Body.Kind, strings.Join(bodyKinds, ", "))
	}
	if !slices.Contains(renderModes, decl.Render) {
		return Declaration{}, fmt.Errorf("render %q: want one of %s", decl.Render, strings.Join(renderModes, ", "))
	}
	for attrName, as := range s.Attributes {
		attr, err := newAttribute(as)
		if err != nil {
			return Declaration{}, fmt.Errorf("attribute %s: %w", attrName, err)
		}
		decl.Attributes[attrName] = attr
	}
	return decl, nil
}

func newAttribute(s attributeSchema) (Attribute, error) {
	attr := Attribute{Type: s.Type, Of: s.Of, Required: s.Required, Values: s.Values, Matches: s.Matches, Doc: s.Doc}
	if !slices.Contains(types, s.Type) {
		return Attribute{}, fmt.Errorf("type %q: want one of %s", s.Type, strings.Join(types, ", "))
	}
	if s.Of != "" && (s.Type != "list" || !slices.Contains(elements, s.Of)) {
		return Attribute{}, fmt.Errorf("of %q: only a list takes one of %s", s.Of, strings.Join(elements, ", "))
	}
	if (s.Type == "enum") != (len(s.Values) > 0) {
		return Attribute{}, fmt.Errorf("values: an enum and only an enum takes values")
	}
	if s.Matches != "" {
		pattern, err := regexp.Compile(`^(?:` + s.Matches + `)$`)
		if err != nil {
			return Attribute{}, fmt.Errorf("matches: %w", err)
		}
		attr.pattern = pattern
	}
	if s.Default != nil {
		raw, ok := defaultText(s.Default)
		if !ok {
			return Attribute{}, fmt.Errorf("default %v: not a scalar or list", s.Default)
		}
		value, err := attr.Coerce(raw)
		if err != nil {
			return Attribute{}, fmt.Errorf("default: %w", err)
		}
		attr.Default = value
	}
	return attr, nil
}

func defaultText(v any) (string, bool) {
	switch d := v.(type) {
	case string:
		return d, true
	case bool:
		return strconv.FormatBool(d), true
	case int:
		return strconv.Itoa(d), true
	case float64:
		return strconv.FormatFloat(d, 'g', -1, 64), true
	case []any:
		items := make([]string, len(d))
		for i, item := range d {
			text, ok := defaultText(item)
			if !ok {
				return "", false
			}
			items[i] = text
		}
		return strings.Join(items, " "), true
	}
	return "", false
}

func (a Attribute) Coerce(raw string) (any, error) {
	switch a.Type {
	case "boolean":
		if raw == "" {
			raw = "true"
		}
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%q is not a boolean", raw)
		}
		return value, a.match(raw)
	case "number":
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", raw)
		}
		return value, a.match(raw)
	case "enum":
		if !slices.Contains(a.Values, raw) {
			return nil, fmt.Errorf("%q is not one of %s", raw, strings.Join(a.Values, ", "))
		}
		return raw, a.match(raw)
	case "list":
		items := strings.Fields(raw)
		for _, item := range items {
			if err := address(a.Of, item); err != nil {
				return nil, err
			}
			if err := a.match(item); err != nil {
				return nil, err
			}
		}
		return items, nil
	}
	if err := address(a.Type, raw); err != nil {
		return nil, err
	}
	return raw, a.match(raw)
}

func address(kind, raw string) error {
	if kind != "ref" && kind != "skill" {
		return nil
	}
	a, err := reference.ParseAddress(raw)
	if err != nil {
		return err
	}
	if kind == "skill" && (a.Kind != reference.TypeSkill || a.Resource != "" || a.Anchor != "") {
		return fmt.Errorf("%q is not a skill name", raw)
	}
	return nil
}

func (a Attribute) Zero() any {
	if a.Default != nil {
		return a.Default
	}
	switch a.Type {
	case "boolean":
		return false
	case "number":
		return 0.0
	case "list":
		return []string{}
	}
	return ""
}

func (a Attribute) match(value string) error {
	if a.pattern != nil && !a.pattern.MatchString(value) {
		return fmt.Errorf("%q does not match %s", value, a.Matches)
	}
	return nil
}
