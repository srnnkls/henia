package markup_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func outline(e *markup.Element, depth int, b *strings.Builder) {
	label := e.Type
	if id := e.Attrs["id"]; id != "" {
		label += "#" + id
	}
	if lang := e.Attrs["lang"]; lang != "" {
		label += ":" + lang
	}
	fmt.Fprintf(b, "%s%s L%d-%d\n", strings.Repeat("  ", depth), label, e.Line, e.EndLine)
	for _, c := range e.Children {
		outline(c, depth+1, b)
	}
}

func TestTree(t *testing.T) {
	source := "# Title\n\nIntro [x](u).\n\n## Usage\n\nFirst.\n\n```bash\necho\n```\n\n- one\n- two\n\n### Deep\n\n> quote\n\n## Usage\n\n:::note\ninside\n:::\n"
	root, err := markup.Tree([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	outline(root, 0, &b)
	want := `file L1-24
  section#title L1-24
    heading L1-1
    paragraph L3-3
      link L3-3
    section#usage L5-18
      heading L5-5
      paragraph L7-7
      code:bash L9-11
      list L13-14
        item L13-13
          paragraph L13-13
        item L14-14
          paragraph L14-14
      section#deep L16-18
        heading L16-16
        quote L18-18
          paragraph L18-18
    section#usage-1 L20-24
      heading L20-20
      directive L22-24
        paragraph L23-23
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestTreeAnchorsMatchSections(t *testing.T) {
	source := []byte("# A\n\n## B b\n\n## B b\n\n### C: d\n")
	root, _ := markup.Tree(source)
	sections, _ := markup.Sections(source)
	var anchors []string
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "section" {
			anchors = append(anchors, e.Attrs["id"])
		}
		return true
	})
	for i, s := range sections {
		if anchors[i] != s.Anchor {
			t.Errorf("section %d: tree %q, Sections %q", i, anchors[i], s.Anchor)
		}
	}
}

func TestTreeSectionsInsideDirectives(t *testing.T) {
	source := []byte("# Skill\n\n:::note\n## Routes\n\nGo.\n\n## Slots\n\nNone.\n:::\n\n## Detail\n\nMore.\n")
	root, _ := markup.Tree(source)
	var got []string
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "section" {
			got = append(got, e.Attrs["id"]+"<"+e.Parent.Type)
		}
		return true
	})
	want := []string{"skill<file", "routes<directive", "slots<directive", "detail<section"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTreePlacesExtraElements(t *testing.T) {
	source := []byte("# A\n\nUse `$other` here.\n")
	start := strings.Index(string(source), "`$other`")
	root, _ := markup.Tree(source, &markup.Element{Type: "link", Attrs: map[string]string{"target": "other"}, Start: start, End: start + 8})
	link := root.Children[0].Children[1].Children[0]
	if link.Type != "link" || link.Text != "`$other`" || link.Line != 3 {
		t.Errorf("got %s %q L%d", link.Type, link.Text, link.Line)
	}
}

func TestPlainTree(t *testing.T) {
	root := markup.PlainTree([]byte("a\nb\n\n\nc\n"))
	var got []string
	for _, p := range root.Children {
		got = append(got, fmt.Sprintf("%q L%d-%d", p.Text, p.Line, p.EndLine))
	}
	if strings.Join(got, " ") != `"a\nb" L1-2 "c" L5-5` {
		t.Errorf("got %v", got)
	}
}

func TestConcurrentParsesKeepTheirOwnErrors(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			source, broken := "::: note\nbody\n:::\n", i%2 == 0
			if broken {
				source = "::: note\nbody\n"
			}
			_, err := markup.Tree([]byte(source))
			if (err != nil) != broken {
				t.Errorf("source %q: err = %v", source, err)
			}
		})
	}
	wg.Wait()
}
