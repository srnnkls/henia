package components

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

const routesComponent = `---
form: container
doc: Routes the skill's arguments.
attributes:
  ordered:
    type: boolean
    doc: Rows apply in order; the first match wins.
  protocol:
    type: boolean
    doc: Links the dispatch protocol.
body:
  kind: markdown
  children: [table, paragraph]
---
## Routes

{{if .ordered}}Apply the first row that matches ` + "`$ARGUMENTS`" + `:

{{end}}::body
{{if .protocol}}
> **Protocol:** [../dispatch/protocol.md](../dispatch/protocol.md)
{{end}}
`

const titleComponent = `---
form: leaf
attributes:
  title:
    type: string
body:
  kind: none
---
# {{.title}}
`

const wrapperComponent = `---
form: leaf
body:
  kind: none
---
::include{component="title" title="X"}
`

const contextComponent = `---
form: leaf
body:
  kind: none
---
before{{range .context_commands}}[{{.}}]{{end}}|{{.context_commands}}|after
`

const headComponent = `---
form: container
attributes:
  title:
    type: string
body:
  kind: markdown
---
# {{.title}}
`

const skillPath = "/pkg/skills/demo/SKILL.md"

func environment(target string) Environment {
	return Environment{
		Target: target,
		Harnesses: map[string]map[string]string{
			"claude": {"preload_context": "true"},
			"codex":  {"preload_context": "false"},
		},
		Values: map[string]any{},
		Known:  map[string]bool{"context_commands": true},
	}
}

func expand(t *testing.T, decls map[string]Declaration, target, source string) (*markup.Document, string, []Diagnostic) {
	t.Helper()
	doc, err := markup.Parse(skillPath, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	diags := ExpandDocument(doc, decls, environment(target))
	return doc, string(markup.Write(doc)), diags
}

func TestExpandDocument(t *testing.T) {
	decls, _ := declare(t, map[string]string{
		"routes":  routesComponent,
		"title":   titleComponent,
		"wrapper": wrapperComponent,
		"context": contextComponent,
		"head":    headComponent,
	})
	table := "| Arg | Route |\n|---|---|\n| a | b |\n"
	cases := []struct {
		name, target, source string
		contains, absent     []string
	}{
		{
			name:     "routes grafts its body",
			target:   "claude",
			source:   "intro\n\n:::routes{ordered}\n" + table + ":::\n",
			contains: []string{"intro\n", "## Routes\n", "Apply the first row that matches `$ARGUMENTS`:\n", table},
			absent:   []string{"Protocol", "protocol.md", ":::", "::body"},
		},
		{
			name:     "generated include resolves in the next round",
			target:   "claude",
			source:   "::wrapper\n",
			contains: []string{"# X\n"},
			absent:   []string{"::include", "::wrapper"},
		},
		{
			name:     "absent skill value is nil and prints blank",
			target:   "claude",
			source:   "::context\n",
			contains: []string{"before||after\n"},
			absent:   []string{"<no value>", "::context", "["},
		},
		{
			name:     "when keeps body for listed target",
			target:   "claude",
			source:   ":::when{target=\"claude\"}\nkept\n:::\n\n:::when{preload_context=true}\npreloaded\n:::\n\n:::unless{target=\"claude\"}\nother\n:::\n",
			contains: []string{"kept\n", "preloaded\n"},
			absent:   []string{"other", ":::", "when", "unless"},
		},
		{
			name:     "when drops body for other target",
			target:   "codex",
			source:   ":::when{target=\"claude\"}\nkept\n:::\n\n:::when{preload_context=true}\npreloaded\n:::\n\n:::unless{target=\"claude\"}\nother\n:::\n",
			contains: []string{"other\n"},
			absent:   []string{"kept", "preloaded", ":::", "when", "unless"},
		},
		{
			name:     "head output stays inside head fences",
			target:   "claude",
			source:   ":::head{title=\"X\"}\nold\n:::\n\nrest\n",
			contains: []string{":::head{title=\"X\"}\n# X\n:::\n\nrest\n"},
			absent:   []string{"old"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got, diags := expand(t, decls, c.target, c.source)
			if len(diags) > 0 {
				t.Fatalf("diagnostics = %+v", diags)
			}
			for _, want := range c.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output lacks %q:\n%s", want, got)
				}
			}
			for _, unwanted := range c.absent {
				if strings.Contains(got, unwanted) {
					t.Errorf("output holds %q:\n%s", unwanted, got)
				}
			}
		})
	}

	t.Run("grafted body keeps its skill positions", func(t *testing.T) {
		doc, _, diags := expand(t, decls, "claude", "intro\n\n:::routes{ordered}\n"+table+":::\n")
		if len(diags) > 0 {
			t.Fatalf("diagnostics = %+v", diags)
		}
		var tables []*markup.Item
		var visit func([]*markup.Item)
		visit = func(items []*markup.Item) {
			for _, it := range items {
				if it.Kind == "table" {
					tables = append(tables, it)
				}
				visit(it.Children)
			}
		}
		visit(doc.Children)
		if len(tables) != 1 {
			t.Fatalf("tables = %d, want 1", len(tables))
		}
		if o := tables[0].Origin; o.File != skillPath || o.Line != 4 || o.Column != 1 {
			t.Errorf("table origin = %+v, want %s:4:1", o, skillPath)
		}
	})
}

