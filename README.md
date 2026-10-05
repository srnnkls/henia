# Henia

Henia compiles canonical Markdown skills for multiple AI harnesses, lints their
metadata, directives, references and structure, and runs a skill runtime
that serves, composes and renders installed skills on demand. Write a skill once, use Go
templates to compose its body, and select a vendor profile for its output.
Phora owns source fetching, installation, deployment state and hook orchestration.

## Quick start

Requires Go 1.26.5 or later.

```bash
go build -o /tmp/henia ./cmd/henia
/tmp/henia build examples --config examples/henia.toml --output .henia/build
/tmp/henia lint examples --config examples/henia.toml --strict
```

The [example skill](examples/skills/review/SKILL.md) demonstrates nested directives,
Go templates, canonical variables and OpenAI sidecar metadata.

## Canonical skills and templates

```markdown
---
name: review
description: Review code changes for correctness and missing coverage.
henia:
  variables:
    priority: high
    checks: [Correctness, Coverage]
---

:::instruction{priority={{.priority}}}
{{range .checks}}- Check {{.}}.
{{end}}
:::
```

Go templates support `if`, `range`, `with`, named `define`/`template` composition
and interpolation. The template context starts with frontmatter, then
`henia.variables`, then harness variables (highest precedence). Nested frontmatter
strings are also templated. Input files are never modified by compilation.

Body processing remains *templates → Goldmark directives → references*.
Metadata expressions receive the templated canonical frontmatter independently.

## Vendor profiles

| Profile | Default build artifact |
|---|---|
| `claude` | `.henia/build/claude/skills/<name>/SKILL.md` |
| `codex` | `.henia/build/codex/skills/<name>/SKILL.md` |
| `opencode` | `.henia/build/opencode/skills/<name>/SKILL.md` |
| `gemini` | `.henia/build/gemini/skills/<name>/SKILL.md` |
| `github` | `.henia/build/github/skills/<name>/SKILL.md` |
| `cursor` | `.henia/build/cursor/skills/<name>/SKILL.md` |
| `chatgpt` | `.henia/build/chatgpt/skills/<name>/SKILL.md` |
| `claude-upload` | `.henia/build/claude-upload/skills/<name>/SKILL.md` |
| `agentskills` | `.henia/build/agentskills/skills/<name>/SKILL.md` |

Profiles map keys, values and types, filter unsupported metadata with diagnostics,
and generate additional files such as `agents/openai.yaml`. Upload profiles stage
skill directories; they do not upload files, create ZIPs or publish plugins.
See [vendor contracts and sources](docs/vendor-research.md) and
[custom profiles](docs/vendor-profiles.md).

```toml
[harness.claude]
profile = "claude"
directives = "xml"
strict = true

[harness.codex]
profile = "codex"
directives = "keep"
```

`directives` sets how `:::` directives render, independent of the vendor:

- `xml`: convert directive nodes to XML tags; keep ordinary Markdown.
- `keep` (default): keep directives, normalizing spacing and attributes.
- `markdown` (or `md`): turn directive labels and attributes into bold Markdown labels.

XML is Henia's default for Claude, not a vendor requirement. Any profile can use
any of these formats. Block `:::name{attrs}` and inline `:name[text]{attrs}` support
nesting, quoted attributes, `#id` and `.class`. Two-colon leaf directives are
currently deferred. Literal code and escaped syntax remain unchanged.

## Lint rules

Lint rules are [hq](docs/query.md) queries in Markdown modules. The standard
library checks metadata, templates and directives, local links and references,
outdated names, line budgets, review dates, duplicate headings, content and
skills, and slots, with optional word-shingle and Model2Vec similarity.
Projects add modules under `.henia/lint/`, users under `~/.config/henia/lint/`,
and installed sources under their own `lint/`; modules import the standard
library's patterns and compose them. `henia.toml` tunes rules and holds one-off
rules:

```toml
[lint.config.large-skill]
max-lines = 600

[[lint.rules]]
id = "instruction-priority"
query = '(directive :name "instruction" :priority (not /^(normal|high|critical)$/)) @d'
message = "Instructions need a valid priority."
```

```bash
henia lint skills --strict
henia lint skills --format json
henia lint skills --disable duplicate-heading,instruction-priority
henia lint test
```

The linter does not execute templates or fetch URLs, and skips Git-ignored
files below each path it scans. Errors fail lint; `--strict` also fails on
warnings. See [lint rules](docs/lint-rules.md).

## Slots

Skills declare typed extension points in `metadata.slots` and fill them in
`metadata.provides`; priorities decide which providers shadow others. Types and
priorities follow the Nix module system (`listOf`, `uniq`, `attrsOf`; `mkForce`,
plain definitions, `mkDefault`):

