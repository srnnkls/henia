# henia query

`henia query '<pattern>...'` matches S-expression patterns against every library
skill and its resources. It prints one row per match, with one line per capture.

Here is how many code blocks each language has in a library holding Tropos, largest first:

```console
$ henia query '(code :lang ?l) @c (group ?l (count @c ?n))' --sort '-?n' --limit 3
?l=bash  ?n=230
@c  230 nodes

?l=elisp  ?n=152
@c  152 nodes

?l=python  ?n=95
@c  95 nodes
henia query: 3 of 17 rows shown
```

An ungrouped capture prints the address `henia show` reads, the lines it spans,
its type and its first line:

```console
$ henia query '(skill :id "gestalt" (heading :level 2) @h)' --limit 2
@h  gestalt#auto-detect-rules  L7  heading  Auto-Detect Rules
@h  gestalt#subagent-orientation  L16  heading  Subagent orientation
henia query: 2 of 17 rows shown
```

## Examples

```
henia query '(skill :id "gestalt" (heading) @h)'
henia query '(skill :id "gestalt" (paragraph)? @prev . (paragraph :contains "cozo") @hit . (paragraph)? @next)'
henia query '(section :id "usage" (code :lang "bash") @c)'
henia query '(skill (link :target "gestalt")) @s'
henia query '(skill :id "gestalt" (reaches (skill) @t))'
henia query '(skill (not (inbound (skill)))) @orphan'
henia query '(link :anchor /./ (not (to (section)))) @broken'
henia query '(section :id "map-vs-analyze" (from (link) @l))'
henia query '(skill :id ?x (paragraph :text ?t) @a) (skill :id (after ?x) (paragraph :text (near ?t 0.6)) @b)'
henia query '(paragraph :node ?n :text ?t) @a (paragraph :node (after ?n) :text (similar ?t 0.85)) @b'
henia query '(file :path /\.md$/ (paragraph :words 100..) @p)'
henia query '(skill :id "gestalt" > (file :main true (heading :level 1..2) @h))'
henia query '(section > (heading) @title (code :lang "bash") @c)'
henia query '[(code :lang "toml") (code :lang "yaml")] @c'
henia query '(code :lang ?l) @c (group ?l)'
henia query '(skill (code :lang "bash")? @c) @s (group @s (count @c ?n))'
henia query '(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)? (group @t (count @l ?n))'
henia query '(skill (code)? @c) @s (group @s (count @c ?n (>= 3)) (sum :lines @c ?l))' --sort '-?n'
```

## Types

| Type | Keys |
|---|---|
| `skill` | `:id` (name), `:package` |
| `file` | `:path` (relative to the skill), `:main` (`true` for SKILL.md) |
| `section` | `:id` (heading anchor, as in `henia show skill#id`), `:title`, `:level` |
| `heading` | `:level` |
| `paragraph`, `item`, `quote`, `table` | |
| `list` | `:ordered` |
| `code` | `:lang` |
| `link` | `:url`; for links into a skill also `:target` (skill name), `:path` (file in it) and `:anchor` (section id), from `` `$name` ``, `` `henia show name/path#anchor` ``, relative paths and `#anchor` |
| `directive` | `:name` and its attributes |
| `frontmatter` | its scalar keys |
| `_` | any type, any key |

A section holds its heading and everything up to the next heading of the same
or a higher level. Files other than Markdown hold paragraphs separated by blank
lines.

Every type also takes these keys:

- `:text "exact"`, `:contains "case-insensitive"` and `:matches "go regexp"`;
- `:words` and `:chars`, word and character counts, and `:lines`, the line count;
- `:norm`, the text lower-cased with whitespace collapsed;
- `:node`, an id in document order;
- `:position`, the file path, then the offset.

On a `file`, `:words`, `:chars` and `:lines` count the text the query reads,
frontmatter excluded. On a `skill` they sum over its files. `:text` stays empty
on both.

## Values

- `:level`, `:words`, `:chars` and `:lines` take a number or a range: `1..3`,
  `80..` or `..20`.
