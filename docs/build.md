# Building

`henia build` compiles canonical skills into one tree per harness. It reads
the skills and the `henia.toml` beside them, and it never changes the input
files.

```sh
mkdir -p skills/release-notes/reference instructions
cat > skills/release-notes/SKILL.md <<'EOF'
---
name: release-notes
description: Draft release notes from the merged pull requests since the last tag.
henia:
  variables:
    audience: users
    sections: [Added, Fixed]
---

# Release notes

Write for {{.audience}}. Group the merged pull requests under these headings:

{{range .sections}}- {{.}}
{{end}}
:::warning
Leave out internal refactors.
:::

See the [style notes](reference/style.md).
EOF
printf '# Style\n\nOne line per change.\n' > skills/release-notes/reference/style.md
printf '# Agents\n\nSee [the notes](../skills/release-notes/SKILL.md).\n' > instructions/AGENTS.md
cat > henia.toml <<'EOF'
[build]
clean = true

[harness.claude]
directives = "xml"

[harness.claude.variables]
audience = "maintainers"

[harness.codex]

[harness.codex.files]
"AGENTS.md" = { source = "instructions/AGENTS.md", replace = { "](../" = "](" } }
EOF
```

```console
$ henia build
Built 2 artifact(s) in .henia/build
$ find .henia/build -type f | sort
.henia/build/claude/skills/release-notes/reference/style.md
.henia/build/claude/skills/release-notes/SKILL.md
.henia/build/codex/AGENTS.md
.henia/build/codex/skills/release-notes/reference/style.md
.henia/build/codex/skills/release-notes/SKILL.md
$ sed -n '/^# /,$p' .henia/build/claude/skills/release-notes/SKILL.md
# Release notes

Write for maintainers. Group the merged pull requests under these headings:

- Added
- Fixed

<warning>
Leave out internal refactors.
</warning>

See the [style notes](reference/style.md).
$ cat .henia/build/codex/AGENTS.md
# Agents

See [the notes](skills/release-notes/SKILL.md).
```

Claude gets its own audience and XML tags; Codex keeps the skill's defaults
and the directive as written, and gets an `AGENTS.md` with its links fixed up.

## Templates

A skill body is a Go template. `if`, `range`, `with`, `define` and `template`
all work, and so does plain interpolation. The values come from three layers,
later ones winning:

1. the skill's frontmatter;
2. its `henia.variables`;
3. the harness's `[harness.<name>.variables]`, which take strings only.

Strings nested in the frontmatter are templated too. The body goes through
templates first, then directives, then references. Metadata expressions in
vendor profiles see the templated frontmatter on their own.

### Shared templates

Every `*.md.tmpl` file in `.henia/templates/` and
`$XDG_CONFIG_HOME/henia/templates/` joins each skill body's template set under
its file name, so `head.md.tmpl` is the template `static`. A project template
shadows a user template of the same name. A skill includes one with
`{{template "head" .}}` and overrides any `{{block}}` it declares with its own
`{{define}}`:

```markdown
{{define "context"}}:slot[git.commits]

{{end}}{{template "head" .}}
```

### Layouts

`henia.layout` names a shared template that renders the whole body. The skill
then holds only `{{define}}` blocks; text outside them fails the build, so every
skill on the layout keeps the layout's sections in the layout's order.
`{{required "name"}}` as a block's default fails the build when a skill leaves
that block out:

```markdown
<!-- .henia/templates/skill.md.tmpl -->
# {{.name}}

{{block "lead" .}}{{required "lead"}}{{end}}

## Procedure

{{block "procedure" .}}{{required "procedure"}}{{end}}
{{block "notes" .}}{{end}}
```

```markdown
---
name: deploy
description: Ship a release to production.
henia:
  layout: skill
---
{{define "lead"}}Tags and ships a release.{{end}}
{{define "procedure"}}1. Run `make release`.{{end}}
```

### Template directives

A block directive renders a shared template: the one its `template`
attribute names, or else the one named after the directive, so `:::head`
always goes through `head.md.tmpl` and `:::related` through
`related.md.tmpl` when they exist. The template sees the skill's values plus
`.args`, the directive's other attributes with each class as a `true` flag,
and `.content`, the directive's rendered body, empty when it holds only blank
lines. The template's output replaces the directive. `:::head` alone keeps
its fences, without attributes, so it still marks a hybrid head:

```markdown
<!-- .henia/templates/head.md.tmpl -->
{{if .args.context}}## Context

{{end}}{{with .args.slots}}{{template "slots" $}}{{end}}{{with .args.title}}# {{.}}

{{end}}{{.content}}{{if .args.contents}}
:contents[]
{{end}}
```

```markdown
:::head{.context .contents title="Git" slots="git.branching git.commits"}
Trunk-based development with short-lived branches.
:::
```

