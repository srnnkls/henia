package markup

import (
	"bytes"
	"text/template"
	templateparse "text/template/parse"
)

// MaskTemplates uses Go's parse trees to retain literal Markdown and replace
// unevaluated actions with scalar placeholders. Coordinates remain unchanged.
func MaskTemplates(source []byte) ([]byte, error) {
	if !bytes.Contains(source, []byte("{{")) {
		return source, nil
	}
	tmpl, err := template.New("skill").Parse(string(source))
	if err != nil {
		return nil, err
	}
	literal := make([]bool, len(source))
	var visit func(templateparse.Node)
	visit = func(node templateparse.Node) {
		switch n := node.(type) {
		case *templateparse.TextNode:
			for i := int(n.Pos); i < int(n.Pos)+len(n.Text) && i < len(literal); i++ {
				literal[i] = true
			}
		case *templateparse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					visit(child)
				}
			}
		case *templateparse.IfNode:
			visit(n.List)
			visit(n.ElseList)
		case *templateparse.RangeNode:
			visit(n.List)
			visit(n.ElseList)
		case *templateparse.WithNode:
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
