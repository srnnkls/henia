// Package markup parses semantic directives with Goldmark while retaining Markdown.
package markup

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Error locates invalid markup in the input, using one-based coordinates.
type Error struct {
	Line, Column int
	ID           string
	Message      string
}

func (e *Error) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%d:%d: %s: %s", e.Line, e.Column, e.ID, e.Message)
}

type span struct{ start, stop int }
type directive struct {
	name        string
	attrs       []attribute
	open, close span
	fence       int
	labeled     bool
	closed      bool
}

var blockKind = ast.NewNodeKind("DirectiveBlock")
var inlineKind = ast.NewNodeKind("DirectiveInline")

type blockNode struct {
	ast.BaseBlock
	directive
	label span
}

func (n *blockNode) leaf() bool { return n.fence == 2 }

func (*blockNode) Kind() ast.NodeKind { return blockKind }
func (n *blockNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.name}, nil)
}

type inlineNode struct {
	ast.BaseInline
	directive
	content span
}

func (*inlineNode) Kind() ast.NodeKind { return inlineKind }
func (n *inlineNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.name}, nil)
}

var failure = parser.NewContextKey()

type errorList []*Error

func (l errorList) Error() string {
	lines := make([]string, len(l))
	for i, e := range l {
		lines[i] = e.Error()
	}
	return strings.Join(lines, "\n")
}

func (l errorList) Unwrap() []error {
	errs := make([]error, len(l))
	for i, e := range l {
		errs[i] = e
	}
	return errs
}

func (l errorList) add(e *Error) errorList {
	at, _ := slices.BinarySearchFunc(l, e, func(a, b *Error) int {
		if c := cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column)); c != 0 {
			return c
		}
		return -1
	})
	return slices.Insert(l, at, e)
}

func fail(pc parser.Context, source []byte, offset int, id, message string) {
	list, _ := pc.Get(failure).(errorList)
	pc.Set(failure, list.add(locate(source, offset, id, message)))
}

func failAttributes(pc parser.Context, source []byte, offset int, problems []problem) {
	for _, p := range problems {
		fail(pc, source, offset+p.offset, p.id, p.message)
	}
}

func locate(source []byte, offset int, id, message string) *Error {
	offset = min(offset, len(source))
	return &Error{bytes.Count(source[:offset], []byte{'\n'}) + 1, offset - bytes.LastIndexByte(source[:offset], '\n'), id, message}
}

func nameLength(s []byte) int {
	for i, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.')) {
			return i
		}
	}
	return len(s)
}

type blockParser struct{}

func (*blockParser) Trigger() []byte             { return []byte{':'} }
func (*blockParser) CanInterruptParagraph() bool { return true }
func (*blockParser) CanAcceptIndentedLine() bool { return false }

func (p *blockParser) Open(parent ast.Node, r text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := r.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos >= len(line) {
		return nil, parser.NoChildren
	}
	i := pos
	for i < len(line) && line[i] == ':' {
		i++
	}
	fence := i - pos
	if fence == 2 {
		return leaf(line, segment, pos, r, pc)
	}
	if fence < 3 {
		return nil, parser.NoChildren
	}
	i = skipBlanks(line, i)
	n := nameLength(line[i:])
	if n == 0 {
		return nil, parser.NoChildren
	}
	d := directive{name: string(line[i : i+n]), fence: fence, open: span{segment.Start + pos, segment.Start + len(bytes.TrimRight(line, "\r\n"))}}
	i = skipBlanks(line, i+n)
	if rest := bytes.TrimRight(line[i:], " \t\r\n"); len(rest) > 0 {
		attrs, consumed, problems, ok := tokenize(rest)
		if ok && consumed < len(rest) {
			problems = append(problems, problem{consumed, "", "unexpected text after directive attributes"})
		}
		failAttributes(pc, r.Source(), segment.Start+i, problems)
		d.attrs = attrs
	}
	r.Advance(len(bytes.TrimRight(line, "\r\n")))
	return &blockNode{directive: d}, parser.HasChildren
}