- Any numeric value takes `(> n)`, `(>= n)`, `(< n)` or `(<= n)`; a date takes
  `(older days)`.
- Other values compare exactly. Quote them with `"`, or leave single words bare.
- A `/regexp/` matches any key, as in `:url /^https:/` or `:title /^Phase/`.
- `(not value)` negates, as in `:lang (not "go")` or `:id (not ?x)`.
- A `?variable` binds the key's value. Every other use of it must be equal,
  which joins the patterns that share it.

Compared with a variable:

- `(after ?x)` and `(before ?x)` sort after or before it. `:id (after ?x)` lists
  each pair once, and `:node (after ?n)` never pairs a node with itself.
- `(contains ?x)` holds it as a substring, ignoring case.
- `(covers ?x)` is it or a dotted prefix of it: `code.style` covers
  `code.style.go`.
- `(near ?t 0.6)` shares at least that Jaccard share of three-word shingles.
- `(overlap ?t 0.9)` shares at least that share of the shorter text's shingles.
- `(similar ?t 0.85)` has at least that cosine similarity under a local
  Model2Vec model: `--model DIR`, else `[lint.semantic] model_path`.

`near`, `overlap` and `similar` take an optional score variable, as in
`(near ?t 0.6 ?score)`. In lint modules, numbers and thresholds may be
`$params`.

## Grammar

```
query    := (pattern | "(" "not" pattern ")")+ group?  ; patterns join on shared variables
pattern  := "(" type item* ")" quant? capture?
          | "[" pattern+ "]" quant? capture?  ; alternatives
value    := "string" | word | number | range | /regexp/ | ?variable
          | $param | "(" "not" value ")" | "(" comparison (number | $param) ")"
          | "(" ("after" | "before" | "contains" | "covers") ?variable ")"
          | "(" ("near" | "overlap" | "similar") ?variable threshold ?score? ")"
comparison := ">" | ">=" | "<" | "<=" | "older"
item     := :key value | capture | pattern | ">" pattern | "."
          | "(" "not" (pattern | relation) ")" | relation
relation := "(" ("reaches" | "inbound" | "to" | "from") pattern ")"
quant    := "?" | "*" | "+"  ; only "?" on a top-level pattern
capture  := "@" name
group    := "(" "group" key* aggregate* ")"
key      := ?variable | @capture
aggregate := "(" "count" (@capture | ?variable) ?out? filter? ")"
          | "(" "sum" measure @capture ?out? filter? ")"
          | "(" ("min" | "max") (measure @capture | :level @capture | ?variable) ?out? filter? ")"
measure  := ":words" | ":lines" | ":chars"
filter   := "(" (">" | ">=" | "<" | "<=") (number | $param) ")"
comment  := ";" to the end of the line
```

## Structure

- A nested pattern matches a descendant at any depth; `>` requires a direct
  child. A section's own heading is `> (heading)`, while `(heading)` also
  reaches the headings of its subsections.
- `.` between two nested patterns makes them adjacent siblings. Before the first
  or after the last, it pins the first or last child. Unanchored nested
  patterns match in document order.
- `?`, `*` and `+` make a sibling optional or repeated. An optional sibling or
  relation target falls back per enclosing row: when no candidate unifies with
  it, the row stays once with its captures empty.
  `(skill :id "limen" (from (link)? @l))` keeps a skill nothing links to.
- `[P1 P2]` matches either. Every alternative binds the same variables.
- `(not P)` rejects a node with a descendant matching P, or with a related node
  for `(not (to P))` and the like.
- `(reaches P)` inside a `skill` matches the skills its links reach,
  transitively. `(inbound P)` matches the skills whose links reach it.
- `(to P)` inside a `link` matches what the link names: the section of an
  `:anchor`, else the file of a `:path`, else the skill. A dangling link
  matches nothing. `(from P)` matches the links that name the node.

## Joins

Patterns side by side match together, joined on the variables they share, and
print their captures.

- A top-level `P?` is a left join. The required patterns join first; then each
  row joins every match of `P`, or stays once with `P`'s captures empty. A
  query needs at least one required pattern.
