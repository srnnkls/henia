# Components

Henia composes skills from typed components: directives in the skill, declared
once with a schema, expanded as trees, validated before anything renders.

## Principles

1. Transform the tree, not the text. A skill body is parsed once; every later
   step rewrites nodes, and every node keeps the source position it came from.
   (unified, Markdoc, MyST)
2. One composition mechanism. A directive is the only way a skill pulls in
   shared structure. Go templates generate text inside a component; they never
   compose skills. (Hugo's shortcode history shows what text-level composition
   costs.)
3. Declared, closed, typed. Every directive name has a declaration with typed
   attributes and a body kind. An undeclared name is an error, never a silent
   drop. (MyST specs, Markdoc schemas, Docusaurus keywords)
4. Check calls without executing, and outcomes where they exist. A call is
   validated against its declaration from the source; what a call produces is
   checked on the rendered tree. (Markdoc `validate`, rustc's lint stages)

## Syntax

The CommonMark generic directive syntax, as micromark implements it:

| Form | Syntax | Node | Children |
|---|---|---|---|
| text | `:name[label]{attrs}` | inline | label, inline |
| leaf | `::name[label]{attrs}` | block | label, inline |
| container | `:::name{attrs}` … `:::` | block | blocks |

Attributes: `key=value`, `key="quoted value"`, bare `key` (boolean true),
`#id`, `.class`. A repeated key is an error, not last-wins. An unclosed
container is an error, not a run to the end of its parent.

Nesting still uses longer outer fences. `henia fmt` rewrites fence lengths from
the tree, so authors write `:::` everywhere and never count colons.

Skill bodies contain no Go template actions. Frontmatter strings stay templated.

## Declarations

A component is a Markdown file whose frontmatter is its schema and whose body is
its template: `.henia/components/<name>.md`, shadowing
`$XDG_CONFIG_HOME/henia/components/<name>.md`. A dependency's skills resolve
against the dependency's own components.

```markdown
---
form: container
doc: Routes the skill's arguments.
attributes:
  ordered:
    type: boolean
    doc: Rows apply in order; the first match wins.
  protocol:
    type: boolean
    doc: Links the dispatch protocol.
body:
  kind: markdown
  children: [table, paragraph]
---
## Routes

{{if .ordered}}Apply the first row that matches `$ARGUMENTS`:

{{end}}::body
{{if .protocol}}
> **Protocol:** [../dispatch/protocol.md](../dispatch/protocol.md)
{{end}}
```

| Field | Values |
|---|---|
| `form` | `container`, `leaf`, `text` |
| `attributes.<name>.type` | `string`, `boolean`, `number`, `list`, `enum`, `slot`, `skill`, `ref` |
| `attributes.<name>` | also `required`, `default`, `values` (enum), `matches` (regexp), `doc` |
| `body.kind` | `markdown` (a tree), `text` (raw, never scanned for directives), `none` |
| `body.children` | allowed child node types; directive children by name |
| `body.required` | the body may not be empty |
| `render` | `expand` (default), or `keep` for a directive the harness renders itself |

A `list` attribute is whitespace-separated: `slots="git.commits review.criteria"`.
`slot`, `skill` and `ref` are typed strings lint resolves: a declared slot, a
library skill, a `skill#section` address.

A declaration with `render: keep` and no body template declares a passthrough
directive such as `instruction`: its attributes are checked, and the harness's
`directives` setting renders it.

The template runs as Go `text/template` with `missingkey=error`. Its data is the
skill's values plus every declared attribute, typed and always present: an
omitted attribute holds its `default`, or its type's zero value. Only an
undeclared field fails.

A `markdown` body is a tree: `::body` marks where it goes, the template's
output is parsed, and the body's subtree is grafted in place of `::body` with
its original positions. A `text` body is a string in `.body`, so a template can
place it anywhere, a code fence included.

## Built-in components

| Component | Purpose |
|---|---|
| `:::head` | Marks the hybrid head; renders the `head` component when one is declared, keeps its fences for the head split |
| `::include{ref="git#commits"}` | Grafts another document's section, or the whole document without an anchor, by the address `henia show` reads |
| `::include{template="context" slots="workflow"}` | Renders a component by name; its other attributes are that component's arguments, checked against its schema |
| `:::when{target="claude"}` / `:::unless{…}` | Keeps the body for the listed targets, or where a harness variable matches: `:::when{preload_context=true}`; targets are a closed list of harness names, variables must be declared in a harness |
| `:slot[…]`, `:contents[]`, `:related[]` | As today |

`include` takes exactly one of `ref` and `template`. A `ref` resolves after
parsing, relative to the including skill; a cycle is an error. A `template`
names a component whose body kind is `none`, so a skill can place a shared
fragment without a directive of its own. Together they replace Go
`{{template}}` includes and `henia.layout`.

## Pipeline

Per document and target:

1. Parse: goldmark with the directive extension; frontmatter templated.
2. Expand to a fixpoint. Each round validates every directive it finds against
   its declaration, resolves `when` and `unless` for the target, resolves
   `include`, and expands components inside out. Directives in a template's
   output join the next round, so generated `include`, `when` and components
   are validated and resolved like authored ones. A round that adds nothing
   ends the loop.
3. Expand `:slot`, `:contents`, `:related`.
4. Split the hybrid head at `:::head`.
5. Render: kept directives per the harness `directives` setting, references
   rewritten, Markdown serialized from the tree.

An `include{ref}` selects from the target document after the target has
expanded for the same harness with its own package's components, the same
rendered headings `henia show` addresses. Includes therefore form a graph over
documents; expansion follows it in dependency order, and a cycle is an error.

Across documents: links, references and section anchors resolve after every
document has expanded.

## Validation

Lint runs in three stages, like rustc's pre-expansion, early and late lints.
Each rule declares its stage.

| Stage | Sees | Checks |
|---|---|---|
| source | the skill as authored | syntax, unknown components, attribute types, duplicate attributes, fences, Go template actions in bodies, template syntax and fields, unused components |
| expanded | the tree after step 2, per target | allowed children of generated nodes, includes, conditions, component output that is not valid in its form |
| rendered | the output of step 5, per target | applied slots (`unused-slot`, `unknown-slot`), rendered headings (`missing-section`, `duplicate-heading`), anchors, head size, references |

Facts that only exist after expansion, such as the slots a partial applies or
the headings a skill renders, are checked on the rendered tree, not predicted
from declarations. Rendered lint runs once per harness, the renders
`henia build` already performs.

Every node created during expansion keeps an expansion backtrace: the directive
call in the skill and the component line that produced it. A diagnostic on a
generated node reports the call site, with a note naming the component.

Diagnostics carry an id, a level, a message and a position:

| Id | Level |
|---|---|
| `unknown-component` | error |
| `missing-attribute`, `unknown-attribute`, `invalid-attribute` | error |
| `duplicate-attribute`, `unclosed-directive` | error |
| `invalid-body`, `disallowed-child`, `wrong-form` | error |
| `missing-include`, `include-cycle`, `invalid-include` | error |
| `unknown-target`, `unknown-variable` | error |
| `unused-component` | warning |

Templates are parsed with `text/template/parse`, so source lint also checks
that every field a template reads is a declared attribute or a known skill
value.

hq sees directives with typed attributes, so queries such as
`(directive :name "routes" :ordered true)` work on canonical sources.

## Migration

One breaking change on top of the template composition branch, which Tropos
already runs on. Henia has no other consumers, so there is no compatibility
period: henia and Tropos change in the same pass.

Henia removes:

- Go template execution over skill bodies, with `{{template}}` includes and
  `henia.layout`;
- `.henia/templates/*.md.tmpl` and the `.args` and `.content` template data;
- silent handling of undeclared directives, duplicate attributes and unclosed
  containers.

Henia keeps:

- project over user resolution, and dependency packages resolving their own;
- directive expansion with `:::head` keeping its fences for the head split;
- lint rendering component output, now as the expanded and rendered stages.

Henia adds: `.henia/components/<name>.md` declarations, typed attributes with
bare-key booleans, `::body` and `.body`, fixpoint expansion, `include`, `when`
and `unless`, the three lint stages and expansion backtraces.

Tropos converts in the same change:

| Today | Becomes |
|---|---|
| `head`, `menu`, `routes`, `related`, `references` partials | components of the same names |
| `context`, `title`, `providers`, reached through `{{template}}` from `head` | components `head` emits as directives |
| `instruction`, rendered by the harness | a `render: keep` declaration |
| classes as flags: `:::routes{.ordered .protocol}` | bare keys: `:::routes{ordered protocol}` |
| `pr`'s `{{if eq .preload_context "true"}}` | `:::when{preload_context=true}` |
| `slots="workflow"` resolved inside `providers` | the same, with `slots` typed as a list of slots or a declared set name |

Done when, for every harness, the Tropos build matches the build before the
change apart from whitespace, all three lint stages pass on Tropos, and the
rendered heads of the hybrid skills are unchanged.

## Prior art

| Decision | From |
|---|---|
| Directive syntax, node shapes | CommonMark generic directives, micromark, mdast-util-directive |
| Typed attribute, argument and body specs; body kind switch | MyST directive specs |
| Attribute records with `required`, `default`, `matches`, levels; standalone validate | Markdoc |
| Child allow-lists, content models | Markdoc `children`, AsciiDoc content models |
| Tree grafting of a body, inside-out expansion | Hugo `.Inner`, unified transforms |
| Include by section address, after parsing | Obsidian embeds, Markdoc partials, MyST `include` and `embed` |
| Target conditions with a closed target set | Quarto `when-format`, AsciiDoc `ifdef` |
| One tree, one writer per target | Pandoc, MyST, unified |
| Formatter owns fence lengths | Djot's tooling-first syntax, remark-stringify |

## Risks

- Serializing Markdown from the tree normalizes it. Rendered skills will differ
  from sources in markers, escaping and blank lines; the output is generated, so
  only the build diff in the migration needs checking.
- A component's template output is parsed in the context of its call: a `text`
  component must produce inline Markdown, a `leaf` or `container` component
  blocks. Expanded lint checks the output's form.
- Rendered lint per harness multiplies lint time by the number of harnesses.
- `text/template` still runs over text inside a component. Positions inside
  template output point at the component file; positions of grafted bodies
  point at the skill.
- `henia fmt` becomes part of authoring; without it, nested fences still need
  counting.
