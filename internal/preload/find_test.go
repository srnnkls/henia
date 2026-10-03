package preload

import (
	"slices"
	"testing"
)

func TestFind(t *testing.T) {
	for _, c := range []struct {
		name, source string
		want         []string
	}{
		{"inline", "Status: !`git status`.\n", []string{"git status"}},
		{"inline with double ticks", "A !``echo `x` `` b\n", []string{"echo `x`"}},
		{"inline in a list item", "- !`date`\n- !`uname`\n", []string{"date", "uname"}},
		{"fenced", "```!\necho a\necho b\n```\n", []string{"echo a\necho b"}},
		{"fenced with tildes", "~~~!\ndate\n~~~\n", []string{"date"}},
		{"plain code span", "Run `date`.\n", nil},
		{"bang separated by a space", "Wow! `date`\n", nil},
		{"example inside a code span", "Write `` !`date` `` to preload.\n", nil},
		{"example inside a fenced block", "```markdown\nStatus: !`git status`\n```\n", nil},
		{"example inside an indented block", "    !`date`\n", nil},
		{"fence with another info string", "```bash !\ndate\n```\n", nil},
		{"fence with a bang language", "```!bash\ndate\n```\n", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, p := range Find([]byte(c.source)) {
				got = append(got, p.Command)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestFindSpans(t *testing.T) {
	source := "- a !`date` b\n\n  ```!\n  uname\n  ```\nend\n"
	found := Find([]byte(source))
	if len(found) != 2 {
		t.Fatalf("found %d preloads", len(found))
	}
	if got := source[found[0].Start:found[0].Stop]; got != "!`date`" || found[0].Indent != "  " || found[0].Block {
		t.Fatalf("inline span %q indent %q", got, found[0].Indent)
	}
	if got := source[found[1].Start:found[1].Stop]; got != "  ```!\n  uname\n  ```\n" || found[1].Indent != "  " || !found[1].Block {
		t.Fatalf("fenced span %q indent %q", got, found[1].Indent)
	}
}
