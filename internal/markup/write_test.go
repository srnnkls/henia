package markup_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestWriteUnchangedIsVerbatim(t *testing.T) {
	sources := map[string]string{
		"nested.md":                         "# Title\n\nSome  *odd*   spacing\\\nand a hard break.\n\n::::outer{#top .wide flag}\nIntro :badge[new]{tone=info}.\n\n:::inner{x=\"a b\"}\n- one\n-   two\n:::\n\n::leaf[text]{k=v}\n::::\n\n```go\n:::not-a-directive\n```\n",
		"plain.md":                          "Para one\n\n\n\nPara   two\t\n\n> quote\n>   indented\n",
		"empty fence after code.md":         "```markdown\n```\n```\n",
		"empty fences in sequence.md":       "Intro\n\n```\n```\n\n~~~\n~~~\n\n- ```\n  ```\n\n```\n",
		"indented atx heading.md":           " # A clear that cannot remove the override must not report the route as gone.\n+    if peer:\n",
		"indented headings of each kind.md": "   ## Deep\ntext\n\n Setext\n ===\n",
		"links labelled with code spans.md": "See [`peer path`](../peer/SKILL.md#x), [a `b`](y.md), [`c` d](z.md), [``e`f``](v.md) and [*`g` h*](w.md).\n",
	}
	root := filepath.Join("..", "..", "tests", "fixtures")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path)
		sources[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			doc, err := markup.Parse(name, []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(markup.Write(doc)); got != src {
				t.Errorf("got\n%q\nwant\n%q", got, src)
			}
		})
	}
}

func TestWriteCanonicalFenceLengths(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"two levels", ":::a\n:::b\nx\n:::\n:::\n", "::::a\n:::b\nx\n:::\n::::\n"},
		{"three levels with attributes", ":::a{k=1}\n:::b\n:::c\nx\n:::\n:::\n:::\n", ":::::a{k=1}\n::::b\n:::c\nx\n:::\n::::\n:::::\n"},
		{"overlong fence", "::::::a\nx\n::::::\n", ":::a\nx\n:::\n"},
		{"siblings at different depths", ":::a\n:::b\n:::c\nx\n:::\n:::\n:::d\ny\n:::\n:::\n", ":::::a\n::::b\n:::c\nx\n:::\n::::\n:::d\ny\n:::\n:::::\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc, err := markup.Parse("fence.md", []byte(c.src))
			if err != nil {
				t.Fatal(err)
			}
			got := string(markup.Write(doc))
			if got != c.want {
				t.Fatalf("got\n%s\nwant\n%s", got, c.want)
			}
			again, err := markup.Parse("fence.md", []byte(got))
			if err != nil {
				t.Fatal(err)
			}
			if second := string(markup.Write(again)); second != got {
				t.Errorf("rewrite changed output:\n%s\nfrom\n%s", second, got)
			}
		})
	}
}

