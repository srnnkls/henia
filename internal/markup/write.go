package markup

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Write serializes a document from its tree. Unchanged nodes keep their source
// bytes; container fences take canonical lengths, each outer fence one colon
// longer than the longest fence it contains. Grafted nodes write their own
// origin span; changed and constructed nodes are written from their fields.
func Write(doc *Document) []byte {
	w := &writer{}
	if doc.parsed == nil {
		w.blocks(doc.Children, "\n\n")
		if w.out.Len() > 0 {
			w.out.WriteByte('\n')
		}
	} else {
		w.region(doc.parsed, doc.Children)
	}
	return w.out.Bytes()
}

type writer struct{ out bytes.Buffer }

func (w *writer) region(p *parsed, children []*Item) {
	var prev *Item
	for i, c := range children {
		switch {
		case c.parsed != nil && (p.inline && slices.Contains(p.children, c) || !p.inline && c.parsed.pred == prev):
			w.out.Write(c.parsed.gap)
		case p.inline:
		case i > 0:
			w.out.WriteString(p.sep)
		default:
			w.out.WriteString(p.lead)
		}
		if c.parsed == nil && p.list {
			w.prefixed(c, marker(p.attrs, i), strings.Repeat(" ", len(marker(p.attrs, i))))
		} else {
			w.item(c)
		}
		prev = c
	}
	trail := p.src.text[p.trail:p.hi]
	if len(children) == 0 && len(p.children) > 0 && p.lo > 0 && p.src.text[p.lo-1] == '\n' {
		trail = trail[bytes.IndexByte(trail, '\n')+1:]
	}
	w.out.Write(trail)
}

func (w *writer) item(it *Item) {
	p := it.parsed
	if p == nil {
		w.generated(it)
		return
	}
	retext, reattr := it.Text != p.text, !maps.Equal(it.Attrs, p.attrs)
	if p.directive == nil {
		if (retext || reattr) && p.inline {
			w.generated(it)
			return
		}
		w.out.Write(p.src.text[p.start:p.lo])
		w.region(p, it.Children)
		return
	}
	head, tail := fences(it, retext || reattr)
	w.out.WriteString(head)
	if retext {
		w.out.WriteString(it.Text)
	} else {
		w.region(p, it.Children)
	}
	w.out.WriteString(tail)
}

func fences(it *Item, rewrite bool) (string, string) {
	p, d := it.parsed, it.parsed.directive
	text, name := p.src.text, it.Attrs["name"]
	if d.fence >= 3 {
		colons := strings.Repeat(":", fenceLength(it))
		head := colons + string(text[d.open.start+d.fence:p.lo])
		if rewrite {
			head = colons + name + attributeText(it.Attrs) + string(text[d.open.stop:p.lo])
		}
		if !d.closed {
			return head, string(text[p.hi:p.end])
		}
		return head, colons + string(bytes.TrimLeft(text[d.close.start:p.end], ":"))
	}
	if !rewrite {
		return string(text[p.start:p.lo]), string(text[p.hi:p.end])
	}
	open, close := strings.Repeat(":", max(d.fence, 1))+name, attributeText(it.Attrs)
	if d.labeled || it.Text != "" {
		open, close = open+"[", "]"+close
	}
	return open, close
}

