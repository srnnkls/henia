package library

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
)

func TestCorpus(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, ProjectDir, SkillsDir)
	write(t, filepath.Join(skills, "a", "SKILL.md"), "---\nname: a\ndescription: A.\n---\n\n# A\n\nUse `$b` and `henia show c#x`.\n")
	write(t, filepath.Join(skills, "a", "ops", "run.md"), "# Run\n\nSee [b](../../b/SKILL.md), [self](../SKILL.md#run) and [up](#top).\n")
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
			targets = append(targets, e.Attrs["target"]+"/"+e.Attrs["path"]+"#"+e.Attrs["anchor"])
		}
		return true
	})
	if got := strings.Join(files, " "); got != "SKILL.md:true ops/run.md:false run.sh:false" {
		t.Errorf("files = %s", got)
	}
	if got := strings.Join(targets, " "); got != "b/# c/SKILL.md#x b/SKILL.md# a/SKILL.md#run a/ops/run.md#top" {
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

func countFixture(t *testing.T) *Library {
	t.Helper()
	root := t.TempDir()
	skills := filepath.Join(root, ProjectDir, SkillsDir)
	write(t, filepath.Join(skills, "a", "SKILL.md"), "---\nname: a\ndescription: Alpha beta gamma.\n---\n# Title\n\nOne two three")
	write(t, filepath.Join(skills, "a", "run.sh"), "echo one\n\necho two")
	return Open(root, nil)
}

func matches(t *testing.T, corpus *Corpus, query string) int {
	t.Helper()
	q, err := pattern.Read(query)
	if err != nil {
		t.Fatalf("read %q: %v", query, err)
	}
	rows, err := q.Run(corpus.Root, pattern.Environment{Resolve: corpus})
	if err != nil {
		t.Fatalf("run %q: %v", query, err)
	}
	return len(rows)
}

func TestCountsAuthored(t *testing.T) {
	corpus := countFixture(t).Corpus(nil)
	for query, want := range map[string]int{
		`(skill :id "a" (file :path "SKILL.md" :words 4))`:  1,
		`(skill :id "a" (file :path "SKILL.md" :chars 22))`: 1,
		`(skill :id "a" (file :path "SKILL.md" :lines 3))`:  1,
		`(skill :id "a" (file :path "run.sh" :words 4))`:    1,
		`(skill :id "a" (file :path "run.sh" :chars 18))`:   1,
		`(skill :id "a" (file :path "run.sh" :lines 3))`:    1,
		`(skill :id "a" :words 8)`:                          1,
		`(skill :id "a" :chars 40)`:                         1,
		`(skill :id "a" :lines 6)`:                          1,
		`(skill :id "a" (file :contains "one"))`:            0,
		`(skill :id "a" :contains "one")`:                   0,
	} {
		if got := matches(t, corpus, query); got != want {
			t.Errorf("%s: %d rows, want %d", query, got, want)
		}
	}
}

func TestCountsRendered(t *testing.T) {
	corpus := countFixture(t).Corpus(&Rendering{
		Body:      func(Entry) string { return "# Title\n\nOne two three four five\n\nsix" },
		Reference: func(Entry) *regexp.Regexp { return nil },
	})
	for query, want := range map[string]int{
		`(skill :id "a" (file :path "SKILL.md" :words 7))`:  1,
		`(skill :id "a" (file :path "SKILL.md" :chars 37))`: 1,
		`(skill :id "a" (file :path "SKILL.md" :lines 5))`:  1,
		`(skill :id "a" :words 11)`:                         1,
		`(skill :id "a" :chars 55)`:                         1,
		`(skill :id "a" :lines 8)`:                          1,
	} {
		if got := matches(t, corpus, query); got != want {
			t.Errorf("%s: %d rows, want %d", query, got, want)
		}
	}
}

func TestCountsDependencyStub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "SKILL.md")
	write(t, path, "---\nname: a\ndescription: Alpha beta gamma.\n---\n# Title\n\nOne two three")
	corpus := Documents([]Document{{Path: path, Kind: "skill", Dependency: true}})
	for query, want := range map[string]int{
		`(skill :id "a" (file :path "SKILL.md" :lines 3))`: 1,
		`(skill :id "a" (file :path "SKILL.md" :words 4))`: 1,
		`(skill :id "a" :lines 3)`:                         1,
		`(skill :id "a" :words 4)`:                         1,
	} {
		if got := matches(t, corpus, query); got != want {
			t.Errorf("%s: %d rows, want %d", query, got, want)
		}
	}
}

func TestCountsMalformedFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "SKILL.md")
	write(t, path, "---\nbroken: [\n---\none two\n")
	corpus := Documents([]Document{{Path: path, Kind: "skill"}})
	for query, want := range map[string]int{
		`(skill :id "a" (file :path "SKILL.md" :words 2))`:     1,
		`(skill :id "a" (file :path "SKILL.md" :chars 8))`:     1,
		`(skill :id "a" (file :path "SKILL.md" :lines 1))`:     1,
		`(skill :id "a" (file (problem :kind "frontmatter")))`: 1,
	} {
		if got := matches(t, corpus, query); got != want {
			t.Errorf("%s: %d rows, want %d", query, got, want)
		}
	}
}

func TestCountsEmptyBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "SKILL.md")
	write(t, path, "---\nname: a\ndescription: Alpha.\n---\n")
	corpus := Documents([]Document{{Path: path, Kind: "skill"}})
	if got := matches(t, corpus, `(skill :id "a" (file :path "SKILL.md" :lines 0))`); got != 1 {
		t.Errorf("empty body :lines 0: %d rows, want 1", got)
	}
}

func TestCountsMalformedFrontmatterCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "SKILL.md")
	write(t, path, "---\r\nbroken: [\r\n---\r\none\r\n")
	corpus := Documents([]Document{{Path: path, Kind: "skill"}})
	for query, want := range map[string]int{
		`(skill :id "a" (file :path "SKILL.md" :words 1))`:     1,
		`(skill :id "a" (file :path "SKILL.md" :chars 5))`:     1,
		`(skill :id "a" (file :path "SKILL.md" :lines 1))`:     1,
		`(skill :id "a" (file (problem :kind "frontmatter")))`: 1,
	} {
		if got := matches(t, corpus, query); got != want {
			t.Errorf("%s: %d rows, want %d", query, got, want)
		}
	}
}

func TestCountsMalformedFrontmatterClosedAtEOF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "SKILL.md")
	write(t, path, "---\nbroken: [\n---")
	corpus := Documents([]Document{{Path: path, Kind: "skill"}})
	for query, want := range map[string]int{
		`(skill :id "a" (file :path "SKILL.md" :words 0))`:     1,
		`(skill :id "a" (file :path "SKILL.md" :lines 0))`:     1,
		`(skill :id "a" (file (problem :kind "frontmatter")))`: 1,
	} {
		if got := matches(t, corpus, query); got != want {
			t.Errorf("%s: %d rows, want %d", query, got, want)
		}
	}
}
