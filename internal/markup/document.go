package markup

import (
	"bytes"
	"maps"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

// Origin locates a node in the file it was parsed from. End excludes the
// trailing line break; Line and Column are one-based.
type Origin struct {
	File         string
	Start, End   int
	Line, Column int
}

// Call is one expansion step: the directive that invoked a component and the
// component line that produced the node.
type Call struct {
	Directive, Component Origin
}

// Item is a mutable node of a Document. Kind uses the Element type names. Text
// is the literal content of an item without block children: a paragraph's
// Markdown, a heading's title, a code block's contents, a directive's label.
type Item struct {
	Kind      string
	Attrs     map[string]string
	Text      string
	Children  []*Item
	Origin    Origin
	Backtrace []Call

	parsed *parsed
}

// Document is a parsed file whose items keep their origins.
type Document struct {
	File     string
	Children []*Item

	parsed *parsed
}

type source struct {
	file string
	text []byte
}

// parsed is the state of an item at parse time. Children sit in the region
// [lo, hi) of the source; gap is the source between an item and its preceding
// sibling pred, trail the source after the last child, sep the separator
// written before a child whose predecessor changed, and lead the content-column
// prefix written before a child that moved into the first position.
type parsed struct {
	src        *source
	start, end int
	lo, hi     int
	trail      int
	gap        []byte
	sep        string
	lead       string
	pred       *Item
	attrs      map[string]string
	text       string
	children   []*Item
	inline     bool
	list       bool
	directive  *directive
}

// Parse builds the document tree of a file. A directive error still yields the
// document.
func Parse(file string, text []byte) (*Document, error) {
	text = bytes.Clone(text)
	pc := parser.NewContext()
	node, err := runContext(documentParser, text, pc)
	b := newBuilder(text, pc, err)
	root := b.file()
	b.blocks(root, node, false)
	b.finish(root)

	directives := map[int]*directive{}
	labels := &builder{source: text, references: b.references}
	labels.collect(node, directives)

	src := &source{file: file, text: text}
	doc := &Document{File: file, parsed: &parsed{src: src, hi: len(text)}}
	for _, e := range root.Children {
		doc.Children = append(doc.Children, convert(src, e, directives, 0))
	}
	doc.parsed.adopt(doc.Children)
	if len(b.failures) == 0 {
		return doc, nil
	}
	return doc, b.failures
}

func (b *builder) collect(node ast.Node, into map[int]*directive) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch d := n.(type) {
		case *blockNode:
			into[d.open.start] = &d.directive
		case *inlineNode:
			into[d.open.start] = &d.directive
			b.collect(b.label(d.content), into)
		}
		return ast.WalkContinue, nil
	})
}

func convert(src *source, e *Element, directives map[int]*directive, indent int) *Item {
	it := &Item{Kind: e.Type, Attrs: e.Attrs}
	inner := content(src.text, e, indent)
	for _, c := range e.Children {
		it.Children = append(it.Children, convert(src, c, directives, inner))
	}
	start, end := e.Start, max(e.Start, e.End)
	d := directives[e.Start]
	if e.Type == "directive" && d != nil {
		end = max(end, d.open.stop)
	}
	if n := len(it.Children); n > 0 {
		end = max(end, it.Children[n-1].parsed.end)
	}
	lo := start
	switch e.Type {
	case "list", "item", "quote":
		if len(it.Children) > 0 {
			start = min(start, it.Children[0].parsed.start)
		}
		lo = start
		start = owner(src.text, start, e.Type)
	case "heading", "section":
		start = afterIndent(src.text, start, indent)
		switch {
		case e.Type != "heading":
		case start < len(src.text) && src.text[start] == '#':
			end = max(end, start+lineLength(src.text[start:]))
		default:
			end = underline(src.text, end)
		}
	case "code":
		if _, fenced := e.Attrs["lang"]; fenced {
			start = afterIndent(src.text, start, indent)
		} else if e.Attrs["inline"] != "true" {
			start = indented(src.text, start)
		}
	}
	it.Origin = locateOrigin(src, start, max(start, end))
	p := &parsed{src: src, start: it.Origin.Start, end: it.Origin.End, attrs: maps.Clone(e.Attrs)}
	p.lo, p.hi = max(p.start, lo), p.end
	switch e.Type {
	case "file", "section", "list", "item", "quote":
		p.list = e.Type == "list"
	case "directive":
		if d == nil {
			p.inline = true
			break
		}
		p.directive = d
		if d.fence >= 3 {
			p.lo = d.open.stop
			if next := bytes.IndexByte(src.text[p.lo:], '\n'); next >= 0 {
				p.lo += next + 1
			}
			if d.closed {
				p.hi = d.close.start
			}
			p.lo, p.hi = min(p.lo, p.end), max(p.hi, min(p.lo, p.end))
			break
		}
		p.inline, p.lo, p.hi = true, d.open.stop, d.close.start
		it.Text = string(src.text[p.lo:p.hi])
	default:
		p.inline = true
		it.Text = e.Text
	}
	p.adopt(it.Children)
	p.text = it.Text
	it.parsed = p
	return it
}

