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
