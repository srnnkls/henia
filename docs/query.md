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
henia query '(join (skill (link :target ?s :path ?p :anchor ?a) @l) (not (skill :id ?s (file :path ?p (section :id ?a)))))'
henia query '(join (skill :id ?x (paragraph :text ?t) @a) (skill :id (after ?x) (paragraph :text (near ?t 0.6)) @b))'
henia query '(join (paragraph :node ?n :text ?t) @a (paragraph :node (after ?n) :text (similar ?t 0.85)) @b)'
henia query '(file :path /\.md$/ (paragraph :words 100..) @p)'
henia query '(skill :id "gestalt" > (file :main true (heading :level 1..2) @h))'
henia query '(section > (heading) @title (code :lang "bash") @c)'
henia query '(skill :id "gestalt" (link :url /^https:/) @l)'
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
`:matches "go regexp"`, `:words` (its word count) and `:node` (an id in
document order). `:level` and `:words` take a number or a range such as `1..3`,
`80..` or `..20`.
Other values compare exactly; quote them with `"`, or leave single words bare.
A `/regexp/` value matches any key, as in `:url /^https:/` or `:title /^Phase/`.
A `?variable` value binds the key's value; every other use of the variable must
be equal, which joins the patterns that share it. `(not value)` negates a
value, as in `:lang (not "go")` or `:id (not ?x)`. Compared with a variable:

- `(after ?x)` sorts after it, so `:id (after ?x)` lists each pair once and
  `:node (after ?n)` never pairs a node with itself.
- `(near ?t 0.6)` shares at least that Jaccard share of three-word shingles,
  the measure `henia lint` uses for similar content.
- `(similar ?t 0.85)` has at least that cosine similarity under a local
  Model2Vec model: `--model DIR`, else `[lint.semantic] model_path`.

A section holds its heading and everything up to the next heading of the same
or a higher level. Files other than Markdown hold blank-line-separated
paragraphs.

## Grammar

```
query    := (pattern | join)+                ; rows of every term, united
join     := "(" "join" (pattern | "(" "not" pattern ")")+ ")"
pattern  := "(" type item* ")" quant? capture?
          | "[" pattern+ "]" quant? capture?  ; alternatives
value    := "string" | word | number | range | /regexp/ | ?variable
          | "(" "not" value ")" | "(" "after" ?variable ")"
          | "(" ("near" | "similar") ?variable threshold ")"
item     := :key value | capture | pattern | ">" pattern | "."
          | "(" "not" (pattern | relation) ")" | relation
relation := "(" ("reaches" | "inbound") pattern ")"
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
- `(not P)` rejects a node with a descendant matching P; `(not (reaches P))`
  and `(not (inbound P))` reject a skill with such a related skill.
- `(reaches P)` inside a `skill` matches the skills its links reach,
  transitively; `(inbound P)` matches the skills whose links reach it.
- `(join P...)` returns the rows of its patterns that agree on shared
  variables; a `(not P)` member drops rows for which P matches with the same
  variables. A join prints its captures only.
- A variable inside `(not P)` within a pattern refers to the same variable
  outside it, so `(not P)` rejects only matches that agree.
- A capture names a result column; a query without captures prints the matched
  node.

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
