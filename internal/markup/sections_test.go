package markup_test

import (
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestSectionsIgnoreRuleAfterDirectiveClose(t *testing.T) {
	sections, err := markup.Sections([]byte("# B\n\n:::note\nDo X.\n:::\n---\n\n## Usage\n\ntext\n"))
	if err != nil {
		t.Fatal(err)
	}
	var anchors []string
	for _, s := range sections {
		anchors = append(anchors, s.Anchor)
	}
	if len(anchors) != 2 || anchors[0] != "b" || anchors[1] != "usage" {
		t.Errorf("anchors = %q, want [b usage]", anchors)
	}
}