- A top-level `(not P)` drops the rows for which P matches with the same
  variables. It must share a variable bound by a required pattern. Inside
  `(not P)`, a variable bound outside joins; one bound only inside stays local.
- A variable bound in an optional pattern may recur only after a required
  binder has matched, and there the optional mention compares. The required
  binder is one of these:
  - an attribute of the pattern that holds the optional one;
  - a required sibling written before it;
  - for a relation target, any required sibling or earlier relation;
  - for a top-level `P?`, any required pattern.

## Grouping

`(group ...)` ends a query. It turns the rows into one row per distinct
combination of keys:

```console
$ henia query '(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)? (group @t (count @l ?n))' --sort '-?n' --limit 2
?n=50
@t  review  skill
@l  50 nodes

?n=29
@t  peer  skill
@l  29 nodes
henia query: 2 of 26 rows shown
```

- A `?variable` key groups by its value. A `@capture` key groups by its nodes,
  so one node per `@s` gives one row per skill. Rows where the key is unbound
  or empty form one group.
- Without keys, the whole result is one group, even when nothing matches. With
  keys, no match means no rows. Without aggregates, the rows are the distinct
  keys.
- `(count @c)` counts the distinct nodes of `@c` in the group, and
  `(count ?x)` the distinct values of `?x`. Empty captures count 0, so
  `(skill (code)? @c) @s` reports skills without code as 0.
- `sum`, `min` and `max` read `:words`, `:lines` or `:chars` from the distinct
  nodes of a capture; `min` and `max` also read `:level`, or the numeric values
  of a `?variable`. With no operand, the result is unbound.
- A filter such as `(>= 3)` or `(< $min-inbound)` drops the groups that fail
  it, and those whose result is unbound.
- `?out` names the result. It must be fresh: no pattern, key or other aggregate
  uses it. Other variables are dropped.

## Rejected queries

The reader rejects:

- a variable that appears once, since it would match anything;
- a variable that is compared but never bound, or bound only in an optional
  pattern and used again without an earlier required binder;
- top-level patterns that share no variable, since every combination would
  match, and a top-level `(not P)` that shares none, since it would drop every
  row or none;
- alternatives that bind different variables, and captures inside `(not ...)`;
- `*` or `+` on a top-level pattern, and a group key captured under `*` or `+`;
- `(group ...)` inside a pattern, `sum` over a variable, and an aggregate `:key`
  other than those above;
- a `--sort` name that no pattern binds or that `(group ...)` drops.

## Output

Each capture prints `@name  address  lines  type  first line`. The address
reads the enclosing section with `henia show`, as in `skill/path#section  L12-14`.

- `--text` prints whole nodes.
- `--json` prints rows keyed by capture.
- `--count` prints the number of rows.
- `--limit N` caps the rows.

A grouped row prints its keys and named aggregates on one line, as
`?l=bash  ?n=41`. Each node of a key capture follows as above, and every other
capture prints as `@c  N nodes`, or `N matches` for an unnamed pattern.
`--text` prints every node of every capture. `--json` adds each variable of a
row under a `"?name"` key, aggregates as numbers and the rest as strings.
`--count` counts the groups.

`--sort KEY` orders the rows by `?var` or `@capture.key`, descending with a
leading `-`: `--sort '-?n'` or `--sort @s.words`. Quote it, since shells expand
`?`.

- A capture holding several nodes sorts by its first.
- Rows without a value sort last.
- Values compare as numbers when all of them are numbers, else as strings.
  Equal rows keep their order.
- Ungrouped rows print the sorted variable on their variable line.
- `--sort`, `--limit` and `--count` apply after grouping.

## Rendering

Skills are read as rendered for the caller, as `henia show` prints them:
templates are resolved, references take the harness's syntax, and lines count
from the top of the rendered text. `--harness` picks another harness.
`--canonical` reads skills as authored, with lines as in the files on disk;
there, a template action that opens a code fence or heading in one branch leaves
the Markdown after it unstructured. Resources are never rendered.
