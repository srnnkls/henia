---
created: 2026-10-05
status: active
issue_type: Feature
---

# query-aggregation

## Goal

`henia query` answers counting and ranking questions without piping `--json` into jq: distinct values, counts per group including zero, sums and extremes over `:words`, `:lines` and `:chars`, filters on those aggregates, and sorted output. Lint modules use the same forms with `$params` thresholds.

## Context

Observed in tropos with the current build:

- `(code :lang ?l) @c` is rejected: "?l appears only once, so it matches anything".
- `--count` prints the row total; `--json` prints captures only, never `Row.Vars`.
- `(skill (code :lang "bash")? @c) @s` yields one capture-less row per skill without a match, so nested `?` already carries zero groups.
- `?` on a relation target is ignored: `(skill :id "limen" (from (link)? @l))` returns 0 rows.
- `(from P)` on a skill reaches only links that name the bare skill: 3 for `peer`, against 34 for `(link :target "peer")`.
- All 29 skills and all 215 files match `:words 0`.

### Key Files

| File | Lines | Description |
|------|-------|-------------|
| `internal/pattern/read.go` | 13-45, 420-440, 546-557 | `Query`/`Pattern` types, quantifier parsing (top-level rejection), relation reader |
| `internal/pattern/check.go` | 40-110 | Mention walk, single-use check, optional-binding check, never-bound check |
| `internal/pattern/match.go` | 30-50, 156-200, 393-412, 449-531, 744-766 | `Row`/`Cell`/`binding`, `join`, relation loop, `chain` (empty row at 528), derived keys |
| `internal/markup/tree.go` | 110-117, 273-295 | File element, text filled from source span except for `file` |
| `internal/library/corpus.go` | 49, 78, 140-160, 228 | Skill and file elements of the corpus |
| `internal/cli/query.go` | 80-100 | `--json`, `--count`, `--limit` output |
| `internal/lint/module.go` | 290-330, 385-400 | Row to diagnostic: `:at`, `:score`, `{?var}` messages |
| `docs/query.md` | all | Grammar reference printed by `henia query --grammar` (embedded via `docs.go`) |
| `docs/lint-rules.md` | all | Lint rule authoring reference |

### Architecture Decisions

#### AD-1: One top-level `(group …)` clause

Decision: grouping is a single optional clause after the patterns of a query, `(group key* aggregate*)`. Keys are explicit `?var` or `@capture`; with no aggregates it yields distinct keys. It runs over the joined bindings after top-level `(not …)` and before the capture-key row deduplication of `Query.Run`, so one captured node contributing several key values yields several groups. `--sort`, `--limit` and `--count` apply to the grouped rows.

With no keys, the whole result is one group, also when it is empty: `count` gives 0 and `sum`, `min`, `max` stay unbound. With keys, an empty result gives no rows. Rows whose key capture is empty or whose key variable is unbound form one group with that key empty.

Alternatives:
- `:group`/`:count` keys on captures: spreads one projection across patterns; keys and aggregates can no longer be read in one place.
- Soufflé-style aggregates nested in a pattern, `(skill (count (code) ?n))`: gives zero per node without `?`, but duplicates what `?` plus `(group …)` already express.

#### AD-2: Aggregates and their null and duplicate semantics

| Form | Value |
|---|---|
| `(count @c ?n)` | distinct non-null nodes captured by `@c` in the group |
| `(count ?x ?n)` | distinct bound values of `?x` |
| `(sum :key @c ?n)` | sum of `:key` (`:words`, `:lines`, `:chars`) over distinct non-null nodes of `@c` |
| `(min :key @c ?n)`, `(max :key @c ?n)` | extreme of `:key` over those nodes |
| `(min ?x ?n)`, `(max ?x ?n)` | extreme of numeric values of `?x` |

`:key` is one of `:words`, `:lines`, `:chars`, and for `min`/`max` also `:level`. `sum` takes captures only, since equal values from distinct nodes must not collapse. `sum`, `min` and `max` skip nodes without the key and values that do not parse as numbers; when every operand is skipped, or the group holds only empty captures, the result is unbound. `count` over only empty captures is 0. `?n` may be omitted when the aggregate only filters.

