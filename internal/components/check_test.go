package components

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const noteComponent = `---
form: container
attributes:
  title:
    type: string
    required: true
  tone:
    type: enum
    values: [info, warn]
body:
  kind: markdown
  children: [paragraph]
  required: true
---
{{.title}}

::body
`

const badgeComponent = `---
form: text
attributes:
  count:
    type: number
body:
  kind: text
---
{{.count}}
`

const tagsComponent = `---
form: leaf
attributes:
  slots:
    type: list
    of: slot
    matches: '[a-z]+(\.[a-z]+)*'
body:
  kind: none
---
{{.slots}}
`

func declare(t *testing.T, components map[string]string) (map[string]Declaration, string) {
	t.Helper()
	project := t.TempDir()
	dir := filepath.Join(project, ".henia", Dir)
	for name, content := range components {
		write(t, filepath.Join(dir, name+".md"), content)
	}
	decls, problems := Load(project, "")
	if len(problems) > 0 {
		t.Fatalf("Load problems = %+v", problems)
	}
	return decls, dir
}

func located(diags []Diagnostic, keep func(Diagnostic) bool) []string {
	var got []string
	for _, d := range diags {
		if keep(d) {
			got = append(got, fmt.Sprintf("%s %s %s:%d:%d", d.ID, d.Level, d.Path, d.Line, d.Column))
		}
	}
	slices.Sort(got)
	return got
}

func TestCheckCalls(t *testing.T) {
	decls, _ := declare(t, map[string]string{"note": noteComponent, "badge": badgeComponent, "tags": tagsComponent})
	const skill = "/pkg/skills/demo/SKILL.md"
	cases := []struct {
		name, source string
		want         []string
		mention      string
	}{
		{"unknown component", "intro\n\n:::rotues\ntext\n:::\n", []string{"unknown-component error " + skill + ":3:1"}, "rotues"},
		{"missing attribute", "intro\n\n:::note\ntext\n:::\n", []string{"missing-attribute error " + skill + ":3:1"}, "title"},
		{"unknown attribute", "intro\n\n:::note{title=a bogus=1}\ntext\n:::\n", []string{"unknown-attribute error " + skill + ":3:1"}, "bogus"},
		{"id on a component without id", "intro\n\n:::note{title=a #main}\ntext\n:::\n", []string{"unknown-attribute error " + skill + ":3:1"}, "id"},
		{"class on a component without class", "intro\n\n:::note{title=a .wide}\ntext\n:::\n", []string{"unknown-attribute error " + skill + ":3:1"}, "class"},
		{"enum outside its values", "intro\n\n:::note{title=a tone=loud}\ntext\n:::\n", []string{"invalid-attribute error " + skill + ":3:1"}, "loud"},
		{"number that is not one", "See :badge[x]{count=many} now.\n", []string{"invalid-attribute error " + skill + ":1:5"}, "many"},
		{"list item breaking matches", "intro\n\n::tags{slots=\"git.commits Nope\"}\n", []string{"invalid-attribute error " + skill + ":3:1"}, "Nope"},
		{"list items pass through to later resolution", "intro\n\n::tags{slots=\"git.commits nope\"}\n", nil, ""},
		{"required body left empty", "intro\n\n:::note{title=a}\n:::\n", []string{"invalid-body error " + skill + ":3:1"}, ""},
		{"child outside the allow-list", "intro\n\n:::note{title=a}\n```\nx\n```\n:::\n", []string{"disallowed-child error " + skill + ":4:1"}, "code"},
		{"container called as leaf", "intro\n\n::note{title=a}\n", []string{"wrong-form error " + skill + ":3:1"}, "leaf"},
		{"template action in a skill body", "Some {{.x}} text\n", []string{"template-action error " + skill + ":1:6"}, ""},
		{"valid call", "intro\n\n:::note{title=a tone=warn}\ntext\n:::\n", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := Check([]Document{{Path: skill, Source: []byte(c.source)}}, decls, nil)
			inSkill := func(d Diagnostic) bool { return d.Path == skill }
			if got := located(diags, inSkill); !slices.Equal(got, c.want) {
				t.Fatalf("diagnostics = %q, want %q", got, c.want)
			}
			for _, d := range diags {
				if inSkill(d) && !strings.Contains(d.Message, c.mention) {
					t.Errorf("%s message %q does not name %q", d.ID, d.Message, c.mention)
				}
			}
		})
	}
}

