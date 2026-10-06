package markup

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
)

type Section struct {
	Anchor string `json:"anchor"`
	Title  string `json:"title"`
	Level  int    `json:"level"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
}

func Sections(source []byte) ([]Section, error) {
	root, _ := run(headingParser, source)
	var sections []Section
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := node.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		id, _ := heading.AttributeString("id")
		anchor, _ := id.([]byte)
		start := 0
		if heading.Lines().Len() > 0 {
			start = bytes.LastIndexByte(source[:heading.Lines().At(0).Start], '\n') + 1
		}
		sections = append(sections, Section{Anchor: string(anchor), Title: string(heading.Lines().Value(source)), Level: heading.Level, Start: start, End: len(source)})
		return ast.WalkSkipChildren, nil
	})
	for i := range sections {
		for _, next := range sections[i+1:] {
			if next.Level <= sections[i].Level {
				sections[i].End = next.Start
				break
			}
		}
	}
	return sections, err
}
