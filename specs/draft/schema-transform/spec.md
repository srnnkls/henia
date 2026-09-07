---
issue_type: Feature
created: 2026-01-18
updated: 2026-09-07
status: Implemented
stage: draft
---

# Harness transforms and lint rules with TOML + Expr

## Goal

Compile canonical Markdown skills into vendor packages without duplicating author
content. Keep templates, semantic markup, metadata transforms and lint rules as
separate stages built on existing language engines.

The user selected TOML + Expr on 2026-09-07. This supersedes the earlier requirement
for Starlark scripts, their module/helper API and interpreter-specific step limits.
Starlark and CUE are deferred. Go templates remain required for body composition.

## Required pipeline

- Parse YAML and preserve canonical types and extra fields.
- Execute Go templates using frontmatter, Henia variables and harness overrides.
  Support conditions, ranges and named template composition, including nested
  frontmatter values.
- Transform metadata with declarative profile aliases and Expr computations.
- Parse semantic directives through Goldmark and select XML, directives or Markdown
  rendering independently of vendor profile.
- Transform body references after directive rendering.
- Validate every output path and collision before writing skills, resources and
  additional generated files.

## Canonical metadata

The Go schema exposes name, description, model tier, tools, tool policy, enabled
and user-invocable values, preserves explicit false/empty values, and round-trips
unknown fields through `ToMap()`. Built-in skill profiles require a valid name
matching the output directory and a nonempty description.

`henia.variables` is template-only data. `henia.auto_invoke` and
`henia.user_invocable` are portable controls. Native frontmatter and file metadata
live under `henia.targets.<profile>`. Unsupported behavior produces a warning;
strict profiles turn warnings into errors. Type errors always fail.

## Declarative extension model

Profiles are TOML definitions with:

- `fields`: output field allowlist.
- `aliases`: key renames.
- `computed`: output field to Expr expression.
- `consume`: inputs consumed by computations.
- `controls`: supported portable invocation preferences.
- `files`: path, serialization format and value expression.

Expressions receive `input`, `settings`, `native` and `ctx`. They support value
conversion, boolean inversion and reshaping without custom parsers. Shared helpers
map tool names, merge maps, and require a nonempty value. Computations do not
mutate inputs or depend on evaluation order.

Additional files are relative to the harness root. JSON and YAML serialize data;
text requires a string. Nil or empty-map results suppress file emission. Multiple
skills claiming a shared configuration file fail with a collision instead of
silently overwriting one another. Per-skill mappings must not implicitly grant
harness-wide permissions; the previous OpenCode permission-emission sketch is
superseded by the researched skill schema.

## Resolution and compatibility

Profile tables merge from bundled definitions, user
`~/.config/henia/harnesses/<profile>/transform.toml`, then project
`.henia/harnesses/<profile>/transform.toml`. A custom profile needs no Go change.
Unreadable or invalid overrides fail instead of falling back silently.

Configuration merges bundled defaults, user `~/.config/henia/henia.toml`, then
project TOML. Scalars override including false; arrays concatenate with later
items last; tables merge recursively. Explicit configurations select only the
harnesses they declare. Existing configurations without a `profile` retain the
legacy mapper. Default builds support all nine researched skill profiles.

## Lint rule DSL

`[[lint.rules]]` declares `id`, `select`, optional `when`, `assert`, `message` and
optional severity. Selectors cover documents, frontmatter keys, directives,
headings, paragraphs, links and images. Expr predicates must be boolean. A false
assertion reports at the selected source location. Configuration and evaluation
errors are distinct from ordinary failed assertions.

Collection checks identify duplicate names, headings and exact normalized
paragraphs, including paragraphs within directives. Optional word-shingle Jaccard
and containment measures report lexical overlap at independent thresholds.
Optional Model2Vec embeddings run entirely in process from local model files and
report semantic candidates by cosine similarity; enabling them requires an explicit
model path and threshold. No servers, subprocesses or model downloads participate
in linting. Paragraph edit distance is removed. Diagnostics include the matching
source location, method, numeric score, shared phrases for lexical findings and
model path for semantic findings. Exact then lexical findings take precedence over
semantic findings. Scores are not probabilities or semantic equivalence claims.

Rules run without executing templates. Dynamic nodes are skipped by default;
compiled output can be linted to validate expanded values. Literal code is not
misidentified as directive content. Built-in and custom rules share disabling,
severity, strict exit handling and JSON diagnostics.

## Runtime and filesystem boundaries

Expressions have no filesystem or network API, and no ambient clock function.
Source size, expression AST size and VM collection allocation limits are enforced;
these are not total memory or wall-clock quotas. No arbitrary runtime scripts load.
Output paths must be local and canonical; writes use `os.Root` to prevent escapes
through symlinks. Resource symlinks are rejected. Write-time failures can leave
partial output; stale outputs are not automatically removed.

## Acceptance evidence

- Vendor profile tests cover all nine profiles, boolean/type/value mapping, native
  overrides, OpenAI sidecars, unsupported fields and custom profile precedence.
- Lint DSL tests cover selectors, nested/code/dynamic directives, YAML source
  locations, invalid expressions, runtime errors and disabling.
- CLI tests verify template composition through vendor rendering and sidecar
  output, plus custom-rule JSON diagnostics and exit status.
- Config tests verify false overrides, recursive merging, array ordering, legacy
  compatibility and rejection of misspelled rule keys.
- Output tests verify traversal, source-resource conflicts and escaping symlinks.

See [profile reference](../../../docs/vendor-profiles.md),
[lint DSL](../../../docs/lint-rules.md) and
[vendor research](../../../docs/vendor-research.md).