With no arguments the template above renders the content alone, so a bare
`:::head` keeps working as before. Lint renders directive templates with the
skill's values and the project's harness variables, so a `:slot[...]` a
partial emits counts as applied.

`henia lint` reports `unknown-template` for a `{{template}}` call or layout
that names no shared template and no block of the same file, and
`unused-template` for a project template that no scanned skill or other
template uses.

## Directives

Block directives `:::name{attrs}` and inline directives `:name[text]{attrs}`
nest, take quoted attributes, `#id` and `.class`. `directives` sets how a
harness renders them, whatever its vendor:

| Value | Output |
| --- | --- |
| `keep` | the directive as written, with spacing and attributes normalized; the default |
| `xml` | an XML tag per directive; ordinary Markdown stays as it is |
| `markdown` or `md` | the directive's label in bold, then its content |

XML is Henia's default for Claude only. Any harness can take any of the three.
Code, escaped syntax and two-colon leaf directives such as `::divider` pass
through unchanged.

## Harnesses

Without a `[harness]` table, a build writes all nine bundled profiles.
Declaring harnesses selects only those, and `--harness claude,codex` narrows
the selection further. A harness named after a bundled profile inherits it.

| Profile | Writes |
| --- | --- |
| `claude` | `claude/skills/<name>/SKILL.md` |
| `codex` | `codex/skills/<name>/SKILL.md`, plus `agents/openai.yaml` |
| `opencode` | `opencode/skills/<name>/SKILL.md` |
| `gemini` | `gemini/skills/<name>/SKILL.md` |
| `github` | `github/skills/<name>/SKILL.md` |
| `cursor` | `cursor/skills/<name>/SKILL.md` |
| `chatgpt` | `chatgpt/skills/<name>/SKILL.md` |
| `claude-upload` | `claude-upload/skills/<name>/SKILL.md` |
| `agentskills` | `agentskills/skills/<name>/SKILL.md` |

A profile maps frontmatter keys, values and types, drops what the vendor does
not support with a warning, and writes sidecar files. `strict = true` turns
those warnings into errors. Upload profiles only stage directories; Henia
uploads nothing and builds no ZIPs.
[Vendor profiles](vendor-profiles.md) covers custom profiles, and
[vendor contracts](vendor-research.md) the formats behind them.

Harnesses build skills. `artifacts = ["skills", "commands", "agents"]`, per
harness or at the top level, adds commands and agents, and each type takes its
own `profile`, `layout` and `frontmatter`:

```toml
[harness.claude.agents]
profile = "claude-agent"
layout = "flat"
```

The `claude-agent` profile lives at `.henia/harnesses/claude-agent/transform.toml`.
`[harness.<name>.frontmatter]` renames keys with `rename` and maps values with
`values`, per harness or per artifact type.

## Extra files

`[harness.<name>.files]` copies support documents into a harness tree, as
with `AGENTS.md` above. The source resolves inside the input directory, the
destination inside the harness root. Markdown sources render their references
in the harness's syntax first; `replace` then applies literal replacements, all
at once.

## Output

Build writes `<output>/<harness>/...`. The output defaults to `.henia/build`:

```toml
[build]
output = "dist/skills"
```

A relative `output` in a config file resolves against that file;
`--output` resolves against the current directory and wins.

With `clean = true`, or `--clean`, the output directory belongs to Henia. A
build writes everything into a temporary sibling and swaps it in only when
every write succeeds, so removed skills disappear and a failed build leaves the
last good tree. That holds when `--harness` narrows the build too: the other
harness trees go.

```console
$ henia build --harness codex --output out
Built 1 artifact(s) in out
$ find out -type f | sort
out/codex/AGENTS.md
out/codex/skills/release-notes/reference/style.md
out/codex/skills/release-notes/SKILL.md
```

Every transformation and every output path is checked before anything is
written. Two artifacts claiming one path, or a source sidecar and a generated
one, fail the build. Writes never follow a symlink out of the harness root.
Resources copy verbatim and keep their executable bit; a symlinked resource is
an error.

A project with skill package dependencies builds them with its own skills.
Each dependency renders with its own harness `variables`, `tools` and
`references`; the project's harness of the same name overrides them key by
key.

## Configuration

Henia merges the bundled defaults, `~/.config/henia/henia.toml`, then the
project's `henia.toml`. Tables merge recursively, scalars override (`false`
included), and arrays concatenate. Unknown keys are errors.

`henia build <source>` reads `<source>/henia.toml` when it exists, else
`./henia.toml`, and `--config` always wins. Relative paths in a project config
resolve against its directory. `--output`, `--clean` and `--harness` override
the config.

To build from a [Phora](https://github.com/srnnkls/phora) hook and deploy the
result, see [Phora integration](phora.md).
