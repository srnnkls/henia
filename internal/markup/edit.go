package markup

import (
	"fmt"
	"slices"
	"strings"
)

type Edit struct {
	Start, End int
	Text, Rule string
}

type Applied struct {
	Text    string
	Dropped []Edit
	kept    []Edit
	out     []int
}

func Apply(source string, edits []Edit) Applied {
	edits = slices.Clone(edits)
	slices.SortStableFunc(edits, func(a, b Edit) int { return a.Start - b.Start })
	var a Applied
	var out strings.Builder
	position := 0
	for _, e := range edits {
		if len(a.kept) > 0 {
			last := a.kept[len(a.kept)-1]
			if last.Start <= e.Start && e.End <= last.End {
				continue
			}
		}
		if e.Start < position {
			a.Dropped = append(a.Dropped, e)
			continue
		}
		out.WriteString(source[position:e.Start])
		a.out = append(a.out, out.Len())
		out.WriteString(e.Text)
		a.kept = append(a.kept, e)
		position = e.End
	}
	out.WriteString(source[position:])
	a.Text = out.String()
	return a
}

func (a Applied) Offset(offset int) (int, bool) {
	delta := 0
	for _, e := range a.kept {
		if e.Start < offset && offset < e.End {
			return 0, false
		}
		if e.End <= offset {
			delta += len(e.Text) - (e.End - e.Start)
		}
	}
	return offset + delta, true
}

func (a Applied) Span(start, end int) (int, int, string, bool) {
	for i, e := range a.kept {
		if e.Start == start && e.End == end {
			return a.out[i], a.out[i] + len(e.Text), e.Rule, true
		}
		if e.Start < end && start < e.End {
			return 0, 0, "", false
		}
	}
	from, ok := a.Offset(start)
	to, _ := a.Offset(end)
	return from, to, "", ok
}

func DirectiveEdits(root *Element, format string) ([]Edit, error) {
	if format != "" && format != "directives" && format != "xml" && format != "markdown" {
		return nil, fmt.Errorf("unsupported markup format %q", format)
	}
	var edits []Edit
	root.Walk(func(e *Element) bool {
		if e.directive == nil {
			return true
		}
		inline := e.Attrs["inline"] == "true"
		open, close := delimiters(*e.directive, format, inline)
		edits = append(edits, Edit{e.directive.open.start, e.directive.open.stop, open, "directive"}, Edit{e.directive.close.start, e.directive.close.stop, close, "directive"})
		return true
	})
	return edits, nil
}

type Link struct {
	Start, End int
	Text, Dest string
}

func LinkSyntax(source string, link *Element) (Link, bool) {
	start, stop := link.Start, link.End
	if link.Type != "link" || start < 0 || stop > len(source) || start >= stop {
		return Link{}, false
	}
	for start > 0 && strings.ContainsRune("`*_~", rune(source[start-1])) {
		start--
	}
	for stop < len(source) && strings.ContainsRune("`*_~", rune(source[stop])) {
		stop++
	}
	if start < 1 || source[start-1] != '[' || stop+2 > len(source) || source[stop:stop+2] != "](" {
		return Link{}, false
	}
	depth := 0
	for i := stop + 2; i < len(source); i++ {
		switch source[i] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return Link{Start: start - 1, End: i + 1, Text: source[start:stop], Dest: link.Attrs["url"]}, true
			}
			depth--
		case '\n':
			return Link{}, false
		}
	}
	return Link{}, false
}