An output variable is fresh: no pattern binds it, it is not a key, and no other aggregate of the clause names it. The reader rejects every collision.

#### AD-3: Filters inside the aggregate

An aggregate takes one trailing comparison, `(>, >=, <, <=)` with a number or `$param`: `(count @c ?n (>= 3))` or `(count @c (< $min-inbound))`. `older` is rejected. A group failing any filter is dropped; a filter on an unbound result fails.

#### AD-4: Zero groups come from optional patterns

- `?` and `*` on a relation target add one empty row when no related node matches, as `chain` does for optional siblings.
- The empty row of an optional sibling or relation target is decided per enclosing row: a row keeps itself once, with the optional captures empty, when no candidate unifies with it. Today `chain` emits its fallback only when no candidate exists at all, so `(skill :id ?s (link :target ?s)? @l) @t` drops a skill whose links all name other skills.
- A query needs at least one required top-level pattern, and a top-level `(not …)` must share a variable bound by a required one; the reader rejects both otherwise.
- A top-level pattern may take `?` and acts as a left join: required members join first; each row then joins every match of the optional member, or stays once with its captures empty. `*` and `+` stay rejected at the top level.
- The optional-binding error applies unless a required binder of the variable is evaluated before the optional (FR-2); then the optional mention compares. Ancestor attributes do not count: a nested pattern is matched without its ancestors' bindings.

#### AD-5: Ordering lives in `--sort`, outside `(group …)`

`--sort KEY` takes `?var` or `@capture.key` (the `{@x.key}` form of lint messages), descending with a leading `-`. A capture holding several nodes sorts by its first node in document order. Rows without a value sort last in both directions. Values compare as numbers when every present value parses as one, else as strings. The sort is stable. A name that no pattern binds, or that `(group …)` drops, is a reader error. Variables named only by `--sort` count as used for the single-use check.

#### AD-6: Output

- Text: per group, the bound key and aggregate variables on one line (`?l=bash  ?n=41`), then each key capture in the existing format, one address line per node, and each other capture as one line `@c  N nodes`. `--text` prints every node of every capture with its whole text. Ungrouped rows add the variable line only for variables named by `--sort`.
- `--json`: captures as today, plus every bound variable under a `"?name"` key. Aggregate outputs are JSON numbers, every other variable a string; `Row` gains the set of its aggregate output names to carry that distinction.
- `--count` prints the number of rows after grouping, i.e. groups.
- Every non-key capture, aggregate inputs included, collects the distinct non-empty nodes of its group into one cell in document order; an all-empty capture stays empty. Lint `:at` and `:related` on such a cell take its first node, as they do today. Unaggregated non-key variables are dropped.

### Constraints

- Grouped rows stay `pattern.Row`, so lint `:at`, `:related`, `:score` and `{?var}`/`{$param}` messages need no new plumbing.
- `:text` on `skill` and `file` stays empty; only the derived counts change.
- `maxBindings` bounds left joins as it bounds inner joins.

## Requirements

### Functional Requirements

- FR-1: `?`/`*` on a `reaches`, `inbound`, `to` or `from` target yields one empty row when nothing related unifies with the enclosing row; optional siblings follow the same per-row rule.
- FR-2: A variable bound in an optional pattern may recur when a required binder is evaluated before the optional: an attribute of the pattern that holds the optional, a required chain of that pattern written before it (with its required descendants), or, for an optional relation target, any required chain or earlier required relation of that pattern. For a top-level optional, any required member. Every other recurrence stays a reader error.
- FR-3: A top-level `P?` keeps every row of the required members, joined with each match of `P` or once with `P`'s captures empty.
- FR-4: `:words`, `:chars` and `:lines` on a `file` count the text the query reads for it: the rendered body by default, the authored body with `--canonical` and in lint, frontmatter excluded; a resource counts its whole text. On a `skill`, each is the sum over its files.
- FR-5: `(group key* aggregate*)` with the aggregates and filters of AD-2 and AD-3; key and output variables satisfy the single-use check.
- FR-6: `--sort` per AD-5.
- FR-7: Text, `--json` and `--count` output per AD-6.
- FR-8: Lint rules may end with `(group …)`; aggregate filters and messages take `$params` and `{?var}`.

