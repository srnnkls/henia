# Henia Template Tests

## Fixture

`proj` starts a fresh project with one harness; `skill` writes a skill into
it; `out` builds the project and prints one built skill, or the build error.

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P)
> henia() { env HENIA_HARNESS=none XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" XDG_CONFIG_HOME="$T/config" henia "$@"; }
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> proj() { P="$T/$1"; mkdir -p "$P" && git -C "$P" init -q && cd "$P" && printf 'artifacts = ["skills"]\n\n[harness.md]\nartifacts = ["skills"]\n' > henia.toml; }
> skill() { mk "skills/$1/SKILL.md" "---\nname: $1\ndescription: The $1 skill.\n$2---\n$3"; }
> out() { henia build --output "$T/build/$(basename "$P")" > /dev/null 2> "$T/err" && sed '1,/^---$/d' "$T/build/$(basename "$P")/md/skills/$1/SKILL.md" | sed '1{/^$/d;}' || sed "s|$P/||g" "$T/err"; }
> lint() { henia lint "$@" 2>&1 | sed "s|$P/||g; s|$T/config/|config/|g"; }
> mk "$T/config/henia/templates/head.md.tmpl" 'Global context.\n'
> mk "$T/config/henia/templates/footer.md.tmpl" 'Global footer.\n'
> echo staged
staged
```

## Shared templates

A skill includes a template by its file name. A project template in
`.henia/templates/` shadows the global one of the same name, and a global
template without a project counterpart stays available.

```scrut
$ proj include && mk .henia/templates/head.md.tmpl 'Project context: {{.description}}\n'
> skill notes '' '{{template "head" .}}\n# Notes\n\n{{template "footer" .}}'
> out notes
Project context: The notes skill.

# Notes

Global footer.
```

A skill overrides a block the template declares; without an override the
block's default renders.

```scrut
$ proj blocks && mk .henia/templates/head.md.tmpl '## Context\n\n{{block "context" .}}No context.\n{{end}}'
> skill plain '' '{{template "head" .}}'
> skill custom '' '{{define "context"}}Custom context.\n{{end}}{{template "head" .}}'
> out plain && out custom
## Context

No context.
## Context

Custom context.
```

A template that names no shared template and no block of the skill fails the
build.

```scrut
$ proj missing && skill broken '' '{{template "absent" .}}\n'
> out broken
Error: skills/broken: transform broken for md: transform body: execute template: template: content:1:11: executing "content" at <{{template "absent" .}}>: template "absent" not defined
```

## Layouts

`henia.layout` renders the layout with the skill's blocks, in the layout's
order regardless of the order the skill defines them in.

```scrut
$ proj layout && mk .henia/templates/article.md.tmpl '# {{.name}}\n\n{{block "lead" .}}{{required "lead"}}{{end}}\n\n## Procedure\n\n{{block "procedure" .}}{{required "procedure"}}{{end}}\n{{block "notes" .}}{{end}}'
> skill deploy 'henia:\n  layout: article\n' '{{define "procedure"}}1. Run `make release`.{{end}}\n\n{{define "lead"}}Tags and ships a release.{{end}}\n'
> out deploy
# deploy

Tags and ships a release.

## Procedure

1. Run `make release`.
```

Text outside the skill's blocks fails the build.

```scrut
$ proj stray && mk .henia/templates/article.md.tmpl '{{block "lead" .}}{{end}}'
> skill loose 'henia:\n  layout: article\n' 'Loose prose.\n{{define "lead"}}Lead.{{end}}\n'
> out loose
Error: skills/loose: transform loose for md: transform body: layout article: text outside define blocks
```

A block whose default is `required` fails the build when the skill leaves it
out.

```scrut
$ proj required && mk .henia/templates/article.md.tmpl '{{block "lead" .}}{{required "lead"}}{{end}}'
> skill partial 'henia:\n  layout: article\n' '{{define "notes"}}Notes.{{end}}\n'
> out partial
Error: skills/partial: transform partial for md: transform body: execute template: template: article:1:20: executing "lead" at <required "lead">: error calling required: block "lead" is required
```

A layout that is not a shared template fails the build.

```scrut
$ proj unknown && skill orphan 'henia:\n  layout: paper\n' '{{define "lead"}}Lead.{{end}}\n'
> out orphan
Error: skills/orphan: transform orphan for md: transform body: unknown layout "paper"
```

A strict profile accepts `henia.layout` as Henia metadata.

```scrut
$ proj strict && printf 'artifacts = ["skills"]\n\n[harness.claude]\nartifacts = ["skills"]\nprofile = "claude"\nstrict = true\n' > henia.toml
> mk .henia/templates/article.md.tmpl '# {{.name}}\n\n{{block "lead" .}}{{end}}\n'
> skill deploy 'henia:\n  layout: article\n' '{{define "lead"}}Ships.{{end}}\n'
> henia build --output "$T/build/strict" > /dev/null && sed -n '/^# /,$p' "$T/build/strict/claude/skills/deploy/SKILL.md"
# deploy

Ships.
```

## Template directives

A block directive with a `template` attribute renders that shared template in
place of its content. The template sees the skill's values, the directive's
other attributes as `.args` and its rendered content as `.content`; the
directive stays, bare, so `:::head` still marks the hybrid head.

```scrut
$ proj directive && mk .henia/templates/banner.md.tmpl '# {{or .args.title .name}}\n\n{{.content}}\n:contents[]\n'
> skill deploy '' ':::head{template="banner" title="Deploy"}\nShips releases for {{.name}}.\n:::\n\n## Usage\n\nRun it.\n'
> out deploy
# Deploy

Ships releases for deploy.

run first: `henia show deploy --toc`

## Usage

Run it.
```

A directive without a `template` attribute uses the shared template of its own
name. Class attributes arrive as flags, so `:::head{.context}` sets
`.args.context`; with no arguments the template renders the content alone, and
a directive with no template of its name stays as written.

```scrut
$ proj implicit && mk .henia/templates/head.md.tmpl '{{if .args.context}}Context.\n\n{{end}}{{with .args.title}}# {{.}}\n\n{{end}}{{.content}}'
> skill plain '' ':::head\nKept as written.\n:::\n\n:::note\nNo note template.\n:::\n'
> skill flagged '' ':::head{.context title="Deploy"}\nShips releases.\n:::\n'
> skill empty '' ':::head{.context}\n:::\n\nBody.\n'
> out plain && out flagged && out empty
Kept as written.

:::note
No note template.
:::
Context.

# Deploy

Ships releases.
Context.


Body.
```

Without arguments or content the directive renders the template's defaults;
a body of blank lines counts as empty.

```scrut
$ proj directive-empty && mk .henia/templates/banner.md.tmpl '# {{or .args.title .name}}\n{{with .content}}\n{{.}}{{end}}'
> skill bare '' ':::head{template="banner"}\n:::\n\nBody.\n'
> skill blank '' ':::head{template="banner"}\n\n\n:::\n\nBody.\n'
> out bare && out blank
# bare

Body.
# blank

Body.
```

A hybrid head keeps the expanded template and gets the contents command once.

```scrut
$ P="$T/directive" && cd "$P" && printf '\n[harness.md.skills]\nhybrid = ["deploy"]\n' >> henia.toml && out deploy
# Deploy

Ships releases for deploy.

run first: `henia show deploy --toc`

run first: `henia context deploy`
```

A template directive inside another's content expands first, so the outer
template receives the inner one's output.

```scrut
$ proj nested && mk .henia/templates/box.md.tmpl '[{{.args.label}}: {{.content}}]'
> skill boxed '' '::::note{template="box" label="outer"}\n:::tip{template="box" label="inner"}\nCore.\n:::\n::::\n'
> out boxed
::::note
[outer: :::tip
[inner: Core.
]
:::
]
::::
```

A directive naming a template that does not exist fails the build.

```scrut
$ proj directive-missing && skill broken '' ':::head{template="absent"}\nText.\n:::\n'
> out broken
Error: skills/broken: transform broken for md: expand template directives: directive head: execute template: template: content:1:11: executing "content" at <{{template "absent" .}}>: template "absent" not defined
```

Lint resolves directive templates like `{{template}}` calls: an unknown one is
reported, and a template only a directive uses, by name or by its own
directive name, counts as used. A directive without a template of its name is
not a reference.

```scrut
$ proj lint-directive && mk .henia/templates/banner.md.tmpl '{{.content}}'
> mk .henia/templates/note.md.tmpl '> {{.content}}'
> skill deploy '' ':::head{template="banner"}\nLead.\n:::\n\n:::note{template="gone"}\nNote.\n:::\n\n:::note\nImplicit.\n:::\n\n:::warning\nNo template.\n:::\n'
> lint skills --strict
skills/deploy/SKILL.md:9:1: error [unknown-template] template "gone" is neither a shared template nor defined in this file
Error: lint found 1 diagnostic(s)
```

## Runtime

`henia show` renders a library skill with its package's templates and the
global ones, and re-renders when a template changes.

```scrut
$ G="$T/data/henia/packages/global/acme"
> mk "$G/skills/guide/SKILL.md" '---\nname: guide\ndescription: A guide.\nhenia:\n  layout: article\n---\n{{define "lead"}}Read me.{{end}}\n'
> mk "$G/.henia/templates/article.md.tmpl" '# Guide\n\n{{block "lead" .}}{{end}}\n\n{{template "footer" .}}'
> proj runtime && henia show guide
# Guide

Read me.

Global footer.
```

```scrut
$ mk "$G/.henia/templates/article.md.tmpl" '# Revised guide\n\n{{block "lead" .}}{{end}}\n' && henia show guide
# Revised guide

Read me.
```

## Lint

`unknown-template` reports a `{{template}}` call or a layout that resolves to
nothing; a block the skill defines itself resolves.

```scrut
$ proj lint-unknown && mk .henia/templates/head.md.tmpl 'Context.\n'
> skill calls 'henia:\n  layout: paper\n' '{{define "own"}}Own.{{end}}{{define "lead"}}{{template "head" .}}{{template "own" .}}{{template "gone" .}}{{end}}\n'
> lint skills
skills/calls/SKILL.md:5:11: error [unknown-template] template "paper" is neither a shared template nor defined in this file
skills/calls/SKILL.md:7:97: error [unknown-template] template "gone" is neither a shared template nor defined in this file
Error: lint found 2 diagnostic(s)
```

`unused-template` reports a project template that no scanned skill or other
template uses. A template used only by another template, or reached through a
block it defines, counts as used; global templates are never reported.

```scrut
$ proj lint-unused
> mk .henia/templates/article.md.tmpl '{{block "lead" .}}{{end}}{{template "part" .}}'
> mk .henia/templates/part.md.tmpl 'Part.\n'
> mk .henia/templates/hooks.md.tmpl '{{define "hook"}}Hook.{{end}}'
> mk .henia/templates/stale.md.tmpl 'Old.\n'
> skill deploy 'henia:\n  layout: article\n' '{{define "lead"}}{{template "hook" .}}{{end}}\n'
> lint skills --strict
.henia/templates/stale.md.tmpl:1:1: warning [unused-template] template stale is not used by any scanned skill or template
Error: lint found 1 diagnostic(s)
```

A slot a directive's template applies counts as applied: lint renders the
template with the skill's values and the project's harness variables.

```scrut
$ proj lint-partial && printf 'artifacts = ["skills"]\n\n[harness.md]\nartifacts = ["skills"]\n\n[harness.md.variables]\nlabel = "Providers"\n' > henia.toml
> mk .henia/templates/head.md.tmpl '{{with .args.slots}}{{template "slots" $}}{{end}}{{.content}}'
> mk .henia/templates/slots.md.tmpl '{{.label}}:\n\n:slot[{{.args.slots}}]\n\n'
> skill git 'henia:\n  slots:\n    git.commits:\n    git.branching:\n' ':::head{slots="git.commits"}\n# Git\n:::\n'
> lint skills --strict
skills/git/SKILL.md:7:5: warning [unused-slot] slot git.branching is declared but no scanned skill applies it
Error: lint found 1 diagnostic(s)
```

```scrut
$ out git
Providers:

run first: `henia slots --markdown git.commits`

# Git
```

`missing-section` reports a slot offer whose section the skill does not
render. A heading that comes from the skill's layout counts.

```scrut
$ proj lint-sections && mk .henia/templates/article.md.tmpl '# {{.name}}\n\n## Commits\n\n{{block "commits" .}}{{end}}\n'
> skill git 'henia:\n  layout: article\n  slots:\n    git.commits:\n    git.branching:\n  provides:\n    git.commits: {section: commits}\n    git.branching: {section: branching}\n' '{{define "commits"}}:slot[git.commits git.branching]{{end}}\n'
> lint skills
skills/git/SKILL.md:11:30: error [missing-section] slot git.branching offers section #branching, which the skill does not render
Error: lint found 1 diagnostic(s)
```

## Ignore rules

The `.henia/.gitignore` a build writes keeps `templates/` under version control.

```scrut
$ proj ignore && skill plain '' 'Plain.\n' && henia build > /dev/null && grep templates .henia/.gitignore
!/templates/
```
