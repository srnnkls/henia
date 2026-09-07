# Lint rule DSL

## Duplicate and similar content

Collection-wide checks run alongside the per-node Expr rules:

- `duplicate-skill`: duplicate artifact names across scanned files.
- `duplicate-heading`: repeated headings within a document.
- `duplicate-content`: exact repeated paragraphs within or across files, including
  paragraphs inside directives and list items.
- `similar-content`: optional lexical overlap using word shingles and containment.
- `semantic-content`: optional semantic candidates using in-process Model2Vec
  embeddings and cosine similarity.

### Exact and lexical checks

```toml
[lint]
duplicate_min_words = 12
duplicate_shingle_words = 3
duplicate_similarity = 0.7
duplicate_containment = 0.9
```

Exact comparisons ignore case and whitespace but retain Markdown syntax, link
destinations and inline directive attributes. Code blocks and paragraphs containing
template actions are excluded from all three checks. The minimum length defaults
to 12 words; lower it to catch short repeated instructions.

Lexical checks tokenize paragraph Markdown into lowercase Unicode words and
numbers, splitting at punctuation. Each distinct sequence of three consecutive
words is a *shingle*. `duplicate_shingle_words` changes that size. Paragraphs with
fewer tokens than the shingle size have no lexical candidates.

For shingle sets A and B:

- **Jaccard**: `|A ∩ B| / |A ∪ B|`. `duplicate_similarity` sets its threshold.
  Shared sequences survive sentence reordering except at changed boundaries.
- **Containment**: `|A ∩ B| / min(|A|, |B|)`. `duplicate_containment` sets its
  threshold. This catches a shorter passage copied into a longer paragraph,
  regardless of which one occurs first. It measures coverage of the smaller
  shingle set, not proof of a contiguous substring.

Both thresholds default to `0` (off) and otherwise range up to `1`. The example
values are starting points to calibrate against your own skills. A score of `1`
means equal shingle sets or complete containment; it does not require identical
Markdown. Punctuation, negation, numbers and tool constraints still need review.

The previous paragraph edit-distance implementation has been removed.
`duplicate_similarity` now means Jaccard overlap; recalibrate existing values.

### Local semantic checks

```toml
[lint.semantic]
enabled = true
model_path = "models/potion-retrieval-32M"
threshold = 0.35 # illustrative; calibrate against accepted/rejected examples
```

`model_path` points to a local Model2Vec directory containing `tokenizer.json`,
`config.json`, and `model.safetensors`. Relative paths resolve against the
configuration file that declares them, including user configuration. Absolute
paths can point directly to an existing Hugging Face cache snapshot, such as the
Potion models used by Anax and Memex. Lint does not download models, call servers,
run subprocesses, or alter model files. No API key, Python, GPU, or native inference
runtime is needed. Model weights are loaded only when semantic comparisons remain
after exact and lexical checks.

Inference uses the pure Go [aikit Model2Vec implementation](https://github.com/townsendmerino/aikit/tree/v1.16.0/embed):
tokenize the paragraph, pool its learned token vectors, normalize, then compare
cosine similarity. Henia loads the model once per lint run and encodes each unique
eligible paragraph once. The model files stay outside the Henia binary.

Semantic checking defaults to disabled. Enabling it requires both `model_path`
and an explicit `threshold` greater than `0` and at most `1`; there is no universal
model-independent cutoff. Model load failures fail lint. Paragraphs with zero
embeddings emit a warning explaining that comparison is unavailable.

[Model2Vec](https://github.com/MinishLab/model2vec) provides fast static embeddings,
which can identify related wording with little lexical overlap, but lose token
order and do not establish equivalent instructions. Changed negation may score
higher than a valid paraphrase. These findings request review; they never rewrite
or remove content. See [the evaluation notes](paragraph-similarity.md) for measured
examples, model revision and test reproduction.

### Diagnostics and cost

For each paragraph, exact duplicates take precedence. Otherwise Henia reports the
highest qualifying Jaccard score; if none qualifies, it reports the highest
qualifying containment score. Semantic checks report the highest qualifying cosine
score only for paragraphs without an exact or lexical finding. A paragraph already
reported can still serve as a comparison for a later paragraph. Ties favor the
first occurrence in sorted scan order.

Findings include the matching location in `related`. Lexical and semantic JSON
findings include `similarity` and `method` (`jaccard`, `containment`, or `cosine`).
Lexical output includes up to five sorted normalized `shared_phrases`. Semantic
output includes the configured model path in `model`. Scores are measures, not
probabilities. Neither lexical nor semantic findings prove duplication.

`--disable similar-content` disables both lexical measures; `--disable
semantic-content` prevents model loading and inference. Exact matching remains
independently controlled by `duplicate-content`. Warnings fail only with
`--strict`, consistent with the other lint rules.

These checks maintain collection indexes in Go; Expr predicates operate on
individual nodes. A shingle index excludes lexically unrelated pairs. Worst-case
lexical work remains quadratic when many paragraphs share shingles. Semantic
comparison is exhaustive, requiring O(P² × D) comparisons and O(P × D) stored
vectors for P unique paragraphs and D embedding dimensions, plus model memory.
Cancellation is checked between encodes and during the comparison loop; it does
not interrupt an individual model load or encode.

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