```yaml
# skills/code/SKILL.md
metadata:
  slots: "code.style:keyed(list) review.criteria:unique"

# .agents/skills/house-go/SKILL.md, in a repository
metadata:
  provides: "code.style.go review.criteria@fallback"
```

Installed skills stay open for extension, so `henia slots` resolves them when a
skill runs, over a harness's global skills directory and the project's skill
directories. Skills preload the resulting rows; `--explain` shows where each
provider and override comes from, `--json` emits the same, and `--check`
reports problems:

```bash
henia slots --global ~/.claude/skills code.style review.criteria
henia slots --global ~/.claude/skills --explain code.style.go
```

See [slots](docs/slots.md) for types, priorities, output and lineage, and
[Tropos's COMPOSITION.md](https://github.com/srnnkls/tropos/blob/main/COMPOSITION.md)
for a generated catalog of real seams.

## Skill runtime

Henia serves skills from a library of installed sources, rendered for the
calling agent, while each harness lists only the entry skills projected into
it:

```bash
henia ls
henia show style-guide#go
henia query '(skill :id "style-guide" (code :lang "go") @c)'
henia context code --global ~/.claude/skills
```

See [skill runtime](docs/runtime.md).

## Build artifacts for Phora

```bash
henia build examples --output .henia/build --harness claude,codex
```

Build writes `<output>/<harness>/skills/<name>/...`. The output directory defaults
to `.henia/build` and can be configured independently of the vendor profiles:

```toml
[build]
output = "dist/skills"

[harness.claude]
profile = "claude"
directives = "xml"
```

A configured relative output path resolves against its configuration file.
`--output` overrides the setting and resolves against the invocation directory.
With no explicit harness selection in config, builds use all nine bundled profiles;
declaring harnesses selects only those entries. `--harness` narrows that selection.

Henia consumes local directories and writes build artifacts. It has no `add`,
`sync`, `update` or `deploy` command, source registry, deployment lock or Phora
library dependency; installation paths belong in Phora configuration, and
`[build].output` sets where compilation writes.

Phora can invoke `henia build` from a hook and consume each harness output as a
local source. See the [Phora integration guide](docs/phora.md) for the phase order
and example configurations.

Config merges bundled defaults, `~/.config/henia/henia.toml`, then project
`henia.toml`. Without `--config`, `henia build <source>` reads
`<source>/henia.toml` when present, else `./henia.toml`; `--config` always wins.
Relative paths in a project config resolve against its directory, and `--output`,
`--clean` and `--harness` override its settings. `henia lint` reads
`--config` or `./henia.toml`. Tables merge recursively, scalars override
(including `false`), and arrays concatenate. A declared harness with a bundled
name inherits that preset, including its profile. Harnesses build skills unless
their `artifacts` (or the top-level `artifacts`) add `commands` or `agents`.
Unknown keys are errors.

All transformations and output collisions are checked before writing. Generated
paths are relative to the harness root; writes cannot follow symlinks outside it.
Resources are copied verbatim, executable modes are retained, and resource symlinks
are rejected. A source sidecar and a generated sidecar cannot claim the same path.
Use `[build] clean = true` (or `--clean`) when Phora consumes the output tree.
Henia then writes every artifact into a temporary sibling and replaces the output
only after all writes succeed. Removed artifacts and resources disappear on a
successful rebuild; failures leave the previous output intact. The entire output
directory is compiler-owned in this mode, including when `--harness` narrows the
selection. Without `clean`, builds retain the existing overwrite behavior.

Each artifact type has its own table under the harness, taking the harness's
`profile`, `layout` and `frontmatter` keys, so agent metadata and layout can be
configured separately from skill profiles:

```toml
[harness.claude.agents]
profile = "claude-agent"
layout = "flat"
```

The custom profile lives at `.henia/harnesses/claude-agent/transform.toml` and uses
the same declarative fields, computed values and native overrides as skill
profiles. `[harness.<name>.frontmatter]` renames keys (`rename`) and maps values
(`values`), per harness or per artifact type. Support documents can be included
without a post-processing script:

```toml
[harness.claude.files]
"instructions/AGENTS.md" = { source = "instructions/AGENTS.md" }
"CLAUDE.md" = { source = "instructions/AGENTS.md", replace = { "](../" = "](" } }
```

Source paths resolve inside the canonical input directory; output paths are
relative to the harness root. Markdown sources render references with the
harness's `references` syntax before replacements, which are literal and
simultaneous. These files participate in collision checks and clean publication.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
```

The repository also has Scrut CLI tests (`mise run test:integration`) and live
harness acceptance tests that compose slots through `claude -p` and `codex exec`
(`mise run test:acceptance`, see [slots](docs/slots.md#acceptance-tests)). Specs
live under [`specs/draft`](specs/draft); the harness scope now uses TOML + Expr.
Starlark and CUE are not required or executed.
