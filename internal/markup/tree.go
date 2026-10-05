package markup

import (
	"bytes"
	"slices"
	"sort"
	"strconv"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Element is a node of the document tree that queries match. A section owns its
// heading and the blocks up to the next heading of the same or a higher level.
type Element struct {
	Type          string
	Attrs         map[string]string
	Text          string
	Start, End    int
	Line, EndLine int
	Column        int
	Parent        *Element
	Children      []*Element
	Index         int
	Order         int
}

// Tree places extra elements under the innermost block that spans them. A
// directive error still yields the tree.
func Tree(source []byte, extra ...*Element) (*Element, error) {
	p := &directiveParser{}
	md := goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithParserOptions(append(directiveOptions(p), parser.WithAutoHeadingID())...))
	doc := md.Parser().Parse(text.NewReader(source))
	b := &builder{source: source}
	root := b.file()
	b.blocks(root, doc, true)
	b.finish(root)
	for _, e := range extra {
		place(root, e)
	}
	b.finish(root)
	return root, p.err
}

func PlainTree(source []byte) *Element {
	b := &builder{source: source}
	root := b.file()
	start := -1
	for offset := 0; offset <= len(source); {
		end := bytes.IndexByte(source[offset:], '\n')
		if end < 0 {
			end = len(source)
		} else {
			end += offset
		}
		blank := len(bytes.TrimSpace(source[offset:end])) == 0
		if !blank && start < 0 {
			start = offset
		}
		if blank && start >= 0 {
			root.add(&Element{Type: "paragraph", Attrs: map[string]string{}, Start: start, End: offset})
			start = -1
		}
		offset = end + 1
	}
	if start >= 0 {
		root.add(&Element{Type: "paragraph", Attrs: map[string]string{}, Start: start, End: len(source)})
	}
	b.finish(root)
	return root
}

func (e *Element) Walk(visit func(*Element) bool) bool {
	if !visit(e) {
		return false
	}
	for _, c := range e.Children {
		if !c.Walk(visit) {
			return false
		}
	}
	return true
}

func (e *Element) Enclosing(kind string) *Element {
	for ; e != nil; e = e.Parent {
		if e.Type == kind {
			return e
		}
	}
	return nil
}

func (e *Element) add(child *Element) {
	child.Parent = e
	e.Children = append(e.Children, child)
}

type builder struct {
	source []byte
	lines  []int
}

func (b *builder) file() *Element {
	for i, c := range b.source {
		if c == '\n' {
			b.lines = append(b.lines, i)
		}
	}
	return &Element{Type: "file", Attrs: map[string]string{}, End: len(b.source)}
}

func (b *builder) blocks(parent *Element, node ast.Node, sectioned bool) {
	stack := []*Element{parent}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if h, ok := child.(*ast.Heading); ok && sectioned {
			for len(stack) > 1 && level(stack[len(stack)-1]) >= h.Level {
				stack = stack[:len(stack)-1]
			}
			id, _ := h.AttributeString("id")
			anchor, _ := id.([]byte)
			section := &Element{Type: "section", Attrs: map[string]string{"id": string(anchor), "title": string(h.Lines().Value(b.source)), "level": strconv.Itoa(h.Level)}, Start: -1}
			stack[len(stack)-1].add(section)
			stack = append(stack, section)
		}
		if e := b.block(child); e != nil {
			stack[len(stack)-1].add(e)
		}
	}
}

func level(e *Element) int {
	n, _ := strconv.Atoi(e.Attrs["level"])
	return n
}

func (b *builder) block(node ast.Node) *Element {
	e := &Element{Attrs: map[string]string{}, Start: -1}
	switch v := node.(type) {
	case *ast.Heading:
		e.Type, e.Attrs["level"] = "heading", strconv.Itoa(v.Level)
		b.inlines(e, v)
	case *ast.Paragraph, *ast.TextBlock:
		e.Type = "paragraph"
		b.inlines(e, v)
	case *ast.FencedCodeBlock:
		e.Type, e.Attrs["lang"] = "code", string(v.Language(b.source))
	case *ast.CodeBlock:
		e.Type = "code"
	case *ast.List:
		e.Type, e.Attrs["ordered"] = "list", strconv.FormatBool(v.IsOrdered())
		b.blocks(e, v, false)
	case *ast.ListItem:
		e.Type = "item"
		b.blocks(e, v, false)
	case *ast.Blockquote:
		e.Type = "quote"
		b.blocks(e, v, false)
	case *east.Table:
		e.Type = "table"
		b.inlines(e, v)
	case *blockNode:
		e.Type, e.Attrs["name"], e.Start, e.End = "directive", v.name, v.open.start, v.close.stop
		for _, a := range v.attrs {
			e.Attrs[a.name] = a.value
		}
		b.blocks(e, v, false)
		return e
	default:
		return nil
	}
	e.Start, e.End = extent(node)
	if fenced, ok := node.(*ast.FencedCodeBlock); ok {
		e.Text = string(node.Lines().Value(b.source))
		e.Start, e.End = b.fence(fenced)
	}
	if h, ok := node.(*ast.Heading); ok {
		e.Text = string(h.Lines().Value(b.source))
	}
	return e
}