func parseDoc(t *testing.T, file, src string) *markup.Document {
	t.Helper()
	doc, err := markup.Parse(file, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestWriteSerializesMutatedTree(t *testing.T) {
	cases := []struct {
		name, src string
		mutate    func(t *testing.T, doc *markup.Document)
		want      string
	}{
		{
			name: "delete",
			src:  "one\n\ntwo\n\nthree\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				doc.Children = append(doc.Children[:1], doc.Children[2:]...)
			},
			want: "one\n\nthree\n",
		},
		{
			name: "delete last",
			src:  "one\n\ntwo\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				doc.Children = doc.Children[:1]
			},
			want: "one\n",
		},
		{
			name: "delete inside container",
			src:  ":::box\nkeep\n\ndrop\n:::\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				box := findKind(doc.Children, "directive", 0)
				box.Children = box.Children[:1]
			},
			want: ":::box\nkeep\n:::\n",
		},
		{
			name: "replace with graft",
			src:  "one\n\ntwo\n\nthree\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				other := parseDoc(t, "other.md", "intro\n\ngrafted  *text*\n")
				doc.Children[1] = other.Children[1]
			},
			want: "one\n\ngrafted  *text*\n\nthree\n",
		},
		{
			name: "reorder",
			src:  "one\n\ntwo\n\n\nthree\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				c := doc.Children
				doc.Children = []*markup.Item{c[2], c[0], c[1]}
			},
			want: "three\n\none\n\ntwo\n",
		},
		{
			name: "container attributes",
			src:  ":::box{b=1 a=x}\nbody\n:::\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				box := findKind(doc.Children, "directive", 0)
				box.Attrs["a"] = "two words"
				delete(box.Attrs, "b")
				box.Attrs["id"] = "top"
			},
			want: ":::box{#top a=\"two words\"}\nbody\n:::\n",
		},
		{
			name: "leaf and inline attributes",
			src:  "::leaf[text]{k=v}\n\nSee :ref[doc]{to=a} here.\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				findKind(doc.Children, "directive", 0).Attrs["k"] = "w"
				findKind(doc.Children, "directive", 1).Attrs["to"] = "b"
			},
			want: "::leaf[text]{k=w}\n\nSee :ref[doc]{to=b} here.\n",
		},
		{
			name: "sourceless items",
			src:  "one\n",
			mutate: func(t *testing.T, doc *markup.Document) {
				doc.Children = append(doc.Children,
					&markup.Item{Kind: "directive", Attrs: map[string]string{"name": "note", "tone": "info"}, Children: []*markup.Item{
						{Kind: "heading", Attrs: map[string]string{"level": "2"}, Text: "Title"},
						{Kind: "paragraph", Text: "made"},
					}},
					&markup.Item{Kind: "directive", Attrs: map[string]string{"name": "mark"}})
			},
			want: "one\n\n:::note{tone=info}\n## Title\n\nmade\n:::\n\n::mark\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			c.mutate(t, doc)
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestWriteDocumentBuiltOutsideMarkup(t *testing.T) {
	doc := &markup.Document{Children: []*markup.Item{
		{Kind: "paragraph", Text: "alpha"},
		{Kind: "directive", Attrs: map[string]string{"name": "outer"}, Children: []*markup.Item{
			{Kind: "directive", Attrs: map[string]string{"name": "inner"}, Children: []*markup.Item{{Kind: "paragraph", Text: "x"}}},
		}},
	}}
	want := "alpha\n\n::::outer\n:::inner\nx\n:::\n::::\n"
	if got := string(markup.Write(doc)); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestWriteCanonicalFencesInsideSections(t *testing.T) {
	doc := parseDoc(t, "doc.md", ":::a\n# H\n:::b\n:::c\nx\n:::\n:::\n:::\n")
	want := ":::::a\n# H\n::::b\n:::c\nx\n:::\n::::\n:::::\n"
	if got := string(markup.Write(doc)); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestWriteItemsOwnTheirSyntax(t *testing.T) {
	without := func(path ...int) func(t *testing.T, doc *markup.Document) {
		return func(t *testing.T, doc *markup.Document) {
			items := &doc.Children
			for _, i := range path[:len(path)-1] {
				items = &(*items)[i].Children
			}
			last := path[len(path)-1]
			*items = append((*items)[:last:last], (*items)[last+1:]...)
		}
	}
	reorder := func(order ...int) func(t *testing.T, doc *markup.Document) {
		return func(t *testing.T, doc *markup.Document) {
			list := findKind(doc.Children, "list", 0)
			var moved []*markup.Item
			for _, i := range order {
				moved = append(moved, list.Children[i])
			}
			list.Children = moved
		}
	}
	cases := []struct {
		name, src string
		mutate    func(t *testing.T, doc *markup.Document)
		want      string
	}{
		{"bullet delete middle", "- a\n- b\n- c\n", without(0, 1), "- a\n- c\n"},
		{"bullet delete first", "- a\n- b\n- c\n", without(0, 0), "- b\n- c\n"},
		{"bullet reorder", "- a\n- b\n- c\n", reorder(2, 0, 1), "- c\n- a\n- b\n"},
		{"ordered delete middle", "1. a\n2. b\n3. c\n", without(0, 1), "1. a\n3. c\n"},
		{"ordered reorder", "1. a\n2. b\n3. c\n", reorder(2, 0, 1), "3. c\n1. a\n2. b\n"},
		{"loose list reorder", "- a\n\n- b\n", reorder(1, 0), "- b\n\n- a\n"},
		{"rule delete after", "A\n\n---\n\nB\n", without(2), "A\n\n---\n"},
		{"rule delete before", "A\n\n---\n\nB\n", without(0), "---\n\nB\n"},
		{"rule delete itself", "A\n\n---\n\nB\n", without(1), "A\n\nB\n"},
		{"quote delete middle", "> a\n>\n> b\n>\n> c\n", without(0, 1), "> a\n>\n> c\n"},
		{"quote delete first", "> a\n>\n> b\n", without(0, 0), "> b\n"},
		{"quote moved before paragraph", "x\n\n> q\n", reorderTop(1, 0), "> q\n\nx\n"},
		{"list in quote delete first", "> - a\n> - b\n", without(0, 0, 0), "> - b\n"},
		{"fence in quote delete paragraph", "> a\n>\n> ```\n> x\n> ```\n", without(0, 0), "> ```\n> x\n> ```\n"},
		{"indented code delete paragraph", "a\n\n    code\n", without(0), "    code\n"},
		{"html delete neighbour", "a\n\n<div>\nx\n</div>\n", without(0), "<div>\nx\n</div>\n"},
		{"directive in item delete first", "- :::note\n  a\n\n  b\n  :::\n", without(0, 0, 0, 0), "- :::note\n  b\n  :::\n"},
		{"directive in item reorder", "- :::note\n  a\n\n  b\n  :::\n", reorderIn([]int{0, 0, 0}, 1, 0), "- :::note\n  b\n\n  a\n  :::\n"},
		{"directive in item delete last", "- :::note\n  a\n\n  b\n  :::\n", without(0, 0, 0, 1), "- :::note\n  a\n  :::\n"},
		{"directive in quote delete first", "> :::note\n> a\n>\n> b\n> :::\n", without(0, 0, 0), "> :::note\n> b\n> :::\n"},
		{"directive in quote reorder", "> :::note\n> a\n>\n> b\n> :::\n", reorderIn([]int{0, 0}, 1, 0), "> :::note\n> b\n>\n> a\n> :::\n"},
		{"directive in list in quote delete first", "> - :::note\n>   a\n>\n>   b\n>   :::\n", without(0, 0, 0, 0, 0), "> - :::note\n>   b\n>   :::\n"},
		{"directive in list in quote reorder", "> - :::note\n>   a\n>\n>   b\n>   :::\n", reorderIn([]int{0, 0, 0, 0}, 1, 0), "> - :::note\n>   b\n>\n>   a\n>   :::\n"},
		{"directive in directive in item delete first", "- ::::outer\n  :::inner\n  a\n\n  b\n  :::\n  ::::\n", without(0, 0, 0, 0, 0), "- ::::outer\n  :::inner\n  b\n  :::\n  ::::\n"},
		{"list in directive in item delete first", "- :::note\n  - a\n  - b\n  :::\n", without(0, 0, 0, 0, 0), "- :::note\n  - b\n  :::\n"},
		{"quote in item delete first", "- > a\n  >\n  > b\n", without(0, 0, 0, 0), "- > b\n"},
		{"item delete first paragraph", "- a\n\n  b\n", without(0, 0, 0), "- b\n"},
		{"directive in item emptied", "- :::note\n  a\n  :::\n", without(0, 0, 0, 0), "- :::note\n  :::\n"},
		{"closed heading delete neighbour", "# a ##\n\nb\n", without(1), "# a ##\n"},
		{"directive in quote emptied", "> :::note\n> a\n> :::\n", without(0, 0, 0), "> :::note\n> :::\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			c.mutate(t, doc)
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func reorderTop(order ...int) func(t *testing.T, doc *markup.Document) {
	return func(t *testing.T, doc *markup.Document) {
		var moved []*markup.Item
		for _, i := range order {
			moved = append(moved, doc.Children[i])
		}
		doc.Children = moved
	}
}

func reorderIn(path []int, order ...int) func(t *testing.T, doc *markup.Document) {
	return func(t *testing.T, doc *markup.Document) {
		items := &doc.Children
		for _, i := range path {
			items = &(*items)[i].Children
		}
		var moved []*markup.Item
		for _, i := range order {
			moved = append(moved, (*items)[i])
		}
		*items = moved
	}
}

func TestWriteInsertsGeneratedChildren(t *testing.T) {
	appendTo := func(path []int, item *markup.Item) func(t *testing.T, doc *markup.Document) {
		return func(t *testing.T, doc *markup.Document) {
			items := &doc.Children
			for _, i := range path {
				items = &(*items)[i].Children
			}
			*items = append(*items, item)
		}
	}
	paragraph := func(text string) *markup.Item { return &markup.Item{Kind: "paragraph", Text: text} }
	twoParagraphs := &markup.Item{Kind: "item", Children: []*markup.Item{paragraph("b"), paragraph("c")}}
	cases := []struct {
		name, src string
		mutate    func(t *testing.T, doc *markup.Document)
		want      string
	}{
		{"empty container", ":::note\n:::\n", appendTo([]int{0}, paragraph("new")), ":::note\nnew\n:::\n"},
		{"empty container in item", "- :::note\n  :::\n", appendTo([]int{0, 0, 0}, paragraph("new")), "- :::note\n  new\n  :::\n"},
		{"empty container in quote", "> :::note\n> :::\n", appendTo([]int{0, 0}, paragraph("new")), "> :::note\n> new\n> :::\n"},
		{"multi-paragraph item in list in quote", "> - a\n", appendTo([]int{0, 0}, twoParagraphs), "> - a\n> - b\n>\n>   c\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			c.mutate(t, doc)
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestWriteChangedSetextHeadingKeepsUnderline(t *testing.T) {
	retext := func(text string) func(t *testing.T, doc *markup.Document) {
		return func(t *testing.T, doc *markup.Document) { doc.Children[0].Text = text }
	}
	cases := []struct {
		name, src string
		mutate    func(t *testing.T, doc *markup.Document)
		want      string
	}{
		{"level one", "Title :slot[x]\n=====\n\nbody\n", retext("Title X"), "Title X\n=====\n\nbody\n"},
		{"level two", "Title :slot[x]\n-----\n", retext("Title X"), "Title X\n-----\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			c.mutate(t, doc)
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestWriteMovedHTMLKeepsClosingLine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"comment", ":::note\n<!--\nnote\n-->\n:::\n", "<!--\nnote\n-->\n"},
		{"pre", ":::note\n<pre>\nx\n</pre>\n:::\n", "<pre>\nx\n</pre>\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			doc.Children = doc.Children[0].Children
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestWriteInlineChildrenKeepTheirText(t *testing.T) {
	text := func(s string) *markup.Item { return &markup.Item{Kind: "text", Text: s} }
	cases := []struct {
		name, src string
		mutate    func(p *markup.Item)
		want      string
	}{
		{"replace both", "See :a[] and *x* :b[] end.\n", func(p *markup.Item) { p.Children = []*markup.Item{text("A"), text("B")} }, "See A and *x* B end.\n"},
		{"delete", "A :empty[] Z\n", func(p *markup.Item) { p.Children = nil }, "A  Z\n"},
		{"delete first", "A :a[] B :b[] C\n", func(p *markup.Item) { p.Children = p.Children[1:] }, "A  B :b[] C\n"},
		{"delete across lines", "A :a[]\nZ\n", func(p *markup.Item) { p.Children = nil }, "A \nZ\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			c.mutate(doc.Children[0])
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}

func TestWriteReplacementsTakeTheirAnchorsPosition(t *testing.T) {
	text := func(s string) *markup.Item { return &markup.Item{Kind: "text", Text: s} }
	paragraph := func(s string) *markup.Item { return &markup.Item{Kind: "paragraph", Text: s} }
	cases := []struct {
		name, src string
		mutate    func(doc *markup.Document)
		want      string
	}{
		{"inline delete then replace", "A :a[] B :b[] C\n", func(doc *markup.Document) {
			p := doc.Children[0]
			p.Children = p.Children[1].Replace(text("X"))
		}, "A  B X C\n"},
		{"block delete then replace", "a\n\nb\n\n\n\nc\n", func(doc *markup.Document) {
			doc.Children = append([]*markup.Item{doc.Children[1]}, doc.Children[2].Replace(paragraph("Z"))...)
		}, "b\n\n\n\nZ\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := parseDoc(t, "doc.md", c.src)
			c.mutate(doc)
			if got := string(markup.Write(doc)); got != c.want {
				t.Errorf("got\n%q\nwant\n%q", got, c.want)
			}
		})
	}
}
