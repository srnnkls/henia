# Lint rules

`henia lint` runs rules written in [hq](query.md) over a package's contents.
Without paths it scans the project's package: the `skills`, `commands` and
`agents` directories that `henia.toml` names, at the project root and in
`.henia/`, plus the sources of its harness `files`. A directory argument yields
its skills, commands and agents and the resources and references inside skill
directories; other Markdown is left out. A file argument is always linted.

Rules live in Markdown modules: the standard library ships with Henia and runs
by default, and projects and users add their own modules, which import the
standard library's patterns and compose new rules from them.

## Standard library

| Module | Rules | Params and data |
|---|---|---|
| `std/structure` | `large-skill`, `duplicate-heading` | `max-lines` (500) |
| `std/metadata` | `metadata`, `stale-review`, `invalid-template`, `invalid-markup` | `max-age-days` (180) |
| `std/references` | `broken-link`, `missing-reference`, `outdated-reference` | `outdated` map |
| `std/duplicates` | `duplicate-skill`, `duplicate-content` | `min-words` (12) |
| `std/slots` | `invalid-slot`, `unknown-slot` | |
| `std/similarity` | `similar-content`, `semantic-content` | `min-words` (12), `similarity`, `containment`, `threshold` (0, off) |

`henia lint test` runs every module's examples, the standard library's
included. The modules are readable with the rest of Henia's source code under
`internal/lint/std/`.

## Configuration

```toml
[lint]
disable = ["duplicate-heading"]

[lint.config.large-skill]
severity = "error"
max-lines = 600

[lint.config.outdated-reference.outdated]
"old-model-id" = "replacement-model-id"

[[lint.rules]]
id = "no-todo"
query = '(paragraph :matches /TODO/) @p'
message = "Resolve the TODO before release."
```

- `disable` turns rules off by id.
- `[lint.config.<id>]` sets `severity` and overrides the rule's params and data
  tables. Each value is parsed against its default: a number param takes a
  number, a list or table takes the same shape, and an unknown key or rule is
  an error that suggests the nearest name.
- `[[lint.rules]]` declares a one-off rule inline with `id`, `query`, `message`,
  and optionally `severity`, `at`, `related`, `params` and `data`.
- A skill can turn a rule off for itself with `henia.lint.disable` in its frontmatter.

References resolve against everything lint can know, never a hand-kept list:

- the scanned files;
- the skills of the project's skill packages, transitively. Those are read for
  their names and anchors but never linted.
- the commands and agents built into each harness in `henia.toml`, which come
  from its profile's `commands` and `agents`.

Errors fail lint, and `--strict` also fails on warnings. `--format json` prints
the diagnostics, including `related` locations and, for similarity findings,
`similarity`, `method`, `shared_phrases` and `model`.

## Modules

Lint loads modules from these layers, in order:

1. the standard library;
2. `.henia/lint/` in each skill package, named `<package>/<module>`;
3. `$XDG_CONFIG_HOME/henia/lint/`;
4. the project's `.henia/lint/`;
5. inline rules.

Rule ids and module names are unique across all layers, and nothing shadows
anything. To change a built-in rule, configure it, or disable it and compose a
replacement.

A module is a Markdown file:

- its frontmatter sets `params` and `data`;
- each `## <rule-id>` section documents a rule in prose, defines it in a
  ```` ```hq ```` block, and tests it with ```` ```md ```` examples under
  `### Matches` and `### Passes`.

````md
---
data:
  banned: [usage]
---

## repeated-usage

Teams keep a single Usage section per document.

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
````

The module name is its path relative to the layer, without `.md`, as in
`team/headings`. Module forms:

- `(rule id :key value ... pattern...)` defines a rule. It is a query whose rows
  become diagnostics, and one module may give an id several clauses.
- `(define (name ?var @capture ...) pattern...)` names a pattern. Callers pass
  the variables and captures it binds, and every other variable stays local to
  each use.
- `(import module)` brings in a module's defines. Names that would collide are
  an error.

Frontmatter `params` scalars become `$name` values. `data` maps and lists become
`row` nodes under a `data` root, with `:table`, `:key` or `:index`, and `:value`.
An example overrides params in its fence info, as in ```` ```md max-lines=2 ````
or ```` ```md outdated.old=new ````. A Matches example must make its rule report;
a Passes example must not.

### Rule keys

| Key | Meaning |
|---|---|
| `:severity` | `warning` (default) or `error` |
| `:message` | text with `{@c}` (path:line), `{@c.line}`, `{@c.text}`, `{@c.KEY}`, `{?var}`, `{?var.percent}`, `{?var.3}`, `{$param}` and `{shared}` |
| `:at @a @b` | location: the first of these captures that matched; else the first capture |
| `:related @c ...` | related location, chosen the same way |
| `:focus ?var` | moves the location to the variable's first occurrence in the node |
| `:score ?var` | fills `similarity` |
| `:method name` | fills `method` |
| `:shared @a @b` | fills `shared_phrases` and `{shared}` with up to five shared phrases |

### Facts lint adds

Beyond the document tree that queries see, lint rules match these:

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
- `:position`, which orders nodes by file path and then offset: "first
  occurrence" across files.
- `:dynamic true` on nodes whose text holds a template action.

Templates are parsed but never executed. Lint the build output to check
expanded values:

```bash
henia build skills-repo --harness codex --output .henia/build
henia lint .henia/build/codex --strict
```

## Similarity

`similar-content` and `semantic-content` compare each paragraph of at least
`min-words` words with the earliest earlier paragraph above a threshold. Exact
copies are left to `duplicate-content`, and paragraphs matched lexically are
left out of the semantic check. Both stay off until a threshold is set:

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

`model_path` points to a local Model2Vec directory containing `tokenizer.json`,
`config.json` and `model.safetensors`. A relative path resolves against the
configuration file that declares it. Lint does not download models, call
servers or run subprocesses. Inference uses the pure Go
[aikit Model2Vec implementation](https://github.com/townsendmerino/aikit/tree/v1.16.0/embed),
loads the model once, and only when a semantic rule runs.

[Model2Vec](https://github.com/MinishLab/model2vec) embeddings find related
wording with little lexical overlap. They lose token order and do not establish
equivalent instructions; changed negation may score higher than a valid
paraphrase. Findings request review; they never rewrite content. See
[the evaluation notes](paragraph-similarity.md).
