# Lint rules

`henia lint` checks a skill package's contents with rules written as
[henia query](query.md) patterns. The standard library runs by default, and
projects and users add their own rules beside it.

Make a skill with two problems:

```bash
git init deploy-skills && cd deploy-skills
mkdir -p skills/deploy
cat > skills/deploy/SKILL.md <<'EOF'
---
name: deploy
description: Ship a release to production.
---

# Deploy

Read the [runbook](reference/runbook.md) first.

## Usage

Run `make release`.

## Usage

Tag the commit.
EOF
```

```console
$ henia lint skills --strict
skills/deploy/SKILL.md:8:11: warning [broken-link] local reference does not exist: reference/runbook.md
skills/deploy/SKILL.md:14:4: warning [duplicate-heading] heading repeated; first occurrence on line 10
Error: lint found 2 diagnostic(s)
```

Errors fail lint. Warnings fail it only under `--strict`. `--disable` turns
rules off by id, and `--format json` prints the diagnostics with their `related`
locations and, for similarity findings, `similarity`, `method`,
`shared_phrases` and `model`.

## What lint reads

Without paths, lint scans the project's package: the `skills`, `commands` and
`agents` directories that `henia.toml` names, at the project root and in
`.henia/`, plus the sources of its harness `files`. A directory argument yields
its skills, commands and agents, and the resources and references inside skill
directories; other Markdown is left out. A file argument is always linted.
Git-ignored files below each scanned path are skipped.

References resolve against everything lint can know, never a hand-kept list:

- the scanned files;
- the skills of the project's skill packages, transitively. Lint reads them for
  their names and anchors and never lints them.
- the commands and agents built into each harness in `henia.toml`, from its
  profile's `commands` and `agents`.

Templates are parsed and never executed, and lint fetches no URLs. To check
expanded values, lint the build output:

```bash
henia build skills-repo --harness codex --output .henia/build
henia lint .henia/build/codex --strict
```

## Standard library

| Module | Rules | Params and data |
|---|---|---|
| `std/structure` | `large-skill`, `large-static`, `duplicate-heading` | `max-lines` (500), `max-static-lines` (150) |
| `std/metadata` | `metadata`, `stale-review`, `invalid-template`, `invalid-markup` | `max-age-days` (180) |
| `std/references` | `broken-link`, `missing-reference`, `outdated-reference` | `outdated` map |
| `std/duplicates` | `duplicate-skill`, `duplicate-content` | `min-words` (12) |
| `std/slots` | `invalid-slot`, `unknown-slot` | |
| `std/similarity` | `similar-content`, `semantic-content` | `min-words` (12), `similarity`, `containment`, `threshold` (0, off) |

The modules live under `internal/lint/std/`, readable like any other module.
`henia lint test` runs the examples of every module, the standard library's
included:

```console
$ henia lint test
29 lint examples passed
```

## Configuration

`henia.toml` tunes rules and holds one-off rules:

```toml
[lint]
disable = ["large-static"]

[lint.config.duplicate-heading]
severity = "error"

[lint.config.large-skill]
max-lines = 600

[lint.config.outdated-reference.outdated]
"old-model-id" = "replacement-model-id"

[[lint.rules]]
id = "no-make"
query = '(paragraph :matches /\bmake /) @p'
message = "Use the mise task, not make."
```

With that file in the project above:

```console
$ henia lint skills
skills/deploy/SKILL.md:8:11: warning [broken-link] local reference does not exist: reference/runbook.md
skills/deploy/SKILL.md:12:1: warning [no-make] Use the mise task, not make.
skills/deploy/SKILL.md:14:4: error [duplicate-heading] heading repeated; first occurrence on line 10
Error: lint found 3 diagnostic(s)
```

- `disable` turns rules off by id.
- `[lint.config.<id>]` sets `severity` and overrides the rule's params and data
  tables. Each value is parsed against its default: a number param takes a
  number, and a list or table takes the same shape. An unknown key or rule is
  an error that suggests the nearest name.
- `[[lint.rules]]` declares a rule inline with `id`, `query` and `message`, and
  optionally `severity`, `at`, `related`, `params` and `data`.
- A skill turns a rule off for itself with `henia.lint.disable` in its
  frontmatter.

## Modules

A module is a Markdown file of rules. Save this as
`.henia/lint/team/headings.md`:

````md
---
data:
  banned: [usage]
---

## repeated-usage

A document keeps a single Usage section.

```hq
(import std/structure)

(rule repeated-usage
  :message "{@h.text} repeats; first on line {@first.line}"
  :at @h
  (repeated-heading @first @h ?text)
  (row :table banned :value ?text))
```

### Matches

```md
## Usage

## Usage
```

### Passes

```md
## Setup

## Setup
```
````

```console
$ henia lint skills --disable duplicate-heading,no-make
skills/deploy/SKILL.md:8:11: warning [broken-link] local reference does not exist: reference/runbook.md
skills/deploy/SKILL.md:14:4: warning [repeated-usage] Usage repeats; first on line 10
```

- The frontmatter sets `params` and `data`.
- Each `## <rule-id>` section explains its rule in prose, defines it in a
  ```` ```hq ```` block, and tests it with ```` ```md ```` examples under
  `### Matches` and `### Passes`. A Matches example must make its rule report;
  a Passes example must not.
- An example overrides params in its fence info, as in ```` ```md max-lines=2 ````
  or ```` ```md outdated.old=new ````.
- The module name is its path relative to the layer, without `.md`, as in
  `team/headings`.

Lint loads modules from these layers, in order:

