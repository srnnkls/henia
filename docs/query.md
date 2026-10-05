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
```

## Types

| Type | Keys |
|---|---|
| `skill` | `:id` (name), `:source` |
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

Every type also takes `:text "exact"`, `:contains "case-insensitive"`,
`:matches "go regexp"`, and the derived keys:

- `:words` and `:chars`: word and character counts;
- `:lines`: line count;
- `:norm`: lower-cased text with collapsed whitespace;
- `:node`: an id in document order;
- `:position`: file path, then offset.

`:level`, `:words`, `:chars` and `:lines` take a number or a range such as
`1..3`, `80..` or `..20`. Any numeric value also takes `(> n)`, `(>= n)`,
`(< n)` or `(<= n)`, and a date takes `(older days)`.
Other values compare exactly; quote them with `"`, or leave single words bare.
A `/regexp/` value matches any key, as in `:url /^https:/` or `:title /^Phase/`.
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
query    := (pattern | "(" "not" pattern ")")+  ; patterns join on shared variables
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
quant    := "?" | "*" | "+"
capture  := "@" name
comment  := ";" to the end of the line
```

- A nested pattern matches a descendant at any depth; `>` requires a direct child.
  A section's own heading is `> (heading)`; `(heading)` also reaches the
  headings of its subsections.
- `.` between two nested patterns makes them adjacent siblings; before the first
  or after the last it pins the first or last child. Unanchored nested patterns
  match in document order.
- `?`, `*` and `+` make a sibling optional or repeated.
- `[P1 P2]` matches either; every alternative binds the same variables.
- `(not P)` rejects a node with a descendant matching P, or with a related node
  for `(not (to P))` and the like.
- `(reaches P)` inside a `skill` matches the skills its links reach,
  transitively; `(inbound P)` matches the skills whose links reach it.
- `(to P)` inside a `link` matches what it names: the section of an `:anchor`,
  else the file of a `:path`, else the skill. A dangling link matches nothing.
  `(from P)` matches the links that name the node.

Patterns side by side match together, joined on the variables they share, and
print their captures. A top-level `(not P)` drops the rows for which P matches
with the same variables. A variable inside `(not P)` that is bound outside it
joins; one bound only inside stays local. The reader rejects:

- a variable that appears once, since it would match anything;
- a variable that is compared but never bound, or bound only in an optional
  pattern and used again;
- top-level patterns that share no variable, since every combination would
  match, and a top-level `(not P)` that shares none, since it would drop every
  row or none;
- alternatives that bind different variables, and captures inside `(not ...)`.

## Output

Each capture prints `@name  address  lines  type  first line`, where the address
reads the enclosing section with `henia show`: `skill/path#section  L12-14`.
`--text` prints whole nodes, `--json` prints rows keyed by capture,
`--count` prints the number of rows, and `--limit N` caps them.

Skills are read as rendered for the caller, as `henia show` prints them:
templates are resolved, references take the harness's syntax, and lines count
from the top of the rendered text. `--harness` picks another harness;
`--canonical` reads skills as authored, with lines in the files on disk; there a
template action that opens a code fence or heading in one branch leaves the
Markdown after it unstructured.
Resources are never rendered.
