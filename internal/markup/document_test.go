package markup_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func findKind(items []*markup.Item, kind string, nth int) *markup.Item {
	var found *markup.Item
	var visit func([]*markup.Item)
	visit = func(items []*markup.Item) {
		for _, it := range items {
			if found != nil {
				return
			}
			if it.Kind == kind {
				if nth == 0 {
					found = it
					return
				}
				nth--
			}
			visit(it.Children)
		}
	}
	visit(items)
	return found
}

func TestGraftedSubtreeKeepsOriginAndBacktrace(t *testing.T) {
	host, err := markup.Parse("skill.md", []byte("# Skill\n\nalpha\n\n::card{x=1}\n"))
	if err != nil {
		t.Fatal(err)
	}
	componentSource := "intro\n\nbeta  *keep*\\\n   odd   text\n"
	component, err := markup.Parse("components/card.md", []byte(componentSource))
	if err != nil {
		t.Fatal(err)
	}

	call := findKind(host.Children, "directive", 0)
	generated := findKind(component.Children, "paragraph", 1)
	if call == nil || generated == nil {
		t.Fatalf("call %v generated %v", call, generated)
	}
	if call.Origin.File != "skill.md" || call.Origin.Line != 5 || call.Origin.Column != 1 || call.Origin.Start != 16 {
		t.Errorf("call origin %+v, want skill.md 5:1 at byte 16", call.Origin)
	}
	want := markup.Origin{File: "components/card.md", Start: 7, End: 7 + len("beta  *keep*\\\n   odd   text"), Line: 3, Column: 1}
	if generated.Origin != want {
		t.Errorf("generated origin %+v, want %+v", generated.Origin, want)
	}

	generated.Backtrace = append(generated.Backtrace, markup.Call{Directive: call.Origin, Component: generated.Origin})
	host.Children = append(host.Children, generated)

	out := string(markup.Write(host))
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "# Skill") {
		t.Errorf("host content lost:\n%s", out)
	}
	if !strings.Contains(out, "beta  *keep*\\\n   odd   text") {
		t.Errorf("grafted node not written from its own origin span:\n%q", out)
	}
	if generated.Origin.File != "components/card.md" || len(generated.Backtrace) != 1 || generated.Backtrace[0].Directive.File != "skill.md" {
		t.Errorf("graft lost origin or backtrace: %+v %+v", generated.Origin, generated.Backtrace)
	}
}

func TestParseOwnsItsSource(t *testing.T) {
	src := []byte("alpha\n\n::card{x=1}\n")
	doc, err := markup.Parse("skill.md", src)
	if err != nil {
		t.Fatal(err)
	}
	copy(src, "omega")
	if got, want := string(markup.Write(doc)), "alpha\n\n::card{x=1}\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseAgreesWithTreeOnBlocks(t *testing.T) {
	src := "A\n\n---\n\n- a\n\n> q\n\n<div>\nx\n</div>\n\n```\nc\n```\n"
	tree, err := markup.Tree([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	doc := parseDoc(t, "doc.md", src)
	if len(doc.Children) != len(tree.Children) {
		t.Fatalf("parse has %d blocks, tree %d", len(doc.Children), len(tree.Children))
	}
	for i, e := range tree.Children {
		if doc.Children[i].Kind != e.Type {
			t.Errorf("block %d: parse %q, tree %q", i, doc.Children[i].Kind, e.Type)
		}
	}
}

func TestParseUnclosedEmptyDirectiveInItem(t *testing.T) {
	src := "- :::note\nb\n  :::\n"
	doc, err := markup.Parse("doc.md", []byte(src))
	var located *markup.Error
	if !errors.As(err, &located) || !strings.Contains(located.Error(), "unclosed-directive") {
		t.Fatalf("err = %v, want unclosed-directive", err)
	}
	if got := string(markup.Write(doc)); got != src {
		t.Errorf("got %q, want %q", got, src)
	}
}

func TestParseWritesEdgeInputsVerbatim(t *testing.T) {
	for _, src := range []string{"- :::note\nb\n  :::\n", ">#\n000000000000", "#\n", "# a ##\n\nb\n"} {
		t.Run(src, func(t *testing.T) {
			doc, _ := markup.Parse("doc.md", []byte(src))
			if got := string(markup.Write(doc)); got != src {
				t.Errorf("got %q, want %q", got, src)
			}
		})
	}
}
