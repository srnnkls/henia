# Lint rule DSL

Declare `[[lint.rules]]` entries in project or user `henia.toml`. Rules augment the
built-in linter and run on parsed canonical Markdown. Conditions use
[Expr syntax](https://expr-lang.org/docs/language-definition), not Go templates.

```toml
[[lint.rules]]
id = "instruction-priority"
select = "directive"
when = 'node.name == "instruction"'
assert = 'node.attrs.priority in ["normal", "high", "critical"]'
message = "Instructions need a valid priority."
severity = "warning"
```

## Evaluation

1. `select` picks candidates of one kind.
2. `when` (default `true`) chooses candidates to check.
3. A false `assert` emits `message` at the selected candidate's location.

`id`, `select`, `assert` and `message` are required. IDs start with a lowercase
letter and contain lowercase letters, digits and hyphens. IDs must be unique and
cannot shadow built-in rules. `severity` is `warning` (default) or `error`.
Messages are literal strings. All expressions compile before scanning files;
unknown fields, invalid syntax and non-boolean predicates reject configuration.
Evaluation failures emit error diagnostics containing the rule ID and Expr error.

Use `--disable <id>` or `[lint].disable` for either built-in or custom rules.
Normal lint fails on errors; `--strict` also fails on warnings. Output supports
plain text and JSON with path, line, column, severity, rule and message.

## Selectors and environment

| Selector | One candidate per | Useful `node` fields |
|---|---|---|
| `document` | Markdown file | `text` |
| `frontmatter` | Existing top-level YAML key | `name`, `value`, `text` |
| `directive` | Container or inline directive | `name`, `attrs`, `text`, `inline` |
| `heading` | Heading | `level`, `text` |
| `paragraph` | Paragraph | `text` |
| `link` | Markdown link | `destination`, `text` |
| `image` | Markdown image | `destination`, `text` |

All candidates expose `kind`, `dynamic`, `start` and `end` (byte offsets relative
to the Markdown body; metadata offsets can be negative). Positions in diagnostics
are one-based coordinates in the original file. Frontmatter positions point to
the selected YAML key. Inline link positions point to link text.

Every expression can access:

- `frontmatter`: the complete parsed YAML map, including nested values.
- `document.path`: scanned file path.
- `document.kind`: `skill`, `command`, `agent`, or an empty string for resources.
- `document.body`: unexpanded Markdown body.
- `document.lines`: body line count.

Missing map keys evaluate to `nil`; use `??` for defaults or `in` for presence.
Use a document rule to require a missing field, because a `frontmatter` selector
only visits keys that exist.

```toml
[[lint.rules]]
id = "skill-description"
select = "document"
when = 'document.kind == "skill"'
assert = 'len(frontmatter.description ?? "") >= 30'
message = "Describe when to use this skill."
severity = "error"

[[lint.rules]]
id = "heading-depth"
select = "heading"
assert = 'node.level <= 3'
message = "Keep headings within three levels."

[[lint.rules]]
id = "https-docs"
select = "link"
when = 'node.destination startsWith "http"'
assert = 'node.destination startsWith "https://"'
message = "Use HTTPS documentation links."
```

## Templates and literal content

The linter parses Go templates without executing them. Rules skip nodes containing
template actions by default, avoiding false assertions about values that depend on
a harness. This is conservative: a directive containing a template in its body is
also considered dynamic. Document rules still run and see the original body.

`include_dynamic = true` explicitly includes those nodes. Their parsed markup may
contain neutral placeholders for template actions; use `node.dynamic` to distinguish
them. For checks against actual expanded values, compile and lint the result:

```bash
henia build skills-repo --harness codex --output .henia/build
henia lint .henia/build/codex --config henia.toml --strict
```

Goldmark handles nested directives, escaping and literal contexts. Directive rules
do not inspect examples inside code fences or inline code. No rule fetches URLs,
executes templates, runs commands or modifies files. Expressions receive data and
Expr built-ins; `now()` is disabled so rule results do not depend on the clock.
Expression source and AST size and VM collection allocations have fixed limits;
these are not a wall-clock or total process-memory quota.
