# Components

A *component* is a directive a skill calls to pull in shared structure. Each
one is declared once in `.henia/components/<name>.md`: the frontmatter is its
schema, the body its template. Every directive in a skill must be declared, so
a typo is an error, not text that silently passes through.

````sh
git init ops && cd ops
mkdir -p .henia/components skills/deploy skills/git
cat > .henia/components/routes.md <<'EOF'
---
form: container
doc: Routes the skill's arguments.
attributes:
  ordered:
    type: boolean
    doc: Rows apply in order; the first match wins.
body:
  kind: markdown
  children: [table]
---
## Routes

{{if .ordered}}Apply the first row that matches `$ARGUMENTS`.

{{end}}::body
EOF
cat > .henia/components/run.md <<'EOF'
---
form: leaf
doc: Shows a shell command.
body:
  kind: text
  required: true
---
```sh
{{.body}}
```
EOF
cat > .henia/components/instruction.md <<'EOF'
---
form: container
doc: An instruction block the harness renders.
attributes:
  priority:
    type: enum
    values: [high, low]
render: keep
---
EOF
cat > skills/git/SKILL.md <<'EOF'
---
name: git
description: Branch, commit and tag.
---

# Git

## Tagging

Sign every tag.
EOF
cat > skills/deploy/SKILL.md <<'EOF'
---
name: deploy
description: Ship a release to production.
henia:
  variables:
    branch: main
---

# Deploy

:::routes{ordered}
| Argument | Route |
|---|---|
| hotfix | Release from the hotfix branch. |
| none | Release from {{.branch}}. |
:::

::run[make release]

:::instruction{priority=high}
Never deploy on a Friday.
:::

:::when{target="claude"}
Ask before pushing the tag.
:::

::include{ref="git#tagging"}
EOF
cat > henia.toml <<'EOF'
[harness.claude]
directives = "xml"

[harness.codex]
EOF
````

````console
$ henia build
Built 4 artifact(s) in .henia/build
$ sed -n '/^# /,$p' .henia/build/claude/skills/deploy/SKILL.md
# Deploy

## Routes

Apply the first row that matches `$ARGUMENTS`.

| Argument | Route |
|---|---|
| hotfix | Release from the hotfix branch. |
| none | Release from main. |

```sh
make release
```

<instruction priority="high">
Never deploy on a Friday.
</instruction>

Ask before pushing the tag.

## Tagging

Sign every tag.
$ sed -n '/^:::instruction/,/^## Tagging/p' .henia/build/codex/skills/deploy/SKILL.md
:::instruction{priority="high"}
Never deploy on a Friday.
:::

## Tagging
````

