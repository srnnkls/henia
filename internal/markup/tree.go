package markup

import (
	"bytes"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
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
	Body          string
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
	pc := parser.NewContext()
	doc, err := runContext(documentParser, source, pc)
	b := newBuilder(source, pc, err)
	root := b.file()
	b.blocks(root, doc, true)
	b.finish(root)
	for _, e := range extra {
		place(root, e)
	}
	b.finish(root)
	if len(b.failures) == 0 {
		return root, nil
	}
	return root, b.failures
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
	source     []byte
	body       string
	lines      []int
	failures   errorList
	references []parser.Reference
	rules      map[ast.Node]text.Segment
	at         int
}

func newBuilder(source []byte, pc parser.Context, err error) *builder {
	b := &builder{source: source, references: pc.References()}
	b.rules, _ = pc.Get(rules).(map[ast.Node]text.Segment)
	b.failures, _ = err.(errorList)
	return b
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
	case *ast.HTMLBlock:
		e.Type = "html"
	case *ast.ThematicBreak:
		segment, ok := b.rules[v]
		if !ok {
			return nil
		}
		e.Type, e.Start, e.End = "rule", segment.Start, segment.Stop
		b.at = max(b.at, e.End)
		return e
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
		e.Type, e.Attrs["name"], e.Start, e.End = "directive", v.name, v.open.start, max(v.open.stop, v.close.stop)
		for _, a := range v.attrs {
			e.Attrs[a.name] = a.value
		}
		b.at = max(b.at, v.open.stop)
		if v.leaf() {
			b.inlines(e, v)
		} else {
			b.blocks(e, v, true)
		}
		b.at = max(b.at, e.End)
		return e
	default:
		return nil
	}
	e.Start, e.End = b.extent(node)
	if fenced, ok := node.(*ast.FencedCodeBlock); ok {
		e.Text = string(node.Lines().Value(b.source))
		e.Start, e.End = b.fence(fenced)
	}
	if h, ok := node.(*ast.Heading); ok {
		e.Text = string(h.Lines().Value(b.source))
	}
	if html, ok := node.(*ast.HTMLBlock); ok && html.HasClosure() {
		e.End = max(e.End, html.ClosureLine.Stop)
	}
	b.at = max(b.at, e.End)
	return e
}

func (b *builder) fence(code *ast.FencedCodeBlock) (int, int) {
	open := 0
	switch {
	case code.Info != nil:
		open = code.Info.Segment.Start
	case code.Lines().Len() > 0:
		open = max(bytes.LastIndexByte(b.source[:code.Lines().At(0).Start], '\n'), 0)
	default:
		open = b.at + fenceAt(b.source[b.at:])
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

func fenceAt(text []byte) int {
	at := len(text)
	for _, marker := range []string{"```", "~~~"} {
		if i := bytes.Index(text, []byte(marker)); i >= 0 {
			at = min(at, i)
		}
	}
	return at
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
			e = &Element{Type: "code", Attrs: map[string]string{"inline": "true"}}
			inner, end := b.extent(n)
			e.Start, e.End = b.ticks(inner, end)
			if inner >= 0 {
				e.Text = b.body[inner:end]
			}
			e.Attrs["ticks"] = strconv.Itoa(inner - e.Start)
			parent.add(e)
			return ast.WalkSkipChildren, nil
		case *inlineNode:
			e = &Element{Type: "directive", Attrs: map[string]string{"name": v.name, "inline": "true"}}
			for _, a := range v.attrs {
				e.Attrs[a.name] = a.value
			}
			e.Start, e.End = v.open.start, v.close.stop
			parent.add(e)
			b.inlines(e, b.label(v.content))
			return ast.WalkSkipChildren, nil
		default:
			return ast.WalkContinue, nil
		}
		e.Start, e.End = b.extent(n)
		parent.add(e)
		b.inlines(e, n)
		return ast.WalkSkipChildren, nil
	})
}

func (b *builder) ticks(start, end int) (int, int) {
	if start < 0 {
		return start, end
	}
	for start > 0 && b.source[start-1] == '`' {
		start--
	}
	for end < len(b.source) && b.source[end] == '`' {
		end++
	}
	return start, end
}

func (b *builder) label(s span) ast.Node {
	stop := s.stop
	if end := bytes.IndexByte(b.source[s.start:s.stop], '\n'); end >= 0 {
		stop = s.start + end + 1
	}
	r := text.NewReader(b.source[:s.stop])
	r.SetPosition(0, text.NewSegment(s.start, stop))
	pc := parser.NewContext()
	for _, ref := range b.references {
		pc.AddReference(ref)
	}
	doc := inlineDirectiveParser.Parse(r, parser.WithContext(pc))
	list, _ := pc.Get(failure).(errorList)
	for _, e := range list {
		b.failures = b.failures.add(e)
	}
	return doc
}

func (b *builder) extent(node ast.Node) (int, int) {
	start, end := -1, -1
	cover := func(s, e int) {
		if s < 0 {
			return
		}
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
		case *ast.CodeSpan:
			if n != node {
				cover(b.ticks(b.extent(v)))
				return ast.WalkSkipChildren, nil
			}
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
	return []parser.Option{
		parser.WithBlockParsers(util.Prioritized(&blockParser{}, 850), util.Prioritized(ruleParser{parser.NewThematicBreakParser()}, 199)),
		parser.WithInlineParsers(util.Prioritized(&inlineParser{}, 150)),
	}
}

var rules = parser.NewContextKey()

type ruleParser struct{ parser.BlockParser }

func (p ruleParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	_, segment := reader.PeekLine()
	node, state := p.BlockParser.Open(parent, reader, pc)
	if node != nil {
		found, _ := pc.Get(rules).(map[ast.Node]text.Segment)
		if found == nil {
			found = map[ast.Node]text.Segment{}
			pc.Set(rules, found)
		}
		segment = segment.TrimLeftSpace(reader.Source())
		found[node] = segment.TrimRightSpace(reader.Source())
	}
	return node, state
}