func leaf(line []byte, segment text.Segment, pos int, r text.Reader, pc parser.Context) (ast.Node, parser.State) {
	i := pos + 2
	n := nameLength(line[i:])
	if n == 0 {
		return nil, parser.NoChildren
	}
	end := len(bytes.TrimRight(line, "\r\n"))
	d := directive{name: string(line[i : i+n]), fence: 2, closed: true}
	i += n
	label := span{i, i}
	if i < end && line[i] == '[' {
		lr := text.NewReader(line[:end])
		lr.Advance(i + 1)
		if _, ok := lr.FindClosure('[', ']', text.FindClosureOptions{CodeSpan: true, Nesting: true, Advance: true}); !ok {
			return nil, parser.NoChildren
		}
		_, after := lr.Position()
		d.labeled, label, i = true, span{i + 1, after.Start - 1}, after.Start
	}
	var problems []problem
	attrStart := i
	if i < end && line[i] == '{' {
		attrs, consumed, found, ok := tokenize(line[i:end])
		if !ok {
			if !d.labeled {
				failAttributes(pc, r.Source(), segment.Start+i, found)
			}
			return nil, parser.NoChildren
		}
		d.attrs, problems, i = attrs, found, i+consumed
	}
	if len(bytes.TrimSpace(line[i:end])) > 0 {
		return nil, parser.NoChildren
	}
	failAttributes(pc, r.Source(), segment.Start+attrStart, problems)
	base := segment.Start
	d.open = span{base + pos, base + label.start}
	d.close = span{base + label.stop, base + end}
	node := &blockNode{directive: d, label: span{base + label.start, base + label.stop}}
	if label.stop > label.start {
		node.Lines().Append(text.NewSegment(node.label.start, node.label.stop))
	}
	r.Advance(end)
	return node, parser.NoChildren
}

