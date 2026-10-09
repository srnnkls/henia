package markup

import "strings"

func Unwrap(source, name string) (string, []string) {
	if !strings.Contains(source, name) {
		return source, nil
	}
	root, _ := Tree([]byte(source))
	type block struct {
		start, end int
		inner      string
	}
	var blocks []block
	var visit func(*Element)
	visit = func(e *Element) {
		if e.Type != "directive" || e.Attrs["name"] != name || e.Attrs["inline"] == "true" {
			for _, child := range e.Children {
				visit(child)
			}
			return
		}
		_, rest, _ := strings.Cut(source[e.Start:e.End], "\n")
		inner := ""
		if i := strings.LastIndexByte(rest, '\n'); i >= 0 {
			inner = rest[:i+1]
		}
		blocks = append(blocks, block{e.Start, e.End, inner})
	}
	visit(root)
	var b strings.Builder
	var inners []string
	last := 0
	for _, bl := range blocks {
		b.WriteString(source[last:bl.start])
		b.WriteString(strings.TrimSuffix(bl.inner, "\n"))
		last = bl.end
		inners = append(inners, bl.inner)
	}
	b.WriteString(source[last:])
	return b.String(), inners
}

func UnwrapDocument(doc *Document, name string) *Document {
	var inners []string
	unwrapItems(doc.Children, name, &inners)
	head := &Document{File: doc.File}
	if text := strings.Join(inners, "\n"); text != "" {
		head.Children = []*Item{{Kind: "text", Text: strings.TrimSuffix(text, "\n"), Origin: Origin{File: doc.File, Line: 1, Column: 1}}}
	}
	return head
}

func unwrapItems(items []*Item, name string, inners *[]string) {
	for i, it := range items {
		if it.Kind != "directive" || it.Attrs["name"] != name || it.Attrs["inline"] == "true" {
			unwrapItems(it.Children, name, inners)
			continue
		}
		_, rest, _ := strings.Cut(written(it), "\n")
		inner := rest[:strings.LastIndexByte(rest, '\n')+1]
		replaceText(items, i, unprefix(strings.TrimSuffix(inner, "\n"), itemPrefix(it)))
		*inners = append(*inners, inner)
	}
}