1. the standard library;
2. `.henia/lint/` in each skill package, named `<package>/<module>`;
3. `$XDG_CONFIG_HOME/henia/lint/`;
4. the project's `.henia/lint/`;
5. inline rules.

Rule ids and module names are unique across all layers, and nothing shadows
anything. To change a built-in rule, configure it, or disable it and compose a
replacement.

### Forms

- `(rule id :key value ... pattern...)` defines a rule: a query whose rows
  become diagnostics. One module may give an id several clauses.
- `(define (name ?var @capture ...) pattern...)` names a pattern. Callers pass
  the variables and captures it binds; every other variable stays local to each
  use.
- `(import module)` brings in a module's defines. Names that would collide are
  an error.

Frontmatter `params` scalars become `$name` values. `data` maps and lists become
`row` nodes under a `data` root, with `:table`, `:key` or `:index`, and `:value`.

### Rule keys

| Key | Meaning |
|---|---|
| `:severity` | `warning` (default) or `error` |
| `:message` | text with `{@c}` (path:line), `{@c.line}`, `{@c.text}`, `{@c.KEY}`, a value capture `{@n}`, `{@n.percent}` or `{@n.3}`, `{?var}`, `{$param}` and `{shared}` |
| `:at @a @b` | location: the first of these captures that matched; else the first capture |
| `:related @c ...` | related location, chosen the same way |
| `:focus ?var` | moves the location to the variable's first occurrence in the node |
| `:score @capture` | fills `similarity` from a score capture |
| `:method name` | fills `method` |
| `:shared @a @b` | fills `shared_phrases` and `{shared}` with up to five shared phrases |

### Grouped rules

A rule may end with a [`(group ...)`](query.md#grouping) clause, whose filters
take `$params`. Saved as `.henia/lint/team/reach.md`, this one flags skills
that no other skill links to:

````md
---
params:
  min-inbound: 1
---

## unreferenced-skill

Every skill is linked from at least one other skill.

```hq
(rule unreferenced-skill
  :message "{@t.id} has {@n} inbound links; under {$min-inbound}"
  :at @t
  (skill :id ?s) @t
  (skill :id (not ?s) (link :target ?s) @l)?
  (group @t (count @l @n (< $min-inbound))))
```
````

```console
$ henia lint skills
skills/deploy/SKILL.md:1:1: warning [unreferenced-skill] deploy has 0 inbound links; under 1
skills/deploy/SKILL.md:8:11: warning [broken-link] local reference does not exist: reference/runbook.md
skills/deploy/SKILL.md:12:1: warning [no-make] Use the mise task, not make.
skills/deploy/SKILL.md:14:4: error [duplicate-heading] heading repeated; first occurrence on line 10
skills/deploy/SKILL.md:14:4: warning [repeated-usage] Usage repeats; first on line 10
Error: lint found 5 diagnostic(s)
```

Grouped rows feed `:at`, `:related`, `{@n}` and `{?var}` unchanged, and a collected
capture such as `@l` locates at its first node.

### Facts lint adds

Lint rules see the document tree that queries see, plus these facts:

- `file`:
  - `:kind` is `skill`, `command`, `agent`, `resource` or `document`;
  - typed artifacts also carry `:name` and `:artifact` (`kind:name`);
  - `:file` is the path on disk.
- `problem` nodes under a file, with `:kind` `frontmatter`, `template` or
  `markup` and a `:message`.
- `entry` nodes under `frontmatter`, one per scalar, with a dotted `:key`,
  `:value`, `:index` for list items, and `:tag` (YAML type, as in `!!str`).
- `slot` nodes for `henia.slots` and `henia.provides` entries and `:slot[...]`
  directives, with `:role` (`declare`, `provide` or `apply`), `:slot`, `:type`,
  `:priority` and `:error`.
- `link` and `image` facts:
  - `:dest` is the destination as written;
  - `:exists`, `:anchored` and `:valid` are `true` or `false`;
  - `:problem` holds an inspection error;
  - references carry `:ref` (`skill`, `command`, `agent`, `file` or `show`),
    `:name` and `:artifact`.
- `:position` orders nodes by file path, then offset, which gives "first
  occurrence" across files.
- `:dynamic true` marks nodes whose text holds a template action.

## Similarity

`similar-content` and `semantic-content` compare each paragraph of at least
`min-words` words with the earliest earlier paragraph above a threshold.
`duplicate-content` reports exact copies, and paragraphs matched lexically are
left out of the semantic check. Both rules stay off until a threshold is set:

```toml
[lint.config.similar-content]
similarity = 0.7   # word-shingle Jaccard
containment = 0.9  # shared shingles over the shorter paragraph

[lint.semantic]
enabled = true
model_path = "models/potion-retrieval-32M"

[lint.config.semantic-content]
threshold = 0.35   # calibrate against accepted and rejected examples
```

`model_path` names a local Model2Vec directory holding `tokenizer.json`,
`config.json` and `model.safetensors`. A relative path resolves against the
configuration file that declares it. Lint downloads no models, calls no servers
and runs no subprocesses. Inference uses the pure Go
[aikit Model2Vec implementation](https://github.com/townsendmerino/aikit/tree/v1.16.0/embed)
and loads the model once, only when a semantic rule runs.

[Model2Vec](https://github.com/MinishLab/model2vec) embeddings find related
wording with little lexical overlap. They lose token order and cannot tell
whether two instructions mean the same: a negated sentence may score higher
than a valid paraphrase. Findings ask for review and never rewrite content.
[Paragraph similarity](paragraph-similarity.md) has the evaluation.
