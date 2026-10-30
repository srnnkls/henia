# Building

`henia build` compiles canonical skills into one tree per harness. It reads
the skills and the `henia.toml` beside them, and it never changes the input
files.

```sh
mkdir -p skills/release-notes/reference instructions .henia/components
cat > .henia/components/warning.md <<'EOF'
---
form: container
doc: A warning the harness renders.
render: keep
---
EOF
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
`warning` is a [component](components.md) that the harness renders.

## Values

A skill body fills in the skill's values with Go `text/template`: plain
interpolation, `if`, `with` and `range`. The values come from three layers,
later ones winning:

1. the skill's frontmatter;
2. its `henia.variables`;
3. the harness's `[harness.<name>.variables]`, which take strings only.

```markdown
Ask the {{.team}} team.{{with .oncall}} On call: {{.}}.{{end}}
```

A value another skill or harness sets but this one does not is absent: it
prints nothing, and `if` and `with` treat it as false. A field no skill or
harness knows fails with `invalid-template`. Code spans and code blocks stay
literal. Strings nested in the frontmatter are templated too.

Values fill in text; shared structure comes from
[components](components.md), which a skill calls as directives.

## Directives

Every directive in a skill is a [component](components.md): declared in
`.henia/components/`, checked against its declaration, and expanded before the
skill renders. An undeclared directive fails the build. A component declared
`render: keep`, such as `warning` above, stays a directive, and `directives`
sets how a harness renders it, whatever its vendor:

| Value | Output |
| --- | --- |
| `keep` | the directive as written, with spacing and attributes normalized; the default |
| `xml` | an XML tag per directive; ordinary Markdown stays as it is |
| `markdown` or `md` | the directive's name and attributes in bold, then its content |

The harness's profile sets the default: `xml` for `claude` and
`claude-upload`, `keep` for the other bundled profiles. Any harness can take any
of the three. Directive syntax in code and escaped colons pass through
unchanged.

## Harnesses

Without a `[harness]` table, a build writes all eleven bundled profiles.
Declaring harnesses selects only those, and `--harness claude,codex` narrows
the selection further. A harness named after a bundled profile inherits its
bundled settings, `profile` among them. When a skill package dependency
configures harnesses, the build writes only the harnesses the project or a
dependency configures; see [dependencies](#dependencies).

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
| `pi` | `pi/skills/<name>/SKILL.md` |
| `omp` | `omp/skills/<name>/SKILL.md` |

A profile maps frontmatter keys, values and types, drops what the vendor does
not support with a warning, and writes sidecar files. `strict = true` turns
those warnings into errors. Upload profiles only stage directories; Henia
uploads nothing and builds no ZIPs.
[Vendor profiles](vendor-profiles.md) covers custom profiles, and
[vendor contracts](vendor-research.md) the formats behind them.

Harnesses build skills. `artifacts = ["skills", "commands", "agents", "config"]`,
per harness or at the top level, selects the types; a later henia.toml layer
replaces the list. Each document type takes its own `layout` and `frontmatter`,
and skills and agents their own `profile`; commands render without one:

```toml
[harness.claude.agents]
profile = "claude-agent"
layout = "flat"
```

The `claude-agent` profile lives at `.henia/harnesses/claude-agent/transform.toml`.
`[harness.<name>.frontmatter]` renames keys with `rename` and maps values with
`values`, per harness or per artifact type.

`config` renders the harness's configuration files, such as Codex's
`config.toml` or Claude Code's `settings.json` and `.mcp.json`, from `[shared]`
and `[harness.<name>.config]`, with defaults the harness may change from
`[harness.<name>.default_config]`. Unrelated to `[lint.config]`, which
configures lint rules. [Harness configuration](vendor-profiles.md#harness-configuration)
covers both tables.

## Extra files

`[harness.<name>.files]` copies support documents into a harness tree, as
with `AGENTS.md` above. The source resolves inside the input directory, or
inside the skill package whose harness lists the file, and the destination
inside the harness root. A file only the user henia.toml lists belongs to the
user's harness homes: `henia install --global` resolves it in
`~/.config/henia/`, and project builds leave it out. Markdown sources render their references
in the harness's syntax and resolve their directives first; `replace` then
applies literal replacements, all at once. Other sources copy verbatim apart
from `replace`.

Nothing preloads a harness file at runtime, so its directives resolve when it
is built. A `:slot[…]` renders as in `henia show`: the list of providers, or
with `{.inline}` their content. `:contents[<skill>]` and `:related[<skill>]`
render the named skill's sections and its related skills. Providers resolve
over the library of whoever builds, as `henia slots` does, with the `[slots]`
of the project's `henia.toml`, or of the user `henia.toml` for
`henia install --global`.
A slot without a provider renders nothing:

```md
Before repository work, orient:

