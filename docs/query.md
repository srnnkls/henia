# henia query

`henia query '<pattern>...'` matches S-expression patterns against every library
skill and its resources, and prints one row per match with a line per capture.

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
henia query '(code :inline true :matches /^\$(?<name>[a-z-]+)$/) @c (skill :id ?name) @s'
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
| `code` | `:lang`; `:inline` (`true` for a code span, `false` for a block) and, on a span, `:ticks` (its backtick count) |
| `link` | `:url`; for links into a skill also `:target` (skill name), `:path` (file in it) and `:anchor` (section id), from `` `$name` ``, `` `henia show name/path#anchor` ``, relative paths and `#anchor` |
| `directive` | `:name` and its attributes |
| `frontmatter` | its scalar keys |
| `_` | any type, any key |

Every type also takes `:text "exact"`, `:contains "case-insensitive"`,
`:matches "go regexp"`, and the derived keys:

- `:words` and `:chars`: word and character counts;
- `:lines`: line count;
- `:norm`: lower-cased text with collapsed whitespace;
- `:node`: an id in document order;
- `:position`: file path, then offset.

On a `file`, `:words`, `:chars` and `:lines` count the text the query reads for
it, frontmatter excluded; on a `skill`, they sum over its files. `:text` on
both stays empty.

`:level`, `:words`, `:chars` and `:lines` take a number or a range such as
`1..3`, `80..` or `..20`. Any numeric value also takes `(> n)`, `(>= n)`,
`(< n)` or `(<= n)`, and a date takes `(older days)`.
Other values compare exactly; quote them with `"`, or leave single words bare.
A `/regexp/` value matches any key, as in `:url /^https:/` or `:title /^Phase/`.
A named group binds the variable of its name, as `:matches /^\$(?<name>[a-z-]+)$/`
binds `?name`. `:text` compares trimmed text, `:matches` the text as written.
A `?variable` value binds the key's value; every other use of the variable must
be equal, which joins the patterns that share it. `(not value)` negates a
value, as in `:lang (not "go")` or `:id (not ?x)`. Compared with a variable:

- `(after ?x)` and `(before ?x)` sort after or before it, so `:id (after ?x)`
  lists each pair once and `:node (after ?n)` never pairs a node with itself.
- `(contains ?x)` holds it as a substring, ignoring case.
- `(covers ?x)` is it or a dotted prefix of it, as `code.style` covers
  `code.style.go`.
- `(near ?t 0.6)` shares at least that Jaccard share of three-word shingles.
- `(overlap ?t 0.9)` shares at least that share of the shorter text's shingles.
- `(similar ?t 0.85)` has at least that cosine similarity under a local
  Model2Vec model: `--model DIR`, else `[lint.semantic] model_path`.

`near`, `overlap` and `similar` take an optional score variable,
`(near ?t 0.6 ?score)`. In lint modules, numbers and thresholds may be
`$params`.

A section holds its heading and everything up to the next heading of the same
or a higher level. Files other than Markdown hold blank-line-separated
paragraphs.

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

- A nested pattern matches a descendant at any depth; `>` requires a direct child.
  A section's own heading is `> (heading)`; `(heading)` also reaches the
  headings of its subsections.
- `.` between two nested patterns makes them adjacent siblings; before the first
  or after the last it pins the first or last child. Unanchored nested patterns
  match in document order.
- `?`, `*` and `+` make a sibling optional or repeated. An optional sibling or
  relation target falls back per enclosing row: the row stays once, with its
  captures empty, when no candidate unifies with it, as
  `(skill :id "limen" (from (link)? @l))` keeps a skill nothing links to.
- `[P1 P2]` matches either; every alternative binds the same variables.
- `(not P)` rejects a node with a descendant matching P, or with a related node
  for `(not (to P))` and the like.
- `(reaches P)` inside a `skill` matches the skills its links reach,
  transitively; `(inbound P)` matches the skills whose links reach it.
