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
| `link` | `:url`, `:target` (skill named by `` `$name` ``, `` `henia show name` `` or `../name/SKILL.md`) |
| `directive` | `:name` and its attributes |
| `frontmatter` | its scalar keys |
| `_` | any type, any key |

Every type also takes `:text "exact"`, `:contains "case-insensitive"` and
`:matches "go regexp"`. `:level` takes a number or a range such as `1..3`.
Other values compare exactly; quote them with `"`, or leave single words bare.
A `/regexp/` value matches any key, as in `:url /^https:/` or `:title /^Phase/`.

A section holds its heading and everything up to the next heading of the same
or a higher level. Files other than Markdown hold blank-line-separated
paragraphs.

## Grammar

```
query    := pattern+                        ; rows of every pattern, united
pattern  := "(" type item* ")" quant? capture?
          | "[" pattern+ "]" quant? capture?  ; alternatives
item     := :key value | capture | pattern | ">" pattern | "."
          | "(" "not" pattern ")" | "(" "reaches" pattern ")"
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
- `(not P)` rejects a node with a descendant matching P.
- `(reaches P)` inside a `skill` matches the skills its links reach, transitively.
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
`--canonical` reads skills as authored, with lines in the files on disk.
Resources are never rendered.
