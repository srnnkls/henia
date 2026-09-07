package lint

import (
	"bytes"
	"text/template"
	"text/template/parse"
)

// templateMask uses Go's parse trees to retain literal Markdown and replace
// unevaluated actions with scalar placeholders. Coordinates remain unchanged.
func templateMask(source []byte) ([]byte, error) {
	if !bytes.Contains(source, []byte("{{")) {
		return source, nil
	}
	tmpl, err := template.New("skill").Parse(string(source))
	if err != nil {
		return nil, err
	}
	literal := make([]bool, len(source))
	var visit func(parse.Node)
	visit = func(node parse.Node) {
		switch n := node.(type) {
		case *parse.TextNode:
			for i := int(n.Pos); i < int(n.Pos)+len(n.Text) && i < len(literal); i++ {
				literal[i] = true
			}
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					visit(child)
				}
			}
		case *parse.IfNode:
			visit(n.List)
			visit(n.ElseList)
		case *parse.RangeNode:
			visit(n.List)
			visit(n.ElseList)
		case *parse.WithNode:
			visit(n.List)
			visit(n.ElseList)
		}
	}
	for _, tree := range tmpl.Templates() {
		visit(tree.Tree.Root)
	}
	masked := bytes.Clone(source)
	start := 0
	for start < len(source) {
		end := bytes.IndexByte(source[start:], '\n')
		if end < 0 {
			end = len(source)
		} else {
			end += start + 1
		}
		hasText := false
		for i := start; i < end; i++ {
			if literal[i] && source[i] != ' ' && source[i] != '\t' && source[i] != '\n' && source[i] != '\r' {
				hasText = true
				break
			}
		}
		for i := start; i < end; i++ {
			if !literal[i] && source[i] != '\n' && source[i] != '\r' {
				masked[i] = ' '
				if hasText {
					masked[i] = 'x'
				}
			}
		}
		start = end
	}
	return masked, nil
}
