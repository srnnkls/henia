package templating

import (
	"bytes"
	"fmt"
	"text/template"
	"text/template/parse"
)

const absent = "henia_absent"

var Funcs = template.FuncMap{absent: blankAbsent}

func ExecuteTemplate[T any](content string, vars map[string]T) (string, error) {
	tmpl, err := template.New("content").Funcs(Funcs).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	BlankAbsent(tmpl)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

func BlankAbsent(tmpl *template.Template) {
	for _, t := range tmpl.Templates() {
		if t.Tree != nil {
			printAbsentBlank(t.Tree.Root)
		}
	}
}

func blankAbsent(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func printAbsentBlank(node parse.Node) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				printAbsentBlank(child)
			}
		}
	case *parse.ActionNode:
		if len(n.Pipe.Decl) == 0 {
			n.Pipe.Cmds = append(n.Pipe.Cmds, &parse.CommandNode{
				NodeType: parse.NodeCommand,
				Pos:      n.Pos,
				Args:     []parse.Node{parse.NewIdentifier(absent).SetPos(n.Pos)},
			})
		}
	case *parse.IfNode:
		printAbsentBlank(n.List)
		printAbsentBlank(n.ElseList)
	case *parse.RangeNode:
		printAbsentBlank(n.List)
		printAbsentBlank(n.ElseList)
	case *parse.WithNode:
		printAbsentBlank(n.List)
		printAbsentBlank(n.ElseList)
	}
}