func (w *writer) generated(it *Item) {
	name, attrs := it.Attrs["name"], attributeText(it.Attrs)
	switch {
	case it.Kind == "directive" && it.Attrs["inline"] == "true":
		w.out.WriteString(":" + name + "[")
		w.label(it)
		w.out.WriteString("]" + attrs)
	case container(it):
		colons := strings.Repeat(":", fenceLength(it))
		w.out.WriteString(colons + name + attrs + "\n")
		w.blocks(it.Children, "\n\n")
		if len(it.Children) > 0 {
			w.out.WriteByte('\n')
		}
		w.out.WriteString(colons)
	case it.Kind == "directive":
		w.out.WriteString("::" + name)
		if it.Text != "" || len(it.Children) > 0 {
			w.out.WriteByte('[')
			w.label(it)
			w.out.WriteByte(']')
		}
		w.out.WriteString(attrs)
	case it.Kind == "heading":
		level, _ := strconv.Atoi(it.Attrs["level"])
		w.out.WriteString(strings.Repeat("#", max(level, 1)) + " " + it.Text)
	case it.Kind == "code" && it.Attrs["inline"] != "true":
		info := it.Attrs["info"]
		if info == "" {
			info = it.Attrs["lang"]
		}
		w.out.WriteString("```" + info + "\n" + it.Text)
		if !strings.HasSuffix(it.Text, "\n") {
			w.out.WriteByte('\n')
		}
		w.out.WriteString("```")
	case it.Kind == "list":
		for i, c := range it.Children {
			if i > 0 {
				w.out.WriteByte('\n')
			}
			w.prefixed(c, marker(it.Attrs, i), strings.Repeat(" ", len(marker(it.Attrs, i))))
		}
	case it.Kind == "quote":
		inner := &writer{}
		inner.blocks(it.Children, "\n\n")
		w.prefix(inner.out.String(), "> ", "> ")
	case it.Text == "":
		w.blocks(it.Children, "\n\n")
	default:
		w.out.WriteString(it.Text)
	}
}

func marker(list map[string]string, i int) string {
	if list["ordered"] == "true" {
		return strconv.Itoa(i+1) + ". "
	}
	return "- "
}

func (w *writer) label(it *Item) {
	if it.Text != "" || len(it.Children) == 0 {
		w.out.WriteString(it.Text)
		return
	}
	for _, c := range it.Children {
		w.item(c)
	}
}

func (w *writer) blocks(items []*Item, separator string) {
	for i, c := range items {
		if i > 0 {
			w.out.WriteString(separator)
		}
		w.item(c)
	}
}

func (w *writer) prefixed(it *Item, first, rest string) {
	inner := &writer{}
	inner.item(it)
	w.prefix(inner.out.String(), first, rest)
}

func (w *writer) prefix(text, first, rest string) {
	for i, line := range strings.Split(text, "\n") {
		lead := rest
		if i == 0 {
			lead = first
		} else {
			w.out.WriteByte('\n')
		}
		if line == "" {
			lead = strings.TrimRight(lead, " ")
		}
		w.out.WriteString(lead + line)
	}
}

func container(it *Item) bool {
	if it.Kind != "directive" || it.Attrs["inline"] == "true" {
		return false
	}
	if it.parsed != nil && it.parsed.directive != nil {
		return it.parsed.directive.fence >= 3
	}
	return it.parsed == nil && len(it.Children) > 0
}

func fenceLength(it *Item) int {
	inner := 0
	var visit func([]*Item)
	visit = func(items []*Item) {
		for _, c := range items {
			if container(c) {
				inner = max(inner, fenceLength(c))
				continue
			}
			visit(c.Children)
		}
	}
	visit(it.Children)
	return max(3, inner+1)
}

func attributeText(attrs map[string]string) string {
	var parts []string
	if id, ok := attrs["id"]; ok {
		parts = append(parts, shorthand("#", "id", id))
	}
	if class, ok := attrs["class"]; ok {
		for _, c := range strings.Fields(class) {
			parts = append(parts, shorthand(".", "class", c))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		switch k {
		case "name", "inline", "id", "class":
		default:
			parts = append(parts, k+"="+attributeValueText(attrs[k]))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func shorthand(marker, name, value string) string {
	if value != "" && bareLength([]byte(value)) == len(value) {
		return marker + value
	}
	return name + "=" + attributeValueText(value)
}

func attributeValueText(value string) string {
	if value != "" && bareLength([]byte(value)) == len(value) {
		return value
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(value)
	return strings.TrimSuffix(b.String(), "\n")
}
