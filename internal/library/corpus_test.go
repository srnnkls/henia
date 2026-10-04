package library

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestCorpus(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, ProjectDir, SkillsDir)
	write(t, filepath.Join(skills, "a", "SKILL.md"), "---\nname: a\ndescription: A.\n---\n\n# A\n\nUse `$b` and `henia show c#x`.\n")
	write(t, filepath.Join(skills, "a", "ops", "run.md"), "# Run\n\nSee [b](../../b/SKILL.md) and [self](../SKILL.md).\n")
	write(t, filepath.Join(skills, "a", "run.sh"), "echo one\n\necho two\n")
	write(t, filepath.Join(skills, "a", "blob.bin"), "\x00\x01")
	write(t, filepath.Join(skills, "b", "SKILL.md"), "# B\n")
	write(t, filepath.Join(skills, "c", "SKILL.md"), "# C\n")
	corpus := Open(root, nil).Corpus(nil)
	a := corpus.Skill("a")
	var files, targets []string
	a.Walk(func(e *markup.Element) bool {
		switch {
		case e.Type == "file":
			files = append(files, e.Attrs["path"]+":"+e.Attrs["main"])
		case e.Type == "link":
			targets = append(targets, e.Attrs["target"])
		}
		return true
	})
	if got := strings.Join(files, " "); got != "SKILL.md:true ops/run.md:false run.sh:false" {
		t.Errorf("files = %s", got)
	}
	if got := strings.Join(targets, " "); got != "b c b " {
		t.Errorf("targets = %q", got)
	}
	main := a.Children[0]
	if head := main.Children[0]; head.Type != "frontmatter" || head.Attrs["description"] != "A." {
		t.Errorf("frontmatter = %+v", head)
	}
	if heading := main.Children[1].Children[0]; heading.Line != 6 {
		t.Errorf("heading line = %d, want 6 after frontmatter", heading.Line)
	}
	if script := a.Children[2]; len(script.Children) != 2 {
		t.Errorf("run.sh paragraphs = %d", len(script.Children))
	}
}
