package markup

import (
	"slices"
	"sync/atomic"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var (
	documentParser        = goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithParserOptions(append(directiveOptions(), parser.WithAutoHeadingID())...)).Parser()
	linkParser            = goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithParserOptions(directiveOptions()...)).Parser()
	directiveParser       = goldmark.New(goldmark.WithParserOptions(directiveOptions()...)).Parser()
	inlineDirectiveParser = parser.NewParser(
		parser.WithBlockParsers(util.Prioritized(parser.NewParagraphParser(), 1000)),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithInlineParsers(util.Prioritized(&inlineParser{}, 150)),
	)
	headingParser    = goldmark.New(goldmark.WithParserOptions(parser.WithAutoHeadingID())).Parser()
	commonMarkParser = goldmark.New().Parser()
	htmlTextParser   = parser.NewParser(
		parser.WithBlockParsers(slices.DeleteFunc(parser.DefaultBlockParsers(), func(p util.PrioritizedValue) bool {
			return p.Value == parser.NewHTMLBlockParser()
		})...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
	)
)

var parses atomic.Int64

func Parses() int64 { return parses.Load() }

func run(p parser.Parser, source []byte) (ast.Node, error) {
	parses.Add(1)
	pc := parser.NewContext()
	doc := p.Parse(text.NewReader(source), parser.WithContext(pc))
	err, _ := pc.Get(failure).(error)
	return doc, err
}

func ParseCommonMark(source []byte) ast.Node {
	doc, _ := run(commonMarkParser, source)
	return doc
}

func ParseHTMLAsText(source []byte) ast.Node {
	doc, _ := run(htmlTextParser, source)
	return doc
}
