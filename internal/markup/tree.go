package markup

import (
	"bytes"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

// Element is a node of the document tree that queries match. A section owns its
// heading and the blocks up to the next heading of the same or a higher level.
type Element struct {
	Type          string
	Attrs         map[string]string
	Text          string
	Body          string
	Start, End    int
	Line, EndLine int
	Column        int
	Parent        *Element
	Children      []*Element
	Index         int
	Order         int
	directive     *directive
}

// Tree places extra elements under the innermost block that spans them. A
// directive error still yields the tree.
func Tree(source []byte, extra ...*Element) (*Element, error) {
	doc, err := run(documentParser, source)
	b := &builder{source: source}
	root := b.file()
	b.blocks(root, doc, true)
	b.finish(root)
	Graft(root, extra...)
	if err == nil {
		err = b.err
	}
	return root, err
}

func Graft(root *Element, extra ...*Element) {
	if len(extra) == 0 {
		return
	}
	for _, e := range extra {
		place(root, e)
	}
	b := &builder{source: []byte(root.Body)}
	b.file()
	b.finish(root)
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
	body   string
	lines  []int
	err    error
}

func (b *builder) file() *Element {
	b.body = string(b.source)
	for i, c := range b.source {
		if c == '\n' {
			b.lines = append(b.lines, i)
		}
	}
	return &Element{Type: "file", Attrs: map[string]string{}, Body: b.body, End: len(b.source)}
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
		e.Type, e.Attrs["lang"], e.Attrs["inline"] = "code", string(v.Language(b.source)), "false"
		if v.Info != nil {
			e.Attrs["info"] = strings.TrimSpace(string(v.Info.Segment.Value(b.source)))
		}
	case *ast.CodeBlock:
		e.Type, e.Attrs["inline"] = "code", "false"
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
		e.Type, e.Attrs["name"], e.Start, e.End, e.directive = "directive", v.name, v.open.start, v.close.stop, &v.directive
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
		var e *Element
		switch v := n.(type) {
		case *ast.Link:
			e = &Element{Type: "link", Attrs: map[string]string{"url": string(v.Destination)}}
		case *ast.Image:
			e = &Element{Type: "image", Attrs: map[string]string{"url": string(v.Destination)}}
		case *ast.AutoLink:
			e = &Element{Type: "link", Attrs: map[string]string{"url": string(v.URL(b.source))}}
		case *ast.CodeSpan:
			if code := b.code(v); code != nil {
				parent.add(code)
			}
			return ast.WalkSkipChildren, nil
		case *inlineNode:
			d := v.directive
			e = &Element{Type: "directive", Attrs: map[string]string{"name": v.name, "inline": "true"}, directive: &d}
			for _, a := range v.attrs {
				e.Attrs[a.name] = a.value
			}
			e.Start, e.End = v.open.start, v.close.stop
			parent.add(e)
			b.content(e, v.content)
			return ast.WalkSkipChildren, nil
		default:
			return ast.WalkContinue, nil
		}
		e.Start, e.End = extent(n)
		parent.add(e)
		b.inlines(e, n)
		return ast.WalkSkipChildren, nil
	})
}

func (b *builder) code(span *ast.CodeSpan) *Element {
	first, firstOK := span.FirstChild().(*ast.Text)
	last, lastOK := span.LastChild().(*ast.Text)
	if !firstOK || !lastOK {
		return nil
	}
	open, inner := first.Segment.Start, first.Segment.Start
	if open > 0 && b.source[open-1] != '`' {
		open--
		inner--
	}
	for open > 0 && b.source[open-1] == '`' {
		open--
	}
	ticks := inner - open
	end := last.Segment.Stop
	if end < len(b.source) && b.source[end] != '`' {
		end++
	}
	close := end + ticks
	if ticks == 0 || close > len(b.source) || strings.Count(string(b.source[end:close]), "`") != ticks {
		return nil
	}
	return &Element{Type: "code", Attrs: map[string]string{"inline": "true", "ticks": strconv.Itoa(ticks)}, Text: string(b.source[inner:end]), Start: open, End: close}
}

func (b *builder) content(directive *Element, content span) {
	if !bytes.ContainsAny(b.source[content.start:content.stop], "`[<:") {
		return
	}
	nested := &builder{source: b.source[content.start:content.stop]}
	doc, err := run(inlineDirectiveParser, nested.source)
	for block := doc.FirstChild(); block != nil; block = block.NextSibling() {
		nested.inlines(directive, block)
	}
	if err == nil {
		err = nested.err
	}
	if location, ok := errors.AsType[*Error](err); ok && b.err == nil {
		b.err = locate(b.source, content.start+location.Column-1, location.Message)
	}
	for _, child := range directive.Children {
		child.Walk(func(e *Element) bool {
			e.Start += content.start
			e.End += content.start
			if e.directive != nil {
				e.directive.open.start += content.start
				e.directive.open.stop += content.start
				e.directive.close.start += content.start
				e.directive.close.stop += content.start
			}
			return true
		})
	}
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
		e.Text = strings.TrimRight(b.body[e.Start:e.End], "\n")
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

func directiveOptions() []parser.Option {
	return []parser.Option{parser.WithBlockParsers(util.Prioritized(&blockParser{}, 850)), parser.WithInlineParsers(util.Prioritized(&inlineParser{}, 150))}
}