func (b *builder) fence(code *ast.FencedCodeBlock) (int, int) {
	open := 0
	switch {
	case code.Info != nil:
		open = code.Info.Segment.Start
	case code.Lines().Len() > 0:
		open = max(code.Lines().At(0).Start-1, 0)
	}
	open = bytes.LastIndexByte(b.source[:open], '\n') + 1
	end := open
	if lines := code.Lines(); lines.Len() > 0 {
		end = lines.At(lines.Len() - 1).Stop
	} else if next := bytes.IndexByte(b.source[open:], '\n'); next >= 0 {
		end = open + next + 1
	}
	if close := bytes.IndexByte(b.source[min(end, len(b.source)):], '\n'); close >= 0 {
		return open, end + close
	}
	return open, len(b.source)
}

func (b *builder) inlines(parent *Element, node ast.Node) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n == node {
			return ast.WalkContinue, nil
		}
		e := &Element{Attrs: map[string]string{}}
		switch v := n.(type) {
		case *ast.Link:
			e.Type, e.Attrs["url"] = "link", string(v.Destination)
		case *ast.AutoLink:
			e.Type, e.Attrs["url"] = "link", string(v.URL(b.source))
		case *inlineNode:
			e.Type, e.Attrs["name"], e.Attrs["inline"] = "directive", v.name, "true"
			for _, a := range v.attrs {
				e.Attrs[a.name] = a.value
			}
			e.Start, e.End = v.open.start, v.close.stop
			parent.add(e)
			return ast.WalkSkipChildren, nil
		default:
			return ast.WalkContinue, nil
		}
		e.Start, e.End = extent(n)
		parent.add(e)
		return ast.WalkSkipChildren, nil
	})
}

func extent(node ast.Node) (int, int) {
	start, end := -1, -1
	cover := func(s, e int) {
		if start < 0 || s < start {
			start = s
		}
		end = max(end, e)
	}
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Type() == ast.TypeBlock {
			if lines := n.Lines(); lines.Len() > 0 {
				cover(lines.At(0).Start, lines.At(lines.Len()-1).Stop)
			}
		}
		switch v := n.(type) {
		case *ast.Text:
			cover(v.Segment.Start, v.Segment.Stop)
		case *inlineNode:
			cover(v.open.start, v.close.stop)
		case *blockNode:
			cover(v.open.start, v.close.stop)
		}
		return ast.WalkContinue, nil
	})
	return start, end
}

func (b *builder) finish(e *Element) {
	for i, c := range e.Children {
		c.Parent, c.Index = e, i
		b.finish(c)
		if c.Start >= 0 && (e.Type == "section" || e.Start < 0) && (e.Start < 0 || c.Start < e.Start) {
			e.Start = c.Start
		}
		if e.Type != "file" {
			e.End = max(e.End, c.End)
		}
	}
	if e.Start < 0 {
		e.Start, e.End = 0, 0
	}
	if e.Column == 0 {
		e.Column = e.Start - bytes.LastIndexByte(b.source[:e.Start], '\n')
	}
	if e.Type == "section" || e.Type == "heading" {
		e.Start = bytes.LastIndexByte(b.source[:e.Start], '\n') + 1
	}
	if e.Text == "" && e.Type != "file" {
		e.Text = string(bytes.TrimRight(b.source[e.Start:e.End], "\n"))
	}
	e.Line = sort.SearchInts(b.lines, e.Start) + 1
	e.EndLine = sort.SearchInts(b.lines, max(e.Start, e.End-1)) + 1
}

func place(e, extra *Element) {
	for _, c := range e.Children {
		if c.Start <= extra.Start && extra.End <= c.End && c.Type != "code" {
			place(c, extra)
			return
		}
	}
	at, _ := slices.BinarySearchFunc(e.Children, extra.Start, func(c *Element, start int) int { return c.Start - start })
	extra.Parent = e
	e.Children = slices.Insert(e.Children, at, extra)
}

func directiveOptions(p *directiveParser) []parser.Option {
	return []parser.Option{parser.WithBlockParsers(util.Prioritized(&blockParser{p}, 850)), parser.WithInlineParsers(util.Prioritized(&inlineParser{p}, 150))}
}