func TestExpandDiagnostics(t *testing.T) {
	bad := "---\nform: leaf\nbody:\n  kind: none\n---\n::include{component=\"title\" title=\"X\" nope=1}\n"
	loop := "---\nform: leaf\nbody:\n  kind: none\n---\n::loop\n"
	ping := "---\nform: leaf\nbody:\n  kind: none\n---\n::pong\n"
	pong := "---\nform: leaf\nbody:\n  kind: none\n---\n::ping\n"
	decls, dir := declare(t, map[string]string{"title": titleComponent, "bad": bad, "loop": loop, "ping": ping, "pong": pong})
	cases := []struct {
		name, source string
		want         string
		note         string
	}{
		{"unknown target", "intro\n\n:::when{target=\"clod\"}\nx\n:::\n", "unknown-target error " + skillPath + ":3:1", ""},
		{"unknown variable", ":::when{nope=true}\nx\n:::\n", "unknown-variable error " + skillPath + ":1:1", ""},
		{"generated call points at the skill", "intro\n\n::bad\n", "unknown-attribute error " + skillPath + ":3:1", filepath.Join(dir, "bad.md") + ":6"},
		{"direct component cycle", "::loop\n", "component-cycle error " + skillPath + ":1:1", ""},
		{"indirect component cycle", "intro\n\n::ping\n", "component-cycle error " + skillPath + ":3:1", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, diags := expand(t, decls, "claude", c.source)
			got := located(diags, func(Diagnostic) bool { return true })
			if !slices.Contains(got, c.want) {
				t.Fatalf("diagnostics = %q, want %q among them", got, c.want)
			}
			if c.note == "" {
				return
			}
			for _, d := range diags {
				if fmt.Sprintf("%s %s %s:%d:%d", d.ID, d.Level, d.Path, d.Line, d.Column) != c.want {
					continue
				}
				if !slices.ContainsFunc(d.Notes, func(n string) bool { return strings.Contains(n, c.note) }) {
					t.Errorf("notes = %q, want one naming %q", d.Notes, c.note)
				}
			}
		})
	}
}

func TestExpandKeepsInlineContent(t *testing.T) {
	text := func(template string) string {
		return "---\nform: text\nbody:\n  kind: text\n---\n" + template
	}
	decls, _ := declare(t, map[string]string{
		"strong": text("**{{.body}}**\n"),
		"empty":  text(""),
		"inner":  "---\nform: text\nbody:\n  kind: none\n---\nX\n",
		"wrap":   "---\nform: leaf\nbody:\n  kind: markdown\n---\n::body\n",
	})
	cases := []struct{ name, source, want string }{
		{"replaced siblings keep the text between them", "See :strong[a] and *x* :strong[b] end.\n", "See **a** and *x* **b** end.\n"},
		{"removed child keeps the text before it", "A :empty[] Z\n", "A  Z\n"},
		{"label body keeps its expanded children", "::wrap[before :inner[] after]\n", "before X after\n"},
		{"replacement after a removed child keeps its position", "A :empty[] B :strong[c] C :empty[] D\n", "A  B **c** C  D\n"},
		{"replacement after a removed child in a quote keeps its position", "> A :empty[]\n> Z :strong[q] w\n", "> A \n> Z **q** w\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got, diags := expand(t, decls, "claude", c.source)
			if len(diags) > 0 {
				t.Fatalf("diagnostics = %+v", diags)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestExpandValidatesFinalChildren(t *testing.T) {
	container := func(render string) string {
		return "---\nform: container\nbody:\n  kind: markdown\n  children: [bridge, paragraph]\n" + render + "---\n::body\n"
	}
	leaf := func(template string) string { return "---\nform: leaf\nbody:\n  kind: none\n---\n" + template }
	t.Run("kept container reports a disallowed final child", func(t *testing.T) {
		decls, _ := declare(t, map[string]string{"box": container("render: keep\n"), "bridge": leaf("::next\n"), "next": leaf("# Heading\n")})
		_, _, diags := expand(t, decls, "claude", ":::box\n::bridge\n:::\n")
		got := located(diags, func(d Diagnostic) bool { return d.ID == DisallowedChild })
		if want := []string{"disallowed-child error " + skillPath + ":2:1"}; !slices.Equal(got, want) {
			t.Errorf("diagnostics = %q, want %q", got, want)
		}
	})
	t.Run("expanding container accepts an allowed final child", func(t *testing.T) {
		decls, _ := declare(t, map[string]string{"box": container(""), "bridge": leaf("::next\n"), "next": leaf("done\n")})
		_, got, diags := expand(t, decls, "claude", ":::box\n::bridge\n:::\n")
		if len(diags) > 0 {
			t.Fatalf("diagnostics = %+v", diags)
		}
		if got != "done\n" {
			t.Errorf("got %q, want %q", got, "done\n")
		}
	})
}
