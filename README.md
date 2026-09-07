# Henia

Henia compiles canonical Markdown skills for multiple AI harnesses and lints their
metadata, directives, references and structure. Write a skill once, use Go
templates to compose its body, and select a vendor profile for its output.

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

Body processing remains **templates → Goldmark directives → references**.
Metadata expressions receive the templated canonical frontmatter independently.

## Vendor profiles

| Profile | Default skill destination / package staging |
|---|---|
| `claude` | `.claude/skills/<name>/SKILL.md` |
| `codex` | `.agents/skills/<name>/SKILL.md` |
| `opencode` | `.opencode/skills/<name>/SKILL.md` |
| `gemini` | `.gemini/skills/<name>/SKILL.md` |
| `github` | `.github/skills/<name>/SKILL.md` |
| `cursor` | `.cursor/skills/<name>/SKILL.md` |
| `chatgpt` | `.henia/packages/chatgpt/skills/<name>/SKILL.md` |
| `claude-upload` | `.henia/packages/claude-upload/skills/<name>/SKILL.md` |
| `agentskills` | `.henia/packages/agentskills/skills/<name>/SKILL.md` |

Profiles map keys, values and types, filter unsupported metadata with diagnostics,
and generate additional files such as `agents/openai.yaml`. Upload profiles stage
skill directories; they do not upload files, create ZIPs or publish plugins.
See [vendor contracts and sources](docs/vendor-research.md) and
[custom profiles](docs/vendor-profiles.md).

```toml
[harness.claude]
profile = "claude"
path = ".claude"
format = "xml"
strict = true

[harness.codex]
profile = "codex"
path = ".agents"
format = "directives"
```

Rendering is independent of the vendor:

- `xml`: convert directive nodes to XML tags; keep ordinary Markdown.
- `directives` (default): keep directives, normalizing spacing and attributes.
- `markdown`: turn directive labels and attributes into bold Markdown labels.

XML is Henia's default for Claude, not a vendor requirement. Any profile can use
any of these formats. Block `:::name{attrs}` and inline `:name[text]{attrs}` support
nesting, quoted attributes, `#id` and `.class`. Two-colon leaf directives are
currently deferred. Literal code and escaped syntax remain unchanged.

## Lint rules

Built-in rules check metadata, template/directive syntax, local links, artifact
references, duplicate headings/content/skills, optional word-shingle overlap and
containment, local semantic similarity, known outdated references, line budgets
and review dates. Exact paragraph checks run by default. Enable lexical measures
with `[lint] duplicate_similarity = 0.7` and `duplicate_containment = 0.9`;
`duplicate_min_words` defaults to 12. Semantic checks use in-process Model2Vec
with local weights and an explicit threshold in `[lint.semantic]`. Findings include
the matching location, method, score and shared phrases where applicable.
See [configuration and model setup](docs/lint-rules.md).
Add your own rules in `henia.toml`:

```toml
[[lint.rules]]
id = "instruction-priority"
select = "directive"
when = 'node.name == "instruction"'
assert = 'node.attrs.priority in ["normal", "high", "critical"]'
message = "Instructions need a valid priority."
severity = "warning"
```

`select` chooses nodes, `when` filters them, and `assert` must hold for each match.
Conditions use [Expr](https://expr-lang.org/docs/language-definition).
Failures report the rule ID and source location. Invalid expressions and runtime
errors are errors, never successful checks. See the [lint DSL reference](docs/lint-rules.md).

```bash
henia lint skills --strict
henia lint skills --format json
henia lint skills --disable duplicate-heading,instruction-priority
```

The linter does not execute templates or fetch URLs. Dynamic nodes are skipped by
custom rules by default; lint compiled output to check expanded values. Errors
fail lint; `--strict` also fails on warnings. JSON output is an array of diagnostics.

## Build and deploy

```bash
# Compile local skills into a separate tree, without fetching or installing.
henia build examples --output .henia/build --harness claude,codex

# Fetch configured sources and deploy them.
henia sync --config henia.toml

# Deploy already fetched sources.
henia deploy --config henia.toml
```

Build writes `<output>/<harness>/skills/<name>/...`. Without a configuration it
uses all nine bundled profiles. An explicit configuration selects only the
harnesses declared in project or user configuration; it does not enable unrelated
installations. `update` aliases `sync`; `add <repo> --ref <ref>` adds and syncs a source.

```toml
[sources.team]
repo = "https://github.com/example/skills"
ref = "main"

[harness.claude]
profile = "claude"
path = ".claude"
```

Config merges bundled defaults, `~/.config/henia/henia.toml`, then project
`henia.toml`: tables merge recursively, scalars override (including `false`), and
arrays concatenate. Existing explicit harness configurations without `profile`
retain the legacy key/value mapper. Legacy flat output, command and agent artifacts
remain available through explicit configuration; the researched profiles target skills.

All transformations and output collisions are checked before writing. Generated
paths are relative to the harness root; writes cannot follow symlinks outside it.
Resources are copied verbatim, executable modes are retained, and resource symlinks
are rejected. A source sidecar and a generated sidecar cannot claim the same path.
Write-time filesystem errors can leave partial output. Rebuilds overwrite generated
files but do not prune stale files; use a clean build directory after removing a
skill or sidecar declaration.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
```

The repository also has Scrut CLI tests (`mise run test:integration`). Specs live
under [`specs/draft`](specs/draft); the harness scope now uses TOML + Expr. Starlark
and CUE are not required or executed.
