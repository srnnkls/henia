# Lint rule DSL

## Duplicate and similar content

Collection-wide checks run alongside the per-node Expr rules:

- `duplicate-skill`: duplicate artifact names across scanned files.
- `duplicate-heading`: repeated headings within a document.
- `duplicate-content`: repeated paragraphs within or across files, including
  paragraphs inside directives and list items.
- `similar-content`: optional near-duplicate paragraphs using normalized
  Levenshtein edit distance.

```toml
[lint]
duplicate_min_words = 12
duplicate_similarity = 0.9
```

Exact comparisons ignore case and whitespace but retain Markdown syntax, link
destinations and inline directive attributes. Code blocks and paragraphs containing
template actions are excluded. The minimum length defaults to 12 words; lower it
to catch short repeated instructions. Near-duplicate checking defaults to off
(`duplicate_similarity = 0`). A threshold of `1` permits only exact matches,
which the exact-duplicate rule handles.

Similarity is `1 - edits / max(length(a), length(b))`, using Unicode characters
after the same normalization. Each insertion, deletion or substitution costs one
edit. A score of `0.9` means at most roughly 10% character edits; it is a
deterministic text similarity score, not a probability or semantic judgment.
Reordered passages and paraphrases may score poorly even when their meaning is
similar. Results are warnings for review, not automatic rewrites.

Each paragraph reports its closest qualifying earlier paragraph in the sorted
scan order. Ties favor the first occurrence. Exact duplicates produce only the
exact warning. Diagnostics identify the first/matching location in `related`;
near-duplicate JSON diagnostics also expose a numeric `similarity` field.

These checks maintain a collection index in Go. Expr predicates still operate on
individual selected nodes. Length and character-count filters plus a bounded
distance calculation reduce comparison work; large collections with many similar
paragraphs can still require quadratic candidate comparisons.

## Custom rule declarations

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
