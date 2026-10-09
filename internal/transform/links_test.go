package transform

import (
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
)

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
				"`worktree` (`henia show 'git.reference.worktree#steps'`), slots (`henia show 'git#slots'`), [up](#routes), " +
				"[site](https://example.test/x), ![img](reference/a.png) and `[code](reference/x.md)`.\n"},
		{"head", Transformer{LibraryLinks: true, Head: true},
			"See `henia show scope.reference.review`, the protocol (`henia show dispatch.protocol`), " +
				"`worktree` (`henia show 'git.reference.worktree#steps'`), slots (`henia show 'git#slots'`), up (`henia show 'scope#routes'`), " +
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

func TestRenderDocument(t *testing.T) {
	body := "# Skill\n\n:::head\n## Routes\n\nSee [review](reference/review.md), [up](#routes) and :term[TTL]{abbr=\"time to live\"}.\n\n:::instruction{priority=high}\nUse `$review` with `!read`.\n:::\n:::\n\n## Detail\n\nMore in [the protocol](../dispatch/protocol.md), `$review` and [site](https://example.test/x).\n\n```md\n:::head\n[x](reference/x.md) `$review`\n:::\n```\n"
	for _, c := range []struct {
		format string
		head   bool
		want   string
	}{
		{"xml", false, "# Skill\n\n## Routes\n\nSee review (`henia show scope.reference.review`), [up](#routes) and <term abbr=\"time to live\">TTL</term>.\n\n<instruction priority=\"high\">\nUse `/skill:review` with `read_file`.\n</instruction>\n\n## Detail\n\nMore in the protocol (`henia show dispatch.protocol`), `/skill:review` and [site](https://example.test/x).\n\n```md\n:::head\n[x](reference/x.md) `$review`\n:::\n```\n"},
		{"directives", false, "# Skill\n\n## Routes\n\nSee review (`henia show scope.reference.review`), [up](#routes) and :term[TTL]{abbr=\"time to live\"}.\n\n:::instruction{priority=\"high\"}\nUse `/skill:review` with `read_file`.\n:::\n\n## Detail\n\nMore in the protocol (`henia show dispatch.protocol`), `/skill:review` and [site](https://example.test/x).\n\n```md\n:::head\n[x](reference/x.md) `$review`\n:::\n```\n"},
		{"markdown", true, "## Routes\n\nSee review (`henia show scope.reference.review`), up (`henia show 'scope#routes'`) and **term abbr=\"time to live\":** TTL.\n\n**instruction priority=\"high\"**\nUse `/skill:review` with `read_file`.\n\n"},
	} {
		t.Run(c.format, func(t *testing.T) {
			tr := Transformer{LibraryLinks: true, Head: c.head, OutputFormat: c.format,
				References: map[string]ReferenceConfig{"skill": {Output: "/skill:{{.Name}}"}},
				Tools:      map[string]string{"read": "read_file"}}
			doc, err := markup.Parse("SKILL.md", []byte(body))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tr.renderDocument(doc, &artifact.Artifact{Name: "scope", Namespace: "ns", Type: artifact.TypeSkill})
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
