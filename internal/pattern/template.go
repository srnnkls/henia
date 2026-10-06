package pattern

import (
	"strconv"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

type Template struct {
	parts []templatePart
}

type templatePart struct {
	literal          string
	sigil, name, key string
}

func ParseTemplate(text string) Template {
	var t Template
	last := 0
	for _, m := range placeholder.FindAllStringSubmatchIndex(text, -1) {
		t.parts = append(t.parts, templatePart{literal: text[last:m[0]]})
		part := templatePart{sigil: text[m[2]:m[3]], name: text[m[4]:m[5]]}
		if m[6] >= 0 {
			part.key = text[m[6]:m[7]]
		}
		t.parts = append(t.parts, part)
		last = m[1]
	}
	t.parts = append(t.parts, templatePart{literal: text[last:]})
	return t
}

func (t Template) Placeholders() []string {
	var names []string
	for _, p := range t.parts {
		if p.sigil != "" {
			names = append(names, strings.TrimSuffix(p.sigil+p.name+"."+p.key, "."))
		}
	}
	return names
}

func (t Template) Render(row Row, params map[string]Value, capture func(e *markup.Element, key string) string) string {
	if capture == nil {
		capture = Field
	}
	var b strings.Builder
	for _, p := range t.parts {
		switch p.sigil {
		case "":
			b.WriteString(p.literal)
		case "$":
			b.WriteString(params[p.name].String())
		case "?":
			b.WriteString(formatVar(row.Vars[p.name], p.key))
		case "@":
			if e := Captured(row, p.name); e != nil {
				b.WriteString(capture(e, p.key))
			}
		}
	}
	return b.String()
}

func Field(e *markup.Element, key string) string {
	if key == "" || key == "text" {
		return e.Text
	}
	return e.Attrs[key]
}

func Captured(row Row, name string) *markup.Element {
	for _, cell := range row.Cells {
		if (name == "" || cell.Name == name) && len(cell.Elements) > 0 {
			return cell.Elements[0]
		}
	}
	return nil
}

func formatVar(value, format string) string {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || format == "" {
		return value
	}
	if format == "percent" {
		return strconv.FormatFloat(n*100, 'f', 1, 64) + "%"
	}
	if digits, err := strconv.Atoi(format); err == nil {
		return strconv.FormatFloat(n, 'f', digits, 64)
	}
	return value
}