`routes` wraps the table, `run` places its label in a code block, and
`instruction` stays a directive that each harness renders through its
[`directives`](build.md#directives) setting. The `when` block and the section
of `git` are built-in components.

## Calling

A directive takes one of three forms:

| Form | Syntax | Body |
|---|---|---|
| text | `:name[label]{attrs}` | the label, inline |
| leaf | `::name[label]{attrs}` | the label, on its own line |
| container | `:::name{attrs}` … `:::` | the blocks between the fences |

Attributes are `key=value`, `key="quoted value"`, a bare `key`, `#id` and
`.class`. A bare key is boolean true: `:::routes{ordered}` and
`:::routes{ordered=true}` are the same call. `#id` and `.class` are the
attributes `id` and `class`; a component accepts them only if it declares
them, `class` as a list.

A repeated key is `duplicate-attribute` and a container without its closing
fence is `unclosed-directive`; neither is resolved by guessing. Every error in
a document is reported, each at its line and column.

A closing fence closes the innermost open container, so `:::` works at every
depth. [`henia fmt`](#formatting) lengthens the outer fences so other
directive parsers read the same nesting.

## Declaring

Henia reads components from `.henia/components/<name>.md` in the project,
which shadows `$XDG_CONFIG_HOME/henia/components/<name>.md`. A skill package
dependency resolves its skills against its own `.henia/components/` only.

| Field | Values |
|---|---|
| `form` | `container`, `leaf` or `text`; required |
| `doc` | a description |
| `attributes.<name>.type` | `string`, `boolean`, `number`, `list`, `enum`, `slot`, `skill`, `ref` |
| `attributes.<name>.of` | a `list`'s element type: `string`, `slot`, `skill`, `ref` |
| `attributes.<name>` | also `required`, `default`, `values` (an `enum`'s choices), `matches` (a regexp the whole value must match), `doc` |
| `body.kind` | `markdown` (default), `text` or `none` |
| `body.children` | the node types a `markdown` body may hold; directives by name |
| `body.required` | the call must have a body |
| `render` | `expand` (default), or `keep` |

A `list` is whitespace-separated: `slots="git.commits review.criteria"`. A
`slot` names a slot some skill in the package declares, a `skill` a skill name,
and a `ref` an address `henia show` reads, such as `git#tagging`. A value that
does not fit its type is `invalid-attribute`, an undeclared attribute
`unknown-attribute`, a missing `required` one `missing-attribute`.

The template is Go `text/template`. Its data holds every declared attribute,
coerced to its type, and the calling skill's values: frontmatter,
`henia.variables` and the harness's variables, the attributes winning. An
omitted attribute holds its `default`, or its type's zero value: `false`, `0`,
an empty string or an empty list. A value another skill in the package or
another harness defines, but the caller does not, is absent: it prints
nothing, and `if`, `with` and `range` skip it. A field that is neither an
attribute nor a known value is `unknown-field`, found by parsing the template,
even in a branch that never runs.

A declaration whose schema does not parse, or that takes a built-in's name,
is `invalid-component`. A project component nothing calls is
`unused-component`, a warning.

## Bodies

`body.kind` says what a call's body is and how the template places it:

- `markdown`: a tree. The template places it with a `::body` line, once; the
  call's blocks replace that line and keep their positions in the skill. A
  component that receives a body and has no `::body` is `invalid-body`.
- `text`: a string in `.body`, so the template can put it anywhere, inside a
  code block too, as `run` does. Henia never scans it for directives.
- `none`: the call takes no body.

`body.children` lists what a `markdown` body may hold: `paragraph`, `heading`,
`list`, `table`, `code`, `quote`, `html`, `rule` or a directive's name.
Anything else is `disallowed-child`.

A text component's template must produce inline Markdown, and a leaf or
container component's blocks. Output in the wrong form is `wrong-form`, as is
calling a component in a form it does not declare.

## Keeping a directive

`render: keep` declares a directive that Henia checks and the harness renders.
Its attributes and body are validated like any call, and the directive stays
in the output for the harness's [`directives`](build.md#directives) setting to
render. It needs no template.

## Built-in components

| Component | Form | Purpose |
|---|---|---|
| `::include{ref="git#tagging"}` | leaf | Grafts another document, or one of its sections |
| `::include{component="footer"}` | leaf | Renders a component with body kind `none`, its other attributes as that component's |
| `:::when{…}`, `:::unless{…}` | container | Keeps or drops the body per harness |
| `:::head` | container | Marks the hybrid head |
| `:related[skill]`, `:::related` | text, container | Related skills; a `## Related` section |
| `:slot[…]`, `:contents[skill]` | text | Slot providers and a skill's sections; see [slots](slots.md) and [extra files](build.md#extra-files) |
| `::body` | leaf | Where a component places its `markdown` body |

A project or user component named `head` supplies the template of the built-in
`head`. Every other built-in name is reserved.

### include

`include` takes exactly one of `ref` and `component`; neither or both is
`invalid-include`.

`ref` takes any address `henia show` reads: a whole skill, a skill's section,
a resource by path or as a module, an agent's or a command's sections.

```md
::include{ref="git"}

::include{ref="git#tagging"}

::include{ref="git/reference/hooks.md#pre-push"}

::include{ref="git.reference.hooks#pre-push"}

::include{ref="@reviewer#output"}
```

The included document is expanded for the same harness with its own package's
components, and the section is selected from the result, as `henia show`
renders it. A section that the target renders only for some harnesses is
missing for the others. An unknown document or section is `missing-include`,
and includes that reach back to their own document are `include-cycle`, with a
note per call on the way. An error in the included document stops the
including one.

`component` renders a shared fragment where a skill has no directive of its
own:

```sh
cat > .henia/components/footer.md <<'EOF'
---
form: leaf
attributes:
  owner:
    type: string
    default: platform
body:
  kind: none
---
Owned by the {{.owner}} team.{{with .oncall}} On call: {{.}}.{{end}}
EOF
mkdir -p skills/handoff
cat > skills/handoff/SKILL.md <<'EOF'
---
name: handoff
description: Hand a release over.
henia:
  variables:
    oncall: dana
---

# Handoff

::include{component="footer"}

::include{component="footer" owner="release"}

:::when{shell="zsh"}
Reload the shell after the upgrade.
:::

:::unless{target="claude"}
Ask the user before tagging.
:::
EOF
cat >> henia.toml <<'EOF'

[harness.claude.variables]
shell = "zsh"
EOF
```

```console
$ henia build
Built 6 artifact(s) in .henia/build
$ sed -n '/^# /,$p' .henia/build/claude/skills/handoff/SKILL.md
# Handoff

Owned by the platform team. On call: dana.

Owned by the release team. On call: dana.

Reload the shell after the upgrade.
$ sed -n '/^# /,$p' .henia/build/codex/skills/handoff/SKILL.md
# Handoff

Owned by the platform team. On call: dana.

Owned by the release team. On call: dana.

Ask the user before tagging.
```

### when and unless

`when` keeps its body where every condition holds, and `unless` where one
fails:

- `target` lists harness names; it holds for the harness being built. A name
  no harness configures is `unknown-target`.
- Any other attribute names a harness variable and holds when the harness's
  value equals the attribute's text. A variable no configured harness declares
  is `unknown-variable`.

Conditions are checked against every configured harness, also when
`--harness` narrows the build.

### head

`:::head` marks the head of a hybrid skill, which `henia show --head` prints.
Without a `head` component its body is the head. With one, the component's
output replaces the body, inside the head:

```sh
cat > .henia/components/head.md <<'EOF'
---
form: container
attributes:
  title:
    type: string
    required: true
body:
  kind: markdown
---
## {{.title}}

::body
EOF
mkdir -p .henia/skills/review
cat > .henia/skills/review/SKILL.md <<'EOF'
---
name: review
description: Review a change.
---

# Review

:::head{title="Start here"}
Read the diff first.
:::

## Checklist

- Tests pass.
EOF
```

```console
$ henia show review --head | sed -n 1,3p
## Start here

Read the diff first.
```

### related

`:related[git]` lists related skills as [extra files](build.md#extra-files)
describe. A `:::related` container renders a `## Related` heading followed by
its body.

## Expansion

Henia expands each document per harness, before slots, the head split and
rendering. Each round validates every directive against its declaration,
resolves `when`, `unless` and `include`, and expands components inside out: a
call's body is expanded before the call. Directives a template emits join the
next round and are checked and expanded like authored ones. A round that
changes nothing ends expansion. A component whose output reaches its own call
again is `component-cycle`.

Every node expansion creates keeps a backtrace: the call in the skill and the
component line that produced it. A diagnostic on generated output points at
the call, with a note naming the component:

```sh
cat > .henia/components/steps.md <<'EOF'
---
form: leaf
body:
  kind: none
---
:::routes{order}
| Step | Command |
|---|---|
| build | make |
:::
EOF
mkdir -p skills/rollback
cat > skills/rollback/SKILL.md <<'EOF'
---
name: rollback
description: Roll a release back.
---

# Rollback

:::routes{ordered=yes}
| Argument | Route |
|---|---|
| none | Revert the tag. |
:::

::run

::steps
EOF
```

```console
$ henia lint
skills/rollback/SKILL.md:8:1: error [invalid-attribute] routes attribute ordered: "yes" is not a boolean
skills/rollback/SKILL.md:14:1: error [invalid-body] run requires a body
skills/rollback/SKILL.md:16:1: error [unknown-attribute] routes does not declare attribute order
  note: expanded from component .henia/components/steps.md:6
Error: lint found 3 diagnostic(s)
```

Errors stop the document: `henia build` and `henia show` print every
diagnostic and render nothing for it, while unrelated documents still build.
[Lint](lint-rules.md#stages) runs the same checks in its source and expanded
stages.

## Formatting

`henia fmt` rewrites directive fences from the tree: each outer fence becomes
one colon longer than the longest fence inside it. Nothing else changes.

```sh
cat > nested.md <<'EOF'
:::when{target="claude"}
:::instruction
Ask first.
:::
:::
EOF
```

```console
$ henia fmt --check nested.md
nested.md
Error: 1 file(s) would change
$ henia fmt nested.md && cat nested.md
::::when{target="claude"}
:::instruction
Ask first.
:::
::::
```

Without paths, `henia fmt` formats the Markdown files under the current
directory. `--check` lists the files it would change, writes nothing, and
exits non-zero if there are any.
