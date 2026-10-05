package markup

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

type Link struct {
	Start, End int
	Text, Dest string
}

func Links(source []byte) []Link {
	p := &directiveParser{}
	md := goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithParserOptions(directiveOptions(p)...))
	doc := md.Parser().Parse(text.NewReader(source))
	var links []Link
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		link, ok := n.(*ast.Link)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		start, stop := -1, -1
		_ = ast.Walk(link, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
			if t, ok := c.(*ast.Text); ok && entering {
				if start < 0 {
					start = t.Segment.Start
				}
				stop = t.Segment.Stop
			}
			return ast.WalkContinue, nil
		})
		if start < 0 {
			return ast.WalkSkipChildren, nil
		}
		for start > 0 && strings.ContainsRune("`*_~", rune(source[start-1])) {
			start--
		}
		for stop < len(source) && strings.ContainsRune("`*_~", rune(source[stop])) {
			stop++
		}
		if start < 1 || source[start-1] != '[' || stop+2 > len(source) || string(source[stop:stop+2]) != "](" {
			return ast.WalkSkipChildren, nil
		}
		depth, end := 0, -1
	scan:
		for i := stop + 2; i < len(source); i++ {
			switch source[i] {
			case '(':
				depth++
			case ')':
				if depth == 0 {
					end = i + 1
					break scan
				}
				depth--
			case '\n':
				break scan
			}
		}
		if end > 0 {
			links = append(links, Link{Start: start - 1, End: end, Text: string(source[start:stop]), Dest: string(link.Destination)})
		}
		return ast.WalkSkipChildren, nil
	})
	return links
}