:slot[code.orientation]{.inline}
```

## Output

Build writes `<output>/<harness>/...`. The output defaults to `.henia/build`:

```toml
[build]
output = "dist/skills"
```

A relative `output` in a config file resolves against that file;
`--output` resolves against the current directory and wins.

`henia install` writes the same trees into the project's harness directories;
see [Installing](runtime.md#installing).

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

## Dependencies

A project with skill package dependencies builds them with its own skills.
When no dependency configures a harness, the project's harnesses render every
package, and each dependency's skills take its own harness `variables`,
`tools` and `references` as defaults, which the project's harness of the same
name overrides key by key.

A harness that a dependency configures renders as `henia install --global`
renders it: with that dependency's harness configuration, skill modes, agents
and `files`, its profiles resolved from its `.henia/harnesses/` and its file
sources from its own tree. The other packages, the project's included, render as its
dependencies. The user henia.toml merges over the dependency's configuration,
then the project's `[harness.<name>]` table over both key by key, so a project with Tropos as a dependency and only
`[harness.claude.variables]` builds Tropos's Claude harness with those
variables. When several dependencies configure one harness, their outputs
merge, and a file two of them write differently fails the build.

The project's harness table assigns modes only to what it lists. A skill,
command or agent it names in `static`, `hybrid` or `dynamic` takes that mode,
whichever package it comes from. The project's own unlisted artifacts take the
table's `default`, and are static without one; its lists never make them
dynamic. The configuring package's `default` and lists keep governing its own
and the other dependencies' unlisted artifacts, so this project table builds
the dependency `impeccable` static and changes nothing else:

```toml
[harness.claude.skills]
static = ["impeccable"]
```

A configuring package's `strict` covers its own artifacts only. The other
packages, the project's included, render strictly only when the project's table
sets `strict`; otherwise their unsupported frontmatter stays a warning.

## Configuration

Henia merges the bundled defaults, `~/.config/henia/henia.toml`, then the
project's `henia.toml`. A package that configures a harness merges between the
defaults and the user henia.toml, so the user's personal settings override the
package's in `henia install --global`, and a project's harness table overrides
both. Tables merge recursively, scalars override (`false` included), and arrays
concatenate, except under `artifacts`, `[shared]` and
`[harness.<name>.config]`, where a later array replaces an earlier one. The
native arrays a profile's config file lists in `extend`, such as each event
under Claude's `hooks`, keep the earlier items and add the later ones not
already present. Unknown keys are errors.

`henia build` without a source builds `.henia/` when it holds `skills`,
`commands` or `agents` and the current directory does not, else the current
directory. `henia build <source>` reads `<source>/henia.toml` when it exists,
else `./henia.toml`; a source named `.henia` reads its project's. `--config`
always wins. Relative paths in a project config resolve against its directory.
`--output`, `--clean` and `--harness` override the config.

To build from a [Phora](https://github.com/srnnkls/phora) hook and deploy the
result, see [Phora integration](phora.md).
