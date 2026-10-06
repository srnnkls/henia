package transform

import (
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
)

func renderLinks(tr *Transformer, body, skill string) string {
	tree, _ := markup.Tree([]byte(body))
	edits, _ := tr.linkEdits(tree, body, skill)
	return markup.Apply(body, edits).Text
}

func TestRenderLinks(t *testing.T) {
	body := "See [reference/review.md](reference/review.md), [the protocol](../dispatch/protocol.md), " +
		"[`worktree`](../git/reference/worktree.md#steps), [slots](../git/SKILL.md#slots), [up](#routes), " +
		"[site](https://example.test/x), ![img](reference/a.png) and `[code](reference/x.md)`.\n"
	if got := renderLinks(&Transformer{LibraryLinks: true}, "[dispatch/protocol.md](../dispatch/protocol.md)", "scope"); got != "`henia show dispatch.protocol`" {
		t.Errorf("target as text: %s", got)
	}
	for _, c := range []struct {
		name string
		tr   Transformer
		want string
	}{
		{"library", Transformer{LibraryLinks: true},
			"See `henia show scope.reference.review`, the protocol (`henia show dispatch.protocol`), " +
				"`worktree` (`henia show git.reference.worktree#steps`), slots (`henia show git#slots`), [up](#routes), " +
				"[site](https://example.test/x), ![img](reference/a.png) and `[code](reference/x.md)`.\n"},
		{"head", Transformer{LibraryLinks: true, Head: true},
			"See `henia show scope.reference.review`, the protocol (`henia show dispatch.protocol`), " +
				"`worktree` (`henia show git.reference.worktree#steps`), slots (`henia show git#slots`), up (`henia show scope#routes`), " +
				"[site](https://example.test/x), ![img](reference/a.png) and `[code](reference/x.md)`.\n"},
		{"static", Transformer{Served: map[string]bool{"dispatch": true}},
			"See [reference/review.md](reference/review.md), the protocol (`henia show dispatch.protocol`), " +
				"[`worktree`](../git/reference/worktree.md#steps), [slots](../git/SKILL.md#slots), [up](#routes), " +
				"[site](https://example.test/x), ![img](reference/a.png) and `[code](reference/x.md)`.\n"},
	} {
		if got := renderLinks(&c.tr, body, "scope"); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestLibraryLinkProvenance(t *testing.T) {
	art := &artifact.Artifact{Name: "scope", Type: artifact.TypeSkill, Body: "See [the protocol](../dispatch/protocol.md#menu) and [git/SKILL.md](../git/SKILL.md).\n"}
	got, refs, err := (&Transformer{LibraryLinks: true}).TransformReferences(art)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"henia show dispatch.protocol#menu": "dispatch|protocol|menu", "henia show git": "git||"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %+v", refs)
	}
	for _, ref := range refs {
		if !ref.Address || got.Body[ref.Start] != '`' || got.Body[ref.Start+1:ref.End] != ref.Raw || want[ref.Raw] != ref.Name+"|"+ref.Module+"|"+ref.Anchor {
			t.Errorf("%+v spans %q in %q", ref, got.Body[ref.Start:ref.End], got.Body)
		}
	}
}
