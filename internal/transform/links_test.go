package transform

import "testing"

func TestRenderLinks(t *testing.T) {
	body := "See [reference/review.md](reference/review.md), [the protocol](../dispatch/protocol.md), " +
		"[`worktree`](../git/reference/worktree.md#steps), [slots](../git/SKILL.md#slots), [up](#routes), " +
		"[site](https://example.test/x), ![img](reference/a.png) and `[code](reference/x.md)`.\n"
	if got := (&Transformer{LibraryLinks: true}).renderLinks("[dispatch/protocol.md](../dispatch/protocol.md)", "scope"); got != "`henia show dispatch.protocol`" {
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
		if got := c.tr.renderLinks(body, "scope"); got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, c.want)
		}
	}
}
