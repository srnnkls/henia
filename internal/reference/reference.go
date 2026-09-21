package reference

import (
	"regexp"
	"slices"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type Type string

const (
	TypeSkill   Type = "skill"
	TypeCommand Type = "command"
	TypeAgent   Type = "agent"
	TypeFile    Type = "file"
	TypeTool    Type = "tool"
)

func (t Type) String() string {
	return string(t)
}

type Reference struct {
	Type Type
	Name string
	Raw  string
	// Start and End delimit the reference's single-backtick span in source bytes.
	Start int
	End   int
}

var referencePattern = regexp.MustCompile(`^([$/@#!])([a-zA-Z0-9._/-]+)$`)

func Parse(body string) []Reference {
	var refs []Reference
	source := []byte(body)
	// Rendered XML directives still contain Markdown references. Keep their
	// contents visible to inline parsing while retaining Markdown code blocks.
	blocks := slices.DeleteFunc(parser.DefaultBlockParsers(), func(p util.PrioritizedValue) bool {
		return p.Value == parser.NewHTMLBlockParser()
	})
	p := parser.NewParser(parser.WithBlockParsers(blocks...), parser.WithInlineParsers(parser.DefaultInlineParsers()...))
	root := p.Parse(text.NewReader(source))
	ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		span, ok := node.(*ast.CodeSpan)
		if !entering || !ok || span.ChildCount() != 1 {
			return ast.WalkContinue, nil
		}
		segment := span.FirstChild().(*ast.Text).Segment
		start, end := segment.Start-1, segment.Stop+1
		if start < 0 || end > len(source) || source[start] != '`' || source[end-1] != '`' ||
			(start > 0 && source[start-1] == '`') || (end < len(source) && source[end] == '`') {
			return ast.WalkSkipChildren, nil
		}
		content := string(segment.Value(source))
		refMatch := referencePattern.FindStringSubmatch(content)
		if refMatch == nil {
			return ast.WalkSkipChildren, nil
		}

		sigil := refMatch[1]
		name := refMatch[2]

		var refType Type
		switch sigil {
		case "$":
			refType = TypeSkill
		case "/":
			refType = TypeCommand
		case "@":
			refType = TypeAgent
		case "#":
			refType = TypeFile
		case "!":
			refType = TypeTool
		default:
			return ast.WalkSkipChildren, nil
		}

		refs = append(refs, Reference{
			Type:  refType,
			Name:  name,
			Raw:   content,
			Start: start,
			End:   end,
		})
		return ast.WalkSkipChildren, nil
	})

	return refs
}
