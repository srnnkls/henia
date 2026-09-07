package markup

import (
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Node is a stable, read-only view for lint rules. Offsets address input bytes.
type Node struct {
	Kind        string            `expr:"kind"`
	Name        string            `expr:"name"`
	Value       any               `expr:"value"`
	Attrs       map[string]string `expr:"attrs"`
	Text        string            `expr:"text"`
	Destination string            `expr:"destination"`
	Level       int               `expr:"level"`
	Inline      bool              `expr:"inline"`
	Dynamic     bool              `expr:"dynamic"`
	Start       int               `expr:"start"`
	End         int               `expr:"end"`
}

func parse(source []byte, inline bool) (ast.Node, error) {
	p := &directiveParser{}
	md := goldmark.New(goldmark.WithParserOptions(parser.WithBlockParsers(util.Prioritized(&blockParser{p}, 850)), parser.WithInlineParsers(util.Prioritized(&inlineParser{p}, 150))))
	if inline {
		md = goldmark.New(goldmark.WithParser(parser.NewParser(
			parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
			parser.WithInlineParsers(parser.DefaultInlineParsers()...),
			parser.WithInlineParsers(util.Prioritized(&inlineParser{p}, 150)),
		)))
	}
	root := md.Parser().Parse(text.NewReader(source))
	return root, p.err
}

// Inspect uses the same Goldmark extension as rendering, including nested inline
// directives. Literal code and HTML blocks do not produce lint candidates.
func Inspect(source []byte) ([]Node, error) { return inspect(source, 0, 0) }

func inspect(source []byte, base, depth int) ([]Node, error) {
	if depth > 100 {
		return nil, fmt.Errorf("directive nesting exceeds 100 levels")
	}
	root, err := parse(source, depth > 0)
	if err != nil {
		return nil, err
	}
	var nodes []Node
	err = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		n := Node{Attrs: map[string]string{}}
		switch v := node.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock, *ast.CodeSpan:
			return ast.WalkSkipChildren, nil
		case *blockNode:
			n.Kind, n.Name, n.Start, n.End = "directive", v.name, v.open.start, v.close.stop
			for _, a := range v.attrs {
				n.Attrs[a.name] = a.value
			}
			n.Text = string(source[v.open.stop:v.close.start])
		case *inlineNode:
			n.Kind, n.Name, n.Start, n.End, n.Inline = "directive", v.name, v.open.start, v.close.stop, true
			for _, a := range v.attrs {
				n.Attrs[a.name] = a.value
			}
			n.Text = string(source[v.content.start:v.content.stop])
		case *ast.Heading:
			n.Kind, n.Level = "heading", v.Level
		case *ast.Paragraph, *ast.TextBlock:
			if depth > 0 {
				return ast.WalkContinue, nil
			}
			n.Kind = "paragraph"
		case *ast.Link:
			n.Kind, n.Destination = "link", string(v.Destination)
		case *ast.Image:
			n.Kind, n.Destination = "image", string(v.Destination)
		default:
			return ast.WalkContinue, nil
		}
		if n.Kind != "directive" {
			n.Text = string(node.Text(source))
			if node.Type() == ast.TypeBlock && node.Lines().Len() > 0 {
				n.Start, n.End = node.Lines().At(0).Start, node.Lines().At(node.Lines().Len()-1).Stop
			} else {
				_ = ast.Walk(node, func(child ast.Node, enter bool) (ast.WalkStatus, error) {
					if t, ok := child.(*ast.Text); ok && enter {
						if n.End == 0 {
							n.Start = t.Segment.Start
						}
						n.End = t.Segment.Stop
					}
					return ast.WalkContinue, nil
				})
			}
		}
		n.Start += base
		n.End += base
		nodes = append(nodes, n)
		if v, ok := node.(*inlineNode); ok {
			children, err := inspect(source[v.content.start:v.content.stop], base+v.content.start, depth+1)
			if err != nil {
				return ast.WalkStop, err
			}
			nodes = append(nodes, children...)
		}
		return ast.WalkContinue, nil
	})
	return nodes, err
}
