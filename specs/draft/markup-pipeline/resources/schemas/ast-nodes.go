//go:build ignore

// AST Node Schemas
// Location: internal/fenceddiv/ast.go

package fenceddiv

import "github.com/yuin/goldmark/ast"

// KindFencedDiv is a NodeKind of the FencedDivNode
var KindFencedDiv = ast.NewNodeKind("FencedDiv")

// KindInlineDirective is a NodeKind of the InlineDirectiveNode
var KindInlineDirective = ast.NewNodeKind("InlineDirective")

// FencedDivNode represents a block fenced div
// Syntax: :::name{attr="value"}
type FencedDivNode struct {
    ast.BaseBlock
    Name       string
    Attributes map[string]string
}

// Kind implements Node.Kind
func (n *FencedDivNode) Kind() ast.NodeKind {
    return KindFencedDiv
}

// Dump implements Node.Dump
func (n *FencedDivNode) Dump(source []byte, level int) {
    ast.DumpHelper(n, source, level, map[string]string{
        "Name": n.Name,
    }, nil)
}

// NewFencedDivNode creates a new FencedDivNode
func NewFencedDivNode(name string, attrs map[string]string) *FencedDivNode {
    return &FencedDivNode{
        Name:       name,
        Attributes: attrs,
    }
}

// InlineDirectiveNode represents an inline directive
// Syntax: :name[content]{attr="value"}
type InlineDirectiveNode struct {
    ast.BaseInline
    Name       string
    Content    []byte
    Attributes map[string]string
}

// Kind implements Node.Kind
func (n *InlineDirectiveNode) Kind() ast.NodeKind {
    return KindInlineDirective
}

// Dump implements Node.Dump
func (n *InlineDirectiveNode) Dump(source []byte, level int) {
    ast.DumpHelper(n, source, level, map[string]string{
        "Name":    n.Name,
        "Content": string(n.Content),
    }, nil)
}

// NewInlineDirectiveNode creates a new InlineDirectiveNode
func NewInlineDirectiveNode(name string, content []byte, attrs map[string]string) *InlineDirectiveNode {
    return &InlineDirectiveNode{
        Name:       name,
        Content:    content,
        Attributes: attrs,
    }
}
