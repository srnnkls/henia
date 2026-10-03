// Package preload finds, checks and runs the shell commands a skill embeds with
// Claude Code's preload syntax.
package preload

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type Preload struct {
	Start, Stop int
	Command     string
	Block       bool
	Indent      string
}

func Find(source []byte) []Preload {
	var found []Preload
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.CodeSpan:
			if p, ok := inline(source, node); ok {
				found = append(found, p)
			}
		case *ast.FencedCodeBlock:
			if p, ok := fenced(source, node); ok {
				found = append(found, p)
			}
		}
		return ast.WalkContinue, nil
	})
	return found
}

func inline(source []byte, span *ast.CodeSpan) (Preload, bool) {
	prev, ok := span.PreviousSibling().(*ast.Text)
	if !ok || prev.Segment.Stop == 0 || source[prev.Segment.Stop-1] != '!' {
		return Preload{}, false
	}
	first, ok := span.FirstChild().(*ast.Text)
	if !ok {
		return Preload{}, false
	}
	open := prev.Segment.Stop
	ticks := 0
	for open+ticks < len(source) && source[open+ticks] == '`' {
		ticks++
	}
	if ticks == 0 || open+ticks > first.Segment.Start+1 {
		return Preload{}, false
	}
	var command strings.Builder
	last := first.Segment.Stop
	for c := span.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			command.Write(t.Segment.Value(source))
			if t.SoftLineBreak() {
				command.WriteByte(' ')
			}
			last = t.Segment.Stop
		}
	}
	closing := bytes.Index(source[last:], bytes.Repeat([]byte{'`'}, ticks))
	if closing < 0 {
		return Preload{}, false
	}
	lineStart := bytes.LastIndexByte(source[:open], '\n') + 1
	return Preload{
		Start:   open - 1,
		Stop:    last + closing + ticks,
		Command: strings.TrimSpace(command.String()),
		Indent:  contentIndent(source[lineStart:open]),
	}, true
}

func fenced(source []byte, block *ast.FencedCodeBlock) (Preload, bool) {
	if block.Info == nil || strings.TrimSpace(string(block.Info.Segment.Value(source))) != "!" {
		return Preload{}, false
	}
	lineStart := bytes.LastIndexByte(source[:block.Info.Segment.Start], '\n') + 1
	fenceLine := source[lineStart:block.Info.Segment.Start]
	indent := fenceLine[:len(fenceLine)-len(bytes.TrimLeft(fenceLine, " \t"))]
	fence := bytes.TrimSpace(fenceLine)
	var command strings.Builder
	end := lineEnd(source, block.Info.Segment.Stop)
	lines := block.Lines()
	for i := range lines.Len() {
		line := lines.At(i)
		command.Write(line.Value(source))
		end = lineEnd(source, line.Stop)
	}
	if end < len(source) {
		closing := source[end:lineEnd(source, end)]
		if trimmed := bytes.TrimSpace(closing); len(trimmed) >= len(fence) && trimmed[0] == fence[0] && len(bytes.Trim(trimmed, string(fence[0]))) == 0 {
			end = lineEnd(source, end)
		}
	}
	return Preload{
		Start:   lineStart,
		Stop:    end,
		Command: strings.TrimRight(command.String(), "\n"),
		Block:   true,
		Indent:  string(indent),
	}, true
}

func lineEnd(source []byte, from int) int {
	if from >= len(source) {
		return len(source)
	}
	if i := bytes.IndexByte(source[from:], '\n'); i >= 0 {
		return from + i + 1
	}
	return len(source)
}

func contentIndent(prefix []byte) string {
	trimmed := bytes.TrimLeft(prefix, " \t")
	indent := len(prefix) - len(trimmed)
	marker := 0
	switch {
	case len(trimmed) >= 2 && bytes.ContainsRune([]byte("-*+"), rune(trimmed[0])) && trimmed[1] == ' ':
		marker = 2
	default:
		digits := 0
		for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
			digits++
		}
		if digits > 0 && digits+1 < len(trimmed) && (trimmed[digits] == '.' || trimmed[digits] == ')') && trimmed[digits+1] == ' ' {
			marker = digits + 2
		}
	}
	return string(prefix[:indent]) + strings.Repeat(" ", marker)
}
