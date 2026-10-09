---
created: 2026-10-08
status: active
issue_type: Feature
---

# components

## Goal

Skills compose shared structure only through typed directive components, as specified in [design.md](design.md): declared once in `.henia/components/<name>.md`, expanded as trees to a fixpoint, validated in source, expanded and rendered lint stages, and serialized from the tree. Go template composition of skill bodies is removed, and Tropos converts in the same pass.

## Context

Research against henia `main` at `b24ebfe` (template composition, #48) and Tropos `feat/head-templates` at `62e4e2e`.

Henia today:

- Directives are parsed by henia's own goldmark parsers (`internal/markup/markup.go:125-237`). Container and text forms exist; the leaf form `::name` is not parsed (`markup.go:142`).
- Attributes go through goldmark `parser.ParseAttributes` (`markup.go:84-123`). Bare keys fail, `.a .b` merge into one `class`, duplicate keys are already rejected, and every value is a string.
- Only the first parse error is kept (`markup.go:18-71`), and most callers drop it (`root, _ := Tree(...)`). Unclosed containers are detected but surface only as a `problem` node in lint.
- A container closes only on its innermost open fence (`markup.go:173-186`), so same-length nesting already parses.
- `Element` (`internal/markup/tree.go:18-30`) is read-only with byte offsets into one source string; it has no file identity and no backtrace. Inline directives carry no label children.
- No Markdown writer exists. Every pipeline step splices text: `markup.Expand`, `Unwrap`, `slots.Expand`, `expandListings`, `renderLinks`, `markup.Render`, `renderReferences`.
- Composition: `internal/templates` loads `.md.tmpl` (project shadows user); `transform.Compose` (`internal/transform/transform.go:57-88`) runs `{{template}}` and `henia.layout`; `markup.Expand` splices template directives with `.args`/`.content` (`internal/markup/expand.go:57-89`), keeping `:::head` fences.
- Build and `henia show` resolve a dependency's templates from its own root (`internal/build/build.go:77-84`, `internal/cli/runtime.go:513`); lint does not (`internal/cli/lint.go:153`).
- Lint rules are hq queries in `internal/lint/std/*.md` over one corpus, without stages. Template slots are rendered once with variables merged across harnesses (`internal/cli/lint.go:154-161`), and `sectionAnchors` over-approximates rendered headings.
- `Diagnostic` (`internal/lint/lint.go:20-32`) has no notes or backtrace.
- hq reads directive attributes as strings (`internal/pattern/match.go:880`); `directive` is an open-key node type (`internal/pattern/read.go:132`).
- No `henia fmt` exists.

Tropos today:

- Eight partials in `.henia/templates/`: `head`, `context`, `providers`, `title`, `menu`, `routes`, `related`, `references`. `head` reaches `context`, `providers` and `title` through `{{template}}`.
- 20 `:::head` uses in 17 files; 9 files use `:::routes`, 5 with `{.ordered .protocol}`; 13 `:::menu`, 9 `:::related`, 4 `:::references`, 4 `:::instruction{priority=high}`.
- No `henia.layout` and no body `{{template}}`; the only body template actions are in `skills/pr/SKILL.md:25-31`.
- `slots="workflow"` appears in `implement`, `continue` and `loop`; `providers.md.tmpl` expands it with a string comparison.
- `routes` links the dispatch protocol relative to the skill `.name`.
- Harness variables `preload_context` and `context_instruction` live in Tropos `henia.toml` for `claude`, `codex`, `pi`, `omp`; 15 skills are hybrid.

### Key Files

| File | Lines | Description |
|------|-------|-------------|
| `internal/markup/markup.go` | 18-71, 84-123, 125-237, 239-351 | Errors, attributes, block and inline directive parsers, `Render` |
| `internal/markup/tree.go` | 18-45, 141-231, 310 | `Element`, `Tree`, directive elements, parser options |
| `internal/markup/expand.go` | 9-99 | Template-directive splicing, `HeadBlock`, `TemplateDirectives` |
| `internal/markup/unwrap.go` | all | Head split |
| `internal/markup/sections.go` | 17 | Section addresses used by `henia show` |
| `internal/templates/templates.go` | 15-75 | Template discovery, shadowing, digest |
| `internal/transform/transform.go` | 57-114, 188-278, 458-490 | `Compose`, `Transform` pipeline, listings |
| `internal/slots/slots.go` | 263-303 | `:slot` applications and expansion |
| `internal/build/build.go` | 77-84, 167-183, 350-357, 421-446 | Template roots, hybrid head, `Render`, directive format |
| `internal/build/head.go` | 27 | `hybridHead` |
| `internal/cli/runtime.go` | 143-162, 513 | `henia show --head`, origin roots |
| `internal/cli/lint.go` | 111-161 | Lint options, template roots, merged variables |
| `internal/lint/lint.go` | 20-32 | `Diagnostic` |
| `internal/lint/compile.go` | 29, 238-273 | `Compile`, `Plan.Run`, `Plan.Test` |
| `internal/lint/module.go` | 169-251, 272-332, 480-497 | Module parsing, clause evaluation, corpus |
| `internal/lint/templates.go` | 31, 109, 158-238 | Template nodes, `sectionAnchors`, `templateSlots` |
| `internal/lint/slots.go` | 13 | Slot nodes |
| `internal/library/corpus.go` | 165-183 | hq tree per file, `problem` nodes |
| `internal/pattern/read.go`, `match.go` | 116-136; 880 | Node schema, attribute matching |
| `internal/config/harness.go` | 12-16, 53 | `directives` values, harness map |
| `docs/build.md` | 81-178 | Templates, layouts, template directives |
| `tests/templates.md` | all | Scrut coverage of template composition |
| `/Users/srnnkls/projects/tropos/.henia/templates/` | all | Partials to convert |

### Architecture Decisions

These settle what [design.md](design.md) leaves open; everything else follows the design.

#### AD-1: Nesting keeps innermost-close parsing

The parser keeps henia's current rule: a closing fence closes the innermost open container whose opening fence it matches or exceeds. Authors may write `:::` at every depth. `henia fmt` rewrites fences to micromark's convention, each outer fence one colon longer than the longest fence it contains, so other CommonMark directive tools parse the same tree.

#### AD-2: A span-preserving tree writer

The document tree is mutable. Every node records its origin (file, byte span, line, column) and, when expansion created it, a backtrace of call sites, each a directive call and the component line that produced it. The writer serializes from the tree: a node unchanged since parsing writes its source span verbatim; directive fences are written from the tree with fmt's lengths; generated and grafted nodes write their own origin spans. `Write(Parse(src))` returns `src` for sources whose fences are already canonical. This keeps the Tropos build diff to whitespace (design Risks, first item).

#### AD-3: Attribute data

- `#id` and `.class` are the attributes `id` (string) and `class` (list), checked against the declaration like any other; a component that declares neither rejects them as `unknown-attribute`.
- Template data is the skill's values (frontmatter, `henia.variables`, harness variables) plus every declared attribute. A declared attribute shadows a skill value of the same name.
- A known skill value is a frontmatter key of any skill in the component's package, a `henia.variables` key, or a variable of any configured harness. Every known value is present in the data; one the calling skill does not define holds nil, so `range`, `with` and `if` skip it. Printing it yields nothing through the absent-blanking pass frontmatter templating already applies (`blankAbsent`, `printAbsentBlank`, `internal/transform/transform.go:107-139`); CMP-003 moves that pass to `internal/templating`, shared by frontmatter and component execution, and it outlives legacy composition. `missingkey=error` therefore fails only on names that are neither attributes nor known values, which `unknown-field` already reports in source lint.
- `unknown-field` checks only field chains rooted in the component data: `.x` where dot is still the root (outside `range` and `with`) and `$.x` anywhere. Inside `range` and `with`, dot is the element and its fields are not checked; Tropos's `{{range .context_commands}}{{.label}}` passes.
- `type: list` takes an optional element type, `of: slot|skill|ref|string`; each item is then checked as that type.
- Harness variables are strings; `when` and `unless` compare an attribute's raw text to the variable's value.
- `when{target=…}` takes the configured harness names (built-in plus project); a variable condition names a variable some configured harness declares.

#### AD-4: Diagnostic ids beyond the design's table

| Id | Level | Stage |
|---|---|---|
| `invalid-component` | error | source: a declaration whose schema is malformed |
| `template-action` | error | source: a Go template action in a skill body |
| `invalid-template` | error | source: a component template that does not parse |
| `unknown-field` | error | source: a template field that is neither a declared attribute nor a known skill value |
| `component-cycle` | error | expanded: a component whose expansion reaches its own call again |
| `removed-composition` | error | source: a `.henia/templates` directory or a `henia.layout` key, with a pointer to components |

`removed-composition` for a directory and `unused-component` carry a path and no line or column.

#### AD-5: Built-in names and the head

- A project or user declaration named `head` supplies the template of the built-in `head`. Its output replaces the call's body inside the kept `:::head` fences, and the call is marked expanded so later rounds skip it; step 4 consumes the fences. Declaring any other built-in name (`include`, `when`, `unless`, `slot`, `contents`, `related`, `body`) is `invalid-component`.
- The built-in `related` has two forms: the text form `:related[…]` as today, and a container form `:::related` that renders a `## Related` heading followed by its body. It is the only built-in with more than one form; Tropos drops its `related` partial.
- `include{ref}` selects from the target's tree after its step 3 (components, `:slot`, `:contents`, `:related` expanded; head not yet split), addressed by the same section anchors `henia show` uses. Kept directives in the grafted subtree render with the including skill's step 5.

#### AD-6: Errors block rendering

Source and expanded errors stop the build and `henia show` for that document, printing every diagnostic. A missing dependency of a document does not stop other documents.

The runtime render cache key (`internal/cli/runtime.go:527`) covers the canonical content of every document the render includes, transitively and across packages, and the component declarations of every package involved, in place of `templates.Digest`.

#### AD-7: No slot sets

`slots` on Tropos's `head` component is `type: list` with `of: slot` (AD-3). The `workflow` shorthand is dropped; `implement`, `continue` and `loop` list the five slots. A set mechanism can follow in its own scope.

#### AD-8: Lint stages

Each std rule in every module declares `stage` (`source`, `expanded`, `rendered`); a rule without one fails `henia lint test`. Built-in checks from AD-4 and the design's table run as Go checks in their stage and report through the same `Diagnostic`, which gains `notes` (the expansion backtrace) and `stage`. Rendered rules run per harness over each skill's full body as `henia show` renders it for that harness, without build's projection filter, so dynamic and hybrid skills are linted on every section their offers address; hybrid heads are rendered separately for the head-size check. Findings report the harness when they are harness-specific. Lint resolves components per package, as build does.

#### AD-9: Tropos baseline

The migration baseline is the Tropos build with henia built at `b24ebfe`, captured before Tropos changes. Tropos runs the migration on a `feat/components` branch cut from `feat/head-templates`.

### Constraints

- Go 1.26, goldmark v1.7.16; no new module dependency.
- Frontmatter strings stay templated with `text/template`.
- Output of `henia build` for skills that use neither directives nor removed composition features (body template actions, `henia.layout`, `.henia/templates`) is byte-identical to `b24ebfe`. Body template actions are migration errors (`template-action`).
- `go build ./...` and `go test ./...` pass at every batch boundary; legacy composition is deleted only after its last consumer has moved (CMP-016).
- The generated `.henia/.gitignore` exempts `components/` (`internal/library/library.go:298`).
- No compatibility period: henia and Tropos change together.

## Requirements

### Functional Requirements

- FR-1: Directive syntax per design "Syntax": text, leaf and container forms; leaf and text labels hold inline children; attributes `key=value`, quoted values, bare keys as boolean true, `#id`, `.class`; repeated keys raise `duplicate-attribute`, unclosed containers `unclosed-directive`, each with position; every error in a document is reported.
- FR-2: Mutable tree with origins and backtraces, and a writer per AD-2.
- FR-3: Component declarations per design "Declarations": discovery (project over user, dependency packages against their own), schema fields, typed attributes (`string`, `boolean`, `number`, `list` with optional `of`, `enum`, `slot`, `skill`, `ref`) with `required`, `default`, `values`, `matches`; `render: keep` passthrough; built-in declarations for `head`, `include`, `when`, `unless`, `slot`, `contents`, `related`, `body`, with built-in names reserved and `related`'s two forms per AD-5.
- FR-4: Source-stage call checks per design "Validation" and AD-4, without executing templates; `text/template/parse` field checks; `unused-component` warning.
- FR-5: Fixpoint expansion per design "Pipeline" step 2: `when`/`unless` per target, `include{template}`, components inside out, `::body` grafting and `.body` text, `missingkey=error` with absent skill values per AD-3, the head rule of AD-5, output form checks, backtraces, `component-cycle`.
- FR-6: `include{ref}` grafts a section (or whole document) of the target expanded for the same harness with its own package's components, selected per AD-5; includes form a graph expanded in dependency order; cycles raise `include-cycle`.
- FR-7: Pipeline steps 3-5 operate on the tree: `:slot`, `:contents`, `:related`, head split, kept-directive rendering per harness `directives`, links and references, serialization.
- FR-8: Build, `henia show` and the runtime use the component pipeline (CMP-008). Once lint no longer consumes them, Go template execution over bodies, `henia.layout`, `.henia/templates`, `.args`/`.content`, `markup.Expand`, `MaskTemplates` and `internal/templates` are deleted, and their inputs raise `removed-composition` (CMP-016).
- FR-9: Lint stages per AD-8: source, expanded per target, rendered per harness (`unused-slot`, `unknown-slot`, `missing-section`, `duplicate-heading`, anchors, head size, references); diagnostics carry backtrace notes.
- FR-10: hq exposes directive attributes typed by their declaration: `(directive :name "routes" :ordered true)`.
- FR-11: `henia fmt [paths]` rewrites directive fence lengths per AD-1, idempotently; `--check` exits non-zero and lists files that would change.
- FR-12: Tropos converts per design "Migration" and AD-7, including its existing `.henia/.gitignore`.

### Technical Requirements

- TR-1: Each diagnostic id in the design table and AD-4 has a unit test that triggers it at an exact line and column, or at an exact path for the file-level ids of AD-4.
- TR-2: `go vet ./... && go build ./...`, `go test ./...` and `mise run test:integration` pass.
- TR-3: `henia lint test` passes for every std module after rules gain stages.
- TR-4: `docs/build.md`, `docs/lint-rules.md`, `docs/query.md` and a new `docs/components.md` describe the shipped behavior; `tests/templates.md` is replaced by `tests/components.md`.

## Acceptance Criteria

- [ ] Given `:::routes{ordered protocol}`, when parsed, then the directive has attributes `ordered` and `protocol`, both boolean true.
- [ ] Given `:x[a]{k=1 k=2}` on line 3, when linted, then `duplicate-attribute` reports line 3 at the column of the second `k`.
- [ ] Given an unclosed `:::menu` and an undeclared `:::rotues` in one skill, when built, then both `unclosed-directive` and `unknown-component` print and the skill does not render.
- [ ] Given `::body` inside a container component's template, when expanded, then the call's body subtree replaces it and keeps its positions in the skill file.
- [ ] Given a fixture `routes` component equal to the design's example and `:::routes{ordered}` holding a table, when built, then the output holds `## Routes`, the ordered line and the table, and no protocol link.
- [ ] Given a component whose template emits `::include{template="title" title="X"}`, when built, then the include resolves in the next round; given that output with an undeclared attribute, then the diagnostic points at the skill's call with a note naming the component file and line.
- [ ] Given skills A and B where A includes `B#Usage`, when built for `codex`, then A holds B's `Usage` section as B renders for `codex`; given B includes A, then `include-cycle` reports both call sites.
- [ ] Given `henia show A` cached while A includes `B#Usage`, when B's `Usage` text changes, then the next `henia show A` holds the new text; when B then gains an undeclared directive, the next `henia show A` fails with `unknown-component`.
- [ ] Given a component reading `{{range .items}}{{.label}}{{end}}` and `{{$.nope}}`, when linted, then only `$.nope` raises `unknown-field`.
- [ ] Given Tropos's hybrid `scope` skill offering `scope.templates` from `#templates`, a section outside its head, when linted, then no `missing-section` is reported.
- [ ] Given `:::when{target="claude"}`, when built, then the body is kept for `claude` and dropped for `codex`; `target="clod"` raises `unknown-target`, `:::when{nope=true}` raises `unknown-variable`.
- [ ] Given `{{.x}}` in a skill body, then `template-action` is reported; given `description: "{{.x}}"` in frontmatter with `x` declared, then the description renders the value.
- [ ] Given a project with `.henia/templates/head.md.tmpl`, when built, then `removed-composition` is reported.
- [ ] Given a component reading `{{range .context_commands}}` called from a skill without `context_commands` while another skill in the package defines it, when built, then the call renders with that block empty and a `{{.context_commands}}` print beside it renders nothing, never `<no value>`; a template reading `.nope`, known nowhere, raises `unknown-field`.
- [ ] Given `:related[git]` and a `:::related` container holding a list in one skill, when built, then the first expands as today and the second renders `## Related` followed by the list.
- [ ] Given a project `head` component and `:::head{title="X"}`, when expanded, then the fixpoint ends after one round with the output inside `:::head` fences; a project `include.md` raises `invalid-component`.
- [ ] Given `slots` declared `type: list` with `of: slot` and `slots="git.commits nope"`, when linted, then `invalid-attribute` names `nope`.
- [ ] Given every file under `tests/fixtures` and Tropos `skills/*/SKILL.md` after `henia fmt`, when parsed and written without changes, then the output equals the input byte for byte.
- [ ] Given `:::a` containing `:::b`, when running `henia fmt`, then the file holds `::::a` … `::::` around `:::b` … `:::`; a second run changes nothing; `henia fmt --check` on the original exits 1 and names the file.
- [ ] Given a partial that applies `:slot[x]` only under `:::when{target="claude"}`, when linted, then `unused-slot` for `x` is reported for `codex` only.
- [ ] Given a fixture with `:::routes{ordered}` and `:::routes`, when running `henia q --canonical '(directive :name "routes" :ordered true) @d'`, then only the first matches.
- [ ] Given Tropos after conversion, when built for `claude`, `codex`, `pi` and `omp`, then `diff -rw` against the AD-9 baseline is empty per harness, `henia lint` reports no errors in any stage, and `henia show <skill> --head` is unchanged for all 15 hybrid skills.

## Non-goals

- Slot sets (AD-7).
- Updating Tropos's `mise.toml` henia pin; that follows a henia release.
- A general Markdown formatter beyond directive fences.

## Gaps

- G-1 (resolved 2026-10-09: feat/head-templates clean at b023942): Tropos's working tree on `feat/head-templates` has uncommitted changes in `skills/peer/SKILL.md`, `skills/peer/scripts/peer` and `skills/peer/scripts/peer-herdr-test`, and untracked `.henia/.gitignore` and `dist/`. CMP-014 cannot branch until the owner commits or sets them aside.