func owner(text []byte, start int, kind string) int {
	i := start
	for i > 0 && (text[i-1] == ' ' || text[i-1] == '\t' || kind == "item" && (text[i-1] == '\n' || text[i-1] == '\r')) {
		i--
	}
	if i == 0 {
		return start
	}
	switch c := text[i-1]; {
	case kind == "quote" && c == '>', kind == "item" && (c == '-' || c == '+' || c == '*'):
		return i - 1
	case kind == "item" && (c == '.' || c == ')'):
		j := i - 1
		for j > 0 && text[j-1] >= '0' && text[j-1] <= '9' {
			j--
		}
		if j < i-1 {
			return j
		}
	}
	return start
}

func afterIndent(text []byte, start, indent int) int {
	line := bytes.LastIndexByte(text[:start], '\n') + 1
	i := line
	for i < len(text) && i-line < indent && text[i] != '\n' {
		i++
	}
	return max(start, i)
}

func content(text []byte, e *Element, indent int) int {
	if e.Type != "quote" && e.Type != "item" {
		return indent
	}
	line := bytes.LastIndexByte(text[:e.Start], '\n') + 1
	i := afterIndent(text, line, indent)
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	if e.Type == "quote" {
		if i < len(text) && text[i] == '>' {
			i++
			if i < len(text) && text[i] == ' ' {
				i++
			}
		}
		return i - line
	}
	n := markerLength(text, i)
	if n == 0 {
		return indent
	}
	i += n
	spaces := 0
	for i+spaces < len(text) && text[i+spaces] == ' ' {
		spaces++
	}
	if spaces == 0 || spaces > 4 || i+spaces == len(text) || text[i+spaces] == '\n' {
		spaces = 1
	}
	return i + spaces - line
}

func markerLength(text []byte, at int) int {
	end := at
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	switch {
	case end == len(text):
		return 0
	case end > at && (text[end] == '.' || text[end] == ')'), end == at && (text[end] == '-' || text[end] == '+' || text[end] == '*'):
		if markerEnd(text, end+1) {
			return end + 1 - at
		}
	}
	return 0
}

func markerEnd(text []byte, at int) bool {
	return at == len(text) || text[at] == ' ' || text[at] == '\t' || text[at] == '\n'
}

func lineLength(text []byte) int {
	if n := bytes.IndexByte(text, '\n'); n >= 0 {
		return n
	}
	return len(text)
}

func underline(text []byte, end int) int {
	end = len(bytes.TrimRight(text[:min(end, len(text))], "\r\n"))
	line := bytes.IndexByte(text[min(end, len(text)):], '\n')
	if line < 0 {
		return end
	}
	next := end + line + 1
	if stop := bytes.IndexByte(text[next:], '\n'); stop >= 0 {
		return next + stop
	}
	return len(text)
}

func indented(text []byte, start int) int {
	for width := 0; width < 4 && start > 0; start-- {
		switch text[start-1] {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return start
		}
	}
	return start
}

func (p *parsed) adopt(children []*Item) {
	p.children = slices.Clone(children)
	at := p.lo
	var pred *Item
	for _, c := range children {
		start := max(at, c.parsed.start)
		c.parsed.gap, c.parsed.pred = p.src.text[at:start], pred
		if p.sep == "" && pred != nil && separating(c.parsed.gap) {
			p.sep = string(c.parsed.gap)
		}
		at, pred = max(at, c.parsed.end), c
	}
	p.trail = min(at, p.hi)
	if len(children) > 0 && len(bytes.Trim(children[0].parsed.gap, " \t\r\n>")) == 0 {
		p.lead = string(children[0].parsed.gap)
	}
	if p.sep != "" || p.inline {
		return
	}
	prefix := ""
	if len(children) > 0 {
		prefix = linePrefix(p.src.text, children[0].parsed.start)
	}
	p.sep = "\n" + prefix
	if !p.list {
		p.sep = "\n" + strings.TrimRight(prefix, " \t") + p.sep
	}
}

func separating(gap []byte) bool {
	return bytes.IndexByte(gap, '\n') >= 0 && len(bytes.Trim(gap, " \t\r\n>")) == 0
}

func linePrefix(text []byte, start int) string {
	line := []byte(string(text[bytes.LastIndexByte(text[:start], '\n')+1 : start]))
	for i, c := range line {
		if c != ' ' && c != '\t' && c != '>' {
			line[i] = ' '
		}
	}
	return string(line)
}

func locateOrigin(src *source, start, end int) Origin {
	end = max(start, start+len(bytes.TrimRight(src.text[start:end], "\r\n")))
	return Origin{
		File:   src.file,
		Start:  start,
		End:    end,
		Line:   bytes.Count(src.text[:start], []byte{'\n'}) + 1,
		Column: start - bytes.LastIndexByte(src.text[:start], '\n'),
	}
}
