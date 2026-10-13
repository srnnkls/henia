package reference

import (
	"regexp"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/yuin/goldmark/ast"
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
	Type     Type
	Name     string
	Raw      string
	Resource string
	Module   string
	Anchor   string
	// Start and End delimit the reference's single-backtick span in source bytes.
	Start int
	End   int
}

var referencePattern = regexp.MustCompile(`^([$/@#!])([a-zA-Z0-9._/-]+)$`)

var artifactName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func Parse(body string) []Reference {
	var refs []Reference
	source := []byte(body)
	root := markup.ParseHTMLAsText(source)
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
		if strings.Contains("$/@", sigil) && !artifactName.MatchString(name) {
			return ast.WalkSkipChildren, nil
		}
		if sigil == "#" && !strings.ContainsAny(name, "./") {
			return ast.WalkSkipChildren, nil
		}

		refType, ok := Sigils[sigil]
		if !ok {
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

var shown = regexp.MustCompile("`henia show '?([^`'\\s]+)'?")

func Skills(body string) []Reference {
	var refs []Reference
	for _, ref := range Parse(body) {
		if ref.Type == TypeSkill {
			refs = append(refs, ref)
		}
	}
	return append(refs, Shown(body)...)
}

func Shown(body string) []Reference {
	var refs []Reference
	for _, m := range shown.FindAllStringSubmatchIndex(body, -1) {
		a, err := ParseAddress(body[m[2]:m[3]])
		if err != nil || a.Kind != TypeSkill {
			continue
		}
		refs = append(refs, Reference{
			Type: TypeSkill, Name: a.Ref(), Resource: a.Path(), Module: strings.Join(a.Modules(), "."), Anchor: a.Anchor,
			Raw: body[m[0]+1 : m[1]], Start: m[0], End: m[1],
		})
	}
	return refs
}
