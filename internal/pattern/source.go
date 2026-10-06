package pattern

import (
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

type Block struct {
	Start, Line int
	Text        string
}

func Source(tree *markup.Element, shift int) (string, []Block) {
	var source strings.Builder
	var blocks []Block
	tree.Walk(func(e *markup.Element) bool {
		if e.Type == "code" && e.Attrs["lang"] == "hq" {
			blocks = append(blocks, Block{source.Len(), e.Line + 1 + shift, e.Text})
			source.WriteString(e.Text + "\n")
		}
		return true
	})
	return source.String(), blocks
}
