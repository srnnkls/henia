// Package markup parses semantic directives with Goldmark while retaining Markdown.
package markup

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Error locates invalid markup in the input, using one-based coordinates.
type Error struct {
	Line, Column int
	Message      string
}

func (e *Error) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Message) }

type span struct{ start, stop int }
type attribute struct{ name, value string }
type directive struct {
	name        string
	attrs       []attribute
	open, close span
	fence       int
	closed      bool
}

var blockKind = ast.NewNodeKind("DirectiveBlock")
var inlineKind = ast.NewNodeKind("DirectiveInline")

type blockNode struct {
	ast.BaseBlock
	directive
}

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

type directiveParser struct{ err error }

func (p *directiveParser) fail(source []byte, offset int, message string) {
	if p.err != nil {
		return
	}
	offset = min(offset, len(source))
	p.err = &Error{bytes.Count(source[:offset], []byte{'\n'}) + 1, offset - bytes.LastIndexByte(source[:offset], '\n'), message}
}

func nameLength(s []byte) int {
	for i, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && (c >= '0' && c <= '9' || c == '-' || c == '.')) {
			return i
		}
	}
	return len(s)
}

// Goldmark owns attribute tokenization and escaping. Restrict its richer value
// model to scalar values so every accepted directive has an XML representation.
func attributes(input []byte) ([]attribute, int, error) {
	r := text.NewReader(input)
	attrs, ok := parser.ParseAttributes(r)
	if !ok {
		return nil, 0, fmt.Errorf("invalid directive attributes")
	}
	_, position := r.Position()
	result := make([]attribute, 0, len(attrs))
	seen := make(map[string]bool)
	for _, a := range attrs {
		name := string(a.Name)
		if nameLength(a.Name) != len(a.Name) || seen[name] {
			return nil, 0, fmt.Errorf("invalid or duplicate attribute %q", name)
		}
		seen[name] = true
		var value string
		switch v := a.Value.(type) {
		case []byte:
			value = string(v)
		case float64:
			value = strconv.FormatFloat(v, 'g', -1, 64)
		case bool:
			value = strconv.FormatBool(v)
		case nil:
			value = "null"
		default:
			return nil, 0, fmt.Errorf("attribute %q must be scalar", name)
		}
		if !utf8.ValidString(value) {
			return nil, 0, fmt.Errorf("attribute %q is not valid UTF-8", name)
		}
		for _, r := range value {
			if r < 0x20 && r != '\n' && r != '\r' && r != '\t' || r == 0xfffe || r == 0xffff {
				return nil, 0, fmt.Errorf("attribute %q contains an invalid XML character", name)
			}
		}
		result = append(result, attribute{name, value})
	}
	return result, position.Start, nil
}

type blockParser struct{ *directiveParser }

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
	if fence < 3 {
		return nil, parser.NoChildren
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	n := nameLength(line[i:])
	if n == 0 {
		return nil, parser.NoChildren
	}
	d := directive{name: string(line[i : i+n]), fence: fence, open: span{segment.Start + pos, segment.Start + len(bytes.TrimRight(line, "\r\n"))}}
	i += n
	rest := bytes.TrimSpace(line[i:])
	if len(rest) > 0 {
		attrs, consumed, err := attributes(rest)
		if err == nil && len(bytes.TrimSpace(rest[consumed:])) > 0 {
			err = fmt.Errorf("unexpected text after directive attributes")
		}
		if err != nil {
			p.fail(r.Source(), segment.Start+i, err.Error())
		}
		d.attrs = attrs
	}
	r.Advance(len(bytes.TrimRight(line, "\r\n")))
	return &blockNode{directive: d}, parser.HasChildren
}

func (p *blockParser) Continue(node ast.Node, r text.Reader, pc parser.Context) parser.State {
	n := node.(*blockNode)
	// Only the innermost container can consume a closing fence. Literal blocks
	// own their contents, including lines that resemble directive delimiters.
	opened := pc.OpenedBlocks()
	for i := len(opened) - 1; i >= 0; i-- {
		child := opened[i].Node
		if child == node {
			break
		}
		if child.Kind() == blockKind || child.Kind() == ast.KindFencedCodeBlock || child.Kind() == ast.KindHTMLBlock {
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
		p.fail(r.Source(), n.open.start, "unclosed directive "+n.name)
	}
}

type inlineParser struct{ *directiveParser }

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
		p.fail(r.Source(), segment.Start, "unclosed inline directive")
		r.SetPosition(savedLine, savedPosition)
		return nil
	}
	_, end := r.Position()
	d := directive{name: string(line[1 : length+1]), open: span{segment.Start, start.Start}, closed: true}
	rest, _ := r.PeekLine()
	spaces := len(rest) - len(bytes.TrimLeft(rest, " \t"))
	if spaces < len(rest) && rest[spaces] == '{' {
		attrs, consumed, err := attributes(rest[spaces:])
		if err != nil {
			p.fail(r.Source(), end.Start+spaces, err.Error())
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
	if format != "" && format != "directives" && format != "xml" {
		return "", fmt.Errorf("unsupported markup format %q", format)
	}
	if depth > 100 {
		return "", fmt.Errorf("directive nesting exceeds 100 levels")
	}
	if !strings.Contains(input, ":") {
		return input, nil
	}
	source := []byte(input)
	p := &directiveParser{}
	md := goldmark.New(goldmark.WithParserOptions(parser.WithBlockParsers(util.Prioritized(&blockParser{p}, 850)), parser.WithInlineParsers(util.Prioritized(&inlineParser{p}, 150))))
	if depth > 0 {
		md = goldmark.New(goldmark.WithParser(parser.NewParser(
			parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
			parser.WithInlineParsers(parser.DefaultInlineParsers()...),
			parser.WithInlineParsers(util.Prioritized(&inlineParser{p}, 150)),
		)))
	}
	doc := md.Parser().Parse(text.NewReader(source))
	if p.err != nil {
		return "", p.err
	}
	var edits []edit
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n := node.(type) {
		case *blockNode:
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
			content, err := render(string(source[n.content.start:n.content.stop]), format, depth+1)
			if err != nil {
				if location, ok := errors.AsType[*Error](err); ok {
					p.fail(source, n.content.start+location.Column-1, location.Message)
					return ast.WalkStop, p.err
				}
				return ast.WalkStop, err
			}
			open, close := delimiters(n.directive, format, true)
			edits = append(edits, edit{span{n.open.start, n.close.stop}, open + content + close})
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

func delimiters(d directive, format string, inline bool) (string, string) {
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
		return ":" + d.name + "[", "]" + attrText
	}
	fence := strings.Repeat(":", d.fence)
	return fence + d.name + attrText, fence
}
