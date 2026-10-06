package reference

import (
	"regexp"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
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

var shown = regexp.MustCompile("`henia show ([a-z0-9][a-z0-9._:-]*)(?:/([^`#\\s]+))?(?:#([^`\\s]+))?")

func Skills(body string) []Reference {
	var refs []Reference
	tree, _ := markup.Tree([]byte(body))
	recognized, _ := Recognize(tree)
	for _, ref := range recognized {
		if ref.Type == TypeSkill {
			refs = append(refs, ref)
		}
	}
	return append(refs, Shown(body)...)
}

func Shown(body string) []Reference {
	var refs []Reference
	for _, m := range shown.FindAllStringSubmatchIndex(body, -1) {
		ref := Reference{Type: TypeSkill, Name: body[m[2]:m[3]], Raw: body[m[0]+1 : m[1]], Start: m[0], End: m[1]}
		if m[4] >= 0 {
			ref.Resource = body[m[4]:m[5]]
		} else {
			prefix, name := "", ref.Name
			if pkg, rest, qualified := strings.Cut(name, ":"); qualified {
				prefix, name = pkg+":", rest
			}
			if skill, module, dotted := strings.Cut(name, "."); dotted {
				ref.Name, ref.Module = prefix+skill, module
			}
		}
		if m[6] >= 0 {
			ref.Anchor = body[m[6]:m[7]]
		}
		refs = append(refs, ref)
	}
	return refs
}