### Technical Requirements

- TR-1: Reader errors with hints for: a key capture on a `*`/`+` pattern, a capture or aggregate inside `(not …)`, `sum` over a variable, an unknown `:key`, `--sort` on an unbound name, and `*`/`+` at the top level.
- TR-2: `docs/query.md` grammar and prose cover every new form; `henia query --grammar` prints it.
- TR-3: `henia lint test` passes for all std modules.

## Acceptance Criteria

- [ ] Given tropos, when running `henia q '(code :lang ?l) @c (group ?l (count @c ?n))' --sort -?n`, then one row per distinct language prints with its block count, largest first.
- [ ] Given tropos, when running `henia q '(skill (code :lang "bash")? @c) @s (group @s (count @c ?n))' --count`, then it prints 29.
- [ ] Given tropos, when running `henia q '(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)? (group @t (count @l ?n))' --count`, then it prints 29.
- [ ] Given tropos, when running the same query with `--json`, then the `peer` object holds `"?n": 29`, the count of `(skill :id (not "peer") (link :target "peer") @l)`.
- [ ] Given a fixture skill whose only link names another skill, when running `henia q '(skill :id ?s (link :target ?s)? @l) @t'`, then that skill prints once with `@l` empty.
- [ ] Given `(skill (code :lang ?l)) @s (group ?l (count @s ?n))` over a fixture skill holding `go` and `bash` blocks, then both languages print with `?n=1`.
- [ ] Given `(code :lang "cobol") @c (group (count @c ?n))`, then one row prints with `?n=0`.
- [ ] Given tropos, when running `henia q '(skill :id "limen" (from (link)? @l))'`, then it prints a row with `@l` empty (rows dedupe on their captures, so both `limen` elements print as one row).
- [ ] Given tropos, when running `henia q '(skill :words 0 (file)) @s' --count`, then it prints 0 (skills without files, such as the dependency duplicates in Gotchas, count 0 by FR-4).
- [ ] Given tropos, when running `henia q '(skill (section)? @x) @s (group @s (count @x ?n))' --sort -?n --json`, then each object holds `"s"` and a numeric `"?n"`, in descending order.
- [ ] Given a lint module rule ending in `(group @t (count @l ?n (< $min-inbound))))` with `min-inbound: 1`, when linting a library with one unreferenced skill, then exactly that skill is reported with `{?n}` rendered as 0.

## Dependency Graph

> Machine-readable: [dependencies.yaml](dependencies.yaml)

```
QAG-001 per-row optional fallback, relation-target optionals
└── QAG-002 optional-binding check
    └── QAG-003 top-level left join
        └── QAG-004 skill/file derived counts
            └── QAG-005 (group …) clause + output
                └── QAG-006 --sort
                    └── QAG-007 lint integration + docs
```

Each task tests behavior its predecessor makes reachable, so batches are serial.

## Non-Goals

- Changing `(from P)` on a skill to include links into its files and sections.
- Nested per-node aggregates.
- New std lint rules; `unreferenced-skill` appears as a test and doc example only.
- Link-destination stripping for `similar-content` (separate change).

## Verification

- `go test ./...`
- `henia lint test`
- The acceptance commands against `~/projects/tropos`.

## Gotchas & Learnings

- Follow-up outside this scope: the corpus attaches both copies' files to the global element when a dependency and a global install share a skill id; fix ownership in internal/library/corpus.go.

- tropos resolves two `limen` skill elements, so per-skill counts group by the `@skill` node, not by `:id`; grouping on `?id` merges them.
- When a dependency package and a global install provide the same skill id, the corpus attaches both copies' files to the global skill element; the dependency element has no files and counts `:words 0` (gestalt, limen, loqui in tropos).
- `(from P)` on a skill misses links into its sections and files; inbound counts join on `(link :target ?s)` instead.