- `(to P)` inside a `link` matches what it names: the section of an `:anchor`,
  else the file of a `:path`, else the skill. A dangling link matches nothing.
  `(from P)` matches the links that name the node.

Patterns side by side match together, joined on the variables they share, and
print their captures. A top-level `P?` is a left join: the required patterns
join first, then each row joins every match of `P`, or stays once with `P`'s
captures empty. A query needs a required pattern. A top-level `(not P)` drops
the rows for which P matches with the same variables, and must share a variable
bound by a required pattern. A variable inside `(not P)` that is bound outside
it joins; one bound only inside stays local.

A variable bound in an optional pattern may recur only when a required binder
is matched before it: an attribute of the pattern that holds the optional one,
a required sibling written before it, for a relation target any required
sibling or earlier relation, and for a top-level `P?` any required pattern.
There the optional mention compares.

`(group ...)` ends a query and turns its rows into one row per distinct
combination of keys:

- A `?variable` key groups by its value; a `@capture` key by its nodes, so one
  node per `@s` gives one row per skill. Rows with the key unbound or empty
  form one group.
- Without keys, the whole result is one group, also when nothing matches;
  with keys, nothing matching gives no rows. Without aggregates, the rows are
  the distinct keys.
- `(count @c)` counts the distinct nodes of `@c` in the group, and
  `(count ?x)` the distinct values of `?x`; empty captures count 0, so
  `(skill (code)? @c) @s` gives zeros.
- `sum`, `min` and `max` read `:words`, `:lines` or `:chars` (`min` and `max`
  also `:level`) from the distinct nodes of a capture; `min` and `max` also
  take the numeric values of a `?variable`. With no operand the result is
  unbound.
- A filter such as `(>= 3)` or `(< $min-inbound)` drops the groups that fail
  it, and those whose result is unbound.
- `?out` names the result. It must be fresh: no pattern, key or other
  aggregate uses it. Other variables are dropped.

The reader rejects:

- a variable that appears once, since it would match anything;
- a variable that is compared but never bound, or bound only in an optional
  pattern and used again without an earlier required binder;
- top-level patterns that share no variable, since every combination would
  match, and a top-level `(not P)` that shares none, since it would drop every
  row or none;
- alternatives that bind different variables, and captures inside `(not ...)`;
- `*` or `+` on a top-level pattern, a group key captured under `*` or `+`,
  `(group ...)` inside a pattern, `sum` over a variable, an aggregate `:key`
  other than those above, and a `--sort` name that no pattern binds or that
  `(group ...)` drops.

## Output

Each capture prints `@name  address  lines  type  first line`, where the address
reads the enclosing section with `henia show`: `skill/path#section  L12-14`.
`--text` prints whole nodes, `--json` prints rows keyed by capture,
`--count` prints the number of rows, and `--limit N` caps them.

A grouped row prints its keys and named aggregates on one line, as
`?l=bash  ?n=41`, then each node of a key capture as above, and every other
capture as `@c  N nodes`, or `N matches` for an unnamed pattern. `--text` prints
every node of every capture. `--json` adds each variable of a row under a
`"?name"` key, aggregates as numbers and the rest as strings. `--count` counts
the groups.

`--sort KEY` orders the rows by `?var` or `@capture.key`, descending with a
leading `-`, as in `--sort '-?n'` or `--sort @s.words`. A capture holding several
nodes sorts by its first. Rows without a value sort last. Values compare as
numbers when all of them are numbers, else as strings, and equal rows keep
their order. Ungrouped rows print the sorted variable on their variable line.
`--sort`, `--limit` and `--count` apply after grouping.

Skills are read as rendered for the caller, as `henia show` prints them:
templates are resolved, references take the harness's syntax, and lines count
from the top of the rendered text. `--harness` picks another harness;
`--canonical` reads skills as authored, with lines in the files on disk; there a
template action that opens a code fence or heading in one branch leaves the
Markdown after it unstructured.
Resources are never rendered.