func (p *blockParser) Continue(node ast.Node, r text.Reader, pc parser.Context) parser.State {
	n := node.(*blockNode)
	if n.leaf() {
		return parser.Close
	}
	// Only the innermost container can consume a closing fence. Literal blocks
	// own their contents, including lines that resemble directive delimiters.
	opened := pc.OpenedBlocks()
	for i := len(opened) - 1; i >= 0; i-- {
		child := opened[i].Node
		if child == node {
			break
		}
		if d, ok := child.(*blockNode); ok && !d.leaf() || child.Kind() == ast.KindFencedCodeBlock || child.Kind() == ast.KindHTMLBlock {
			return parser.Continue | parser.HasChildren
		}
	}
	line, segment := r.PeekLine()
	trimmed := bytes.TrimSpace(line)
	indent := len(line) - len(bytes.TrimLeft(line, " \t"))
	if indent < 4 && len(trimmed) >= n.fence && bytes.Count(trimmed, []byte{':'}) == len(trimmed) {
		n.close = span{segment.Start + indent, segment.Start + len(bytes.TrimRight(line, "\r\n"))}
		n.closed = true
		r.Advance(len(bytes.TrimRight(line, "\r\n")))
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (p *blockParser) Close(node ast.Node, r text.Reader, pc parser.Context) {
	n := node.(*blockNode)
	if !n.closed {
		fail(pc, r.Source(), n.open.start, "unclosed-directive", "directive "+n.name+" is not closed")
	}
}

type inlineParser struct{}

func (*inlineParser) Trigger() []byte { return []byte{':'} }
func (p *inlineParser) Parse(parent ast.Node, r text.Reader, pc parser.Context) ast.Node {
	line, segment := r.PeekLine()
	length := nameLength(line[1:])
	if length == 0 || length+1 >= len(line) || line[length+1] != '[' {
		return nil
	}
	savedLine, savedPosition := r.Position()
	r.Advance(length + 2)
	_, start := r.Position()
	_, ok := r.FindClosure('[', ']', text.FindClosureOptions{CodeSpan: true, Nesting: true, Advance: true})
	if !ok {
		fail(pc, r.Source(), segment.Start, "", "unclosed inline directive")
		r.SetPosition(savedLine, savedPosition)
		return nil
	}
	_, end := r.Position()
	d := directive{name: string(line[1 : length+1]), open: span{segment.Start, start.Start}, labeled: true, closed: true}
	rest, _ := r.PeekLine()
	spaces := skipBlanks(rest, 0)
	if spaces < len(rest) && rest[spaces] == '{' {
		attrs, consumed, problems, ok := tokenize(rest[spaces:])
		failAttributes(pc, r.Source(), end.Start+spaces, problems)
		if !ok {
			r.SetPosition(savedLine, savedPosition)
			return nil
		}
		d.attrs = attrs
		r.Advance(spaces + consumed)
	}
	_, stop := r.Position()
	d.close = span{end.Start - 1, stop.Start}
	return &inlineNode{directive: d, content: span{start.Start, end.Start - 1}}
}

type edit struct {
	span
	value string
}

// Render transforms directives only. Source spans from Goldmark's AST let all
// other Markdown survive byte-for-byte, including code, escapes and whitespace.
func Render(source, format string) (string, error) { return render(source, format, 0) }

func render(input, format string, depth int) (string, error) {
	if format != "" && format != "directives" && format != "xml" && format != "markdown" {
		return "", fmt.Errorf("unsupported markup format %q", format)
	}
	if depth > 100 {
		return "", fmt.Errorf("directive nesting exceeds 100 levels")
	}
	if !strings.Contains(input, ":") {
		return input, nil
	}
	source := []byte(input)
	doc, err := parse(source, depth > 0)
	if err != nil {
		return "", err
	}
	var edits []edit
	err = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n := node.(type) {
		case *blockNode:
			if n.leaf() {
				if !entering {
					return ast.WalkContinue, nil
				}
				e, err := relabel(source, n.directive, n.label, format, depth)
				edits = append(edits, e)
				return ast.WalkSkipChildren, err
			}
			open, close := delimiters(n.directive, format, false)
			if entering {
				edits = append(edits, edit{n.open, open})
			} else {
				edits = append(edits, edit{n.close, close})
			}
		case *inlineNode:
			if !entering {
				return ast.WalkContinue, nil
			}
			e, err := relabel(source, n.directive, n.content, format, depth)
			if err != nil {
				return ast.WalkStop, err
			}
			edits = append(edits, e)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	position := 0
	for _, e := range edits {
		if e.start < position || e.stop > len(source) {
			return "", fmt.Errorf("overlapping directive source spans")
		}
		out.Write(source[position:e.start])
		out.WriteString(e.value)
		position = e.stop
	}
	out.Write(source[position:])
	return out.String(), nil
}

func relabel(source []byte, d directive, label span, format string, depth int) (edit, error) {
	content, err := render(string(source[label.start:label.stop]), format, depth+1)
	if err != nil {
		if location, ok := errors.AsType[*Error](err); ok {
			return edit{}, locate(source, label.start+location.Column-1, location.ID, location.Message)
		}
		return edit{}, err
	}
	open, close := delimiters(d, format, true)
	return edit{span{d.open.start, d.close.stop}, open + content + close}, nil
}

func delimiters(d directive, format string, inline bool) (string, string) {
	if format == "markdown" {
		label := d.name
		for _, a := range d.attrs {
			label += " " + a.name + "=" + strconv.Quote(a.value)
		}
		label = strings.NewReplacer("*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;").Replace(label)
		if inline {
			return "**" + label + ":** ", ""
		}
		return "**" + label + "**", ""
	}
	var attrs strings.Builder
	for i, a := range d.attrs {
		if i > 0 {
			attrs.WriteByte(' ')
		}
		attrs.WriteString(a.name)
		attrs.WriteByte('=')
		if format == "xml" {
			attrs.WriteByte('"')
			attrs.WriteString(strings.NewReplacer("\n", "&#xA;", "\r", "&#xD;", "\t", "&#x9;").Replace(html.EscapeString(a.value)))
			attrs.WriteByte('"')
		} else {
			attrs.WriteByte('"')
			attrs.WriteString(strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(a.value))
			attrs.WriteByte('"')
		}
	}
	if format == "xml" {
		attrText := attrs.String()
		if attrText != "" {
			attrText = " " + attrText
		}
		return "<" + d.name + attrText + ">", "</" + d.name + ">"
	}
	attrText := ""
	if len(d.attrs) > 0 {
		attrText = "{" + attrs.String() + "}"
	}
	if inline {
		open := strings.Repeat(":", max(d.fence, 1)) + d.name
		if d.labeled {
			return open + "[", "]" + attrText
		}
		return open + attrText, ""
	}
	fence := strings.Repeat(":", d.fence)
	return fence + d.name + attrText, fence
}