func TestCheckTemplates(t *testing.T) {
	rows := `---
form: leaf
attributes:
  items:
    type: list
  title:
    type: string
body:
  kind: none
---
{{range .items}}{{.label}}{{end}}
{{with .title}}{{.whatever}}{{end}}
{{.items}} {{.description}} {{.title}}
x {{$.nope}}
{{.missing}}
`
	broken := `---
form: leaf
body:
  kind: none
---
ok
{{end}}
`
	decls, dir := declare(t, map[string]string{"rows": rows, "broken": broken})
	rowsPath, brokenPath := filepath.Join(dir, "rows.md"), filepath.Join(dir, "broken.md")

	diags := Check(nil, decls, map[string]bool{"description": true})

	got := located(diags, func(d Diagnostic) bool { return d.ID != "unused-component" })
	want := []string{
		fmt.Sprintf("invalid-template error %s:7:1", brokenPath),
		fmt.Sprintf("unknown-field error %s:14:5", rowsPath),
		fmt.Sprintf("unknown-field error %s:15:3", rowsPath),
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("diagnostics = %q, want %q", got, want)
	}
	for _, d := range diags {
		if d.ID == "unknown-field" && !strings.Contains(d.Message, "nope") && !strings.Contains(d.Message, "missing") {
			t.Errorf("unknown-field message %q names neither nope nor missing", d.Message)
		}
	}
}

func TestCheckReportsUnusedComponents(t *testing.T) {
	decls, dir := declare(t, map[string]string{"note": noteComponent, "tags": tagsComponent})
	skill := Document{Path: "/pkg/skills/demo/SKILL.md", Source: []byte(":::note{title=a}\ntext\n:::\n")}

	diags := Check([]Document{skill}, decls, nil)

	got := located(diags, func(Diagnostic) bool { return true })
	want := []string{fmt.Sprintf("unused-component warning %s:0:0", filepath.Join(dir, "tags.md"))}
	if !slices.Equal(got, want) {
		t.Fatalf("diagnostics = %q, want %q", got, want)
	}
}

const boxComponent = `---
form: container
body:
  kind: markdown
  required: true
---
::body
`

const ruleComponent = `---
form: container
body:
  kind: none
---
rule
`

func TestCheckBodies(t *testing.T) {
	decls, _ := declare(t, map[string]string{"box": boxComponent, "bare": ruleComponent})
	const skill = "/pkg/skills/demo/SKILL.md"
	cases := []struct {
		name, source string
		want         []string
	}{
		{"inline code holds template text", "Write `{{.name}}` in a component.\n", nil},
		{"text body leaves later calls checked", ":slot[code.validation]\n\n:::rotues\ntext\n:::\n", []string{"unknown-component error " + skill + ":3:1"}},
		{"thematic break fills a required body", ":::box\n---\n:::\n", nil},
		{"thematic break breaks an empty body", ":::bare\n---\n:::\n", []string{"invalid-body error " + skill + ":1:1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := Check([]Document{{Path: skill, Source: []byte(c.source)}}, decls, nil)
			if got := located(diags, func(d Diagnostic) bool { return d.Path == skill }); !slices.Equal(got, c.want) {
				t.Fatalf("diagnostics = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCheckAssociatedTemplates(t *testing.T) {
	cases := map[string]string{
		"define": "{{define \"row\"}}{{$.nope}}{{end}}{{template \"row\" .}}\n",
		"block":  "{{block \"row\" .}}{{$.nope}}{{end}}\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			decls, dir := declare(t, map[string]string{"part": "---\nform: leaf\nbody:\n  kind: none\n---\n" + text})

			diags := Check(nil, decls, nil)

			got := located(diags, func(d Diagnostic) bool { return d.ID == UnknownField })
			want := []string{fmt.Sprintf("unknown-field error %s:6:%d", filepath.Join(dir, "part.md"), strings.Index(text, "$.nope")+1)}
			if !slices.Equal(got, want) {
				t.Fatalf("diagnostics = %q, want %q", got, want)
			}
		})
	}
}
