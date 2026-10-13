package markup_test

import (
	"errors"
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

func TestTreeDirectiveSyntax(t *testing.T) {
	source := []byte(":::routes{ordered protocol}\n:::\n\n::item[see [doc](u) and :term[[ref](v)]]{#main k=\"a b\"}\n\nafter\n")
	root, err := markup.Tree(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 3 {
		t.Fatalf("got %d top-level elements, want routes, item and paragraph", len(root.Children))
	}
	routes, item, after := root.Children[0], root.Children[1], root.Children[2]
	if routes.Attrs["name"] != "routes" || routes.Attrs["ordered"] != "true" || routes.Attrs["protocol"] != "true" {
		t.Errorf("routes attrs = %v, want ordered and protocol true", routes.Attrs)
	}
	if item.Type != "directive" || item.Attrs["name"] != "item" || item.Attrs["inline"] == "true" || item.Line != 4 || item.EndLine != 4 {
		t.Fatalf("got %s %v L%d-%d, want leaf directive item on line 4", item.Type, item.Attrs, item.Line, item.EndLine)
	}
	if item.Attrs["id"] != "main" || item.Attrs["k"] != "a b" {
		t.Errorf("item attrs = %v, want id main and k \"a b\"", item.Attrs)
	}
	if after.Type != "paragraph" || after.Text != "after" {
		t.Errorf("got %s %q after the leaf, want paragraph \"after\"", after.Type, after.Text)
	}
	var label []string
	for _, c := range item.Children {
		label = append(label, c.Type+":"+c.Attrs["url"]+c.Attrs["name"])
		for _, g := range c.Children {
			label = append(label, "  "+g.Type+":"+g.Attrs["url"]+g.Attrs["name"])
		}
	}
	want := []string{"link:u", "directive:term", "  link:v"}
	if !slices.Equal(label, want) {
		t.Errorf("label children = %q, want %q", label, want)
	}
}

func TestTreeReportsEveryDirectiveError(t *testing.T) {
	source := []byte("intro\n\n:x[a]{k=1 k=2}\n\n:::menu\nbody\n")
	_, err := markup.Tree(source)
	var got []string
	var collect func(error)
	collect = func(err error) {
		switch e := err.(type) {
		case *markup.Error:
			got = append(got, e.Error())
		case interface{ Unwrap() []error }:
			for _, inner := range e.Unwrap() {
				collect(inner)
			}
		case interface{ Unwrap() error }:
			collect(e.Unwrap())
		}
	}
	collect(err)
	if len(got) != 2 || !strings.HasPrefix(got[0], "3:11: ") || !strings.Contains(got[0], "duplicate-attribute") ||
		!strings.HasPrefix(got[1], "5:1: ") || !strings.Contains(got[1], "unclosed-directive") {
		t.Fatalf("errors = %q, want duplicate-attribute at 3:11 and unclosed-directive at 5:1", got)
	}
}

func TestTreeLabelsResolveReferenceLinks(t *testing.T) {
	for _, source := range []string{":x[[doc][r]]\n\n[r]: https://example.test\n", ":x[:y[[doc][r]]]\n\n[r]: https://example.test\n"} {
		t.Run(source, func(t *testing.T) {
			root, err := markup.Tree([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			var urls []string
			root.Walk(func(e *markup.Element) bool {
				if e.Type == "link" {
					urls = append(urls, e.Attrs["url"])
				}
				return true
			})
			if !slices.Equal(urls, []string{"https://example.test"}) {
				t.Errorf("links = %q, want the reference destination", urls)
			}
		})
	}
}

func TestTreeKeepsEveryBlock(t *testing.T) {
	root, err := markup.Tree([]byte(":::x\n---\n:::\n\n<div>\nhi\n</div>\n\n> ***\n"))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	outline(root, 0, &b)
	want := `file L1-9
  directive L1-3
    rule L2-2
  html L5-7
  quote L9-9
    rule L9-9
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestTreeInlineCode(t *testing.T) {
	source := "Write `{{.name}}` in ``a ` b``.\n"
	root, err := markup.Tree([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range root.Children[0].Children {
		got = append(got, c.Type+":"+c.Attrs["inline"]+":"+source[c.Start:c.End])
	}
	want := []string{"code:true:`{{.name}}`", "code:true:``a ` b``"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTreeUnclosedEmptyDirectiveInItem(t *testing.T) {
	root, err := markup.Tree([]byte("- :::note\nb\n  :::\n"))
	var located *markup.Error
	if !errors.As(err, &located) || !strings.HasPrefix(located.Error(), "1:3: ") || !strings.Contains(located.Error(), "unclosed-directive") {
		t.Fatalf("err = %v, want unclosed-directive at 1:3", err)
	}
	note := root.Children[0].Children[0].Children[0]
	if note.Type != "directive" || note.Text != ":::note" || note.Start != 2 || note.End != 9 {
		t.Errorf("directive = %s %q [%d,%d), want directive \":::note\" [2,9)", note.Type, note.Text, note.Start, note.End)
	}
}

func TestTreeLinkSpansCodeSpanLabels(t *testing.T) {
	for _, c := range []struct{ source, code string }{
		{"[`peer path`](../peer/SKILL.md#x)\n", "`peer path`"},
		{"[see `b`](y.md)\n", "`b`"},
		{"[`a` then](y.md)\n", "`a`"},
		{"[``a`b``](y.md)\n", "``a`b``"},
		{"[*`e` f*](w.md)\n", "`e`"},
		{"[**`e`**](w.md)\n", "`e`"},
	} {
		t.Run(c.source, func(t *testing.T) {
			root, err := markup.Tree([]byte(c.source))
			if err != nil {
				t.Fatal(err)
			}
			var link, code *markup.Element
			root.Walk(func(e *markup.Element) bool {
				switch e.Type {
				case "link":
					link = e
				case "code":
					code = e
				}
				return true
			})
			if link == nil || code == nil {
				t.Fatal("want a link and a code span")
			}
			if got := c.source[code.Start:code.End]; got != c.code {
				t.Errorf("code extent = %q, want %q", got, c.code)
			}
			if code.Start < link.Start || code.End > link.End {
				t.Errorf("code span %q escapes link %q", c.source[code.Start:code.End], c.source[link.Start:link.End])
			}
		})
	}
}
