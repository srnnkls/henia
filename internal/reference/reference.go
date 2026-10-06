package reference

import (
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

func Skills(body string) []Reference {
	var refs []Reference
	tree, _ := markup.Tree([]byte(body))
	recognized, _ := Recognize(tree)
	for _, ref := range recognized {
		if ref.Type == TypeSkill {
			refs = append(refs, ref)
		}
	}
	addresses, _ := Addresses(tree)
	return append(refs, addresses...)
}
