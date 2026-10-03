---
issue_type: Feature
created: 2026-10-03
updated: 2026-10-03
status: Proposed
stage: draft
---

# Skill runtime: library, runtime, projection

## Goal

Henia becomes a skill runtime. Harnesses list only a few entry skills; every
other skill is resolved, composed and rendered by Henia when an agent asks for
it. Henia keeps its compiler, which becomes one consumer of the runtime: it
projects entry skills into harness trees.

## Model

| Concept | Meaning |
|---|---|
| Library | Installed canonical skill sources, as authored |
| Source | One tree of skills plus the `henia.toml` that renders them |
| Catalog | The index of every library skill |
| Runtime | Resolves, composes, renders, caches and serves library skills |
| Projection | The entry skills built into one harness's skill tree |

A skill is not static or dynamic by itself. Every skill lives in the library.
A consumer's harness configuration decides which skills it projects; the
runtime serves all of them. Frontmatter carries no exposure flag, only
invocation policy such as `disable-model-invocation`.

## Library

### Project

```
.henia/
  .gitignore     # written by Henia: "*", then !.gitignore !henia.toml !skills/ !harnesses/
  henia.toml     # project configuration (or henia.toml at the repository root; both is an error)
  skills/        # the project source
  harnesses/     # project harness profiles
  build/         # projections (ignored)
  cache/         # ignored
  state/         # ignored
```

Henia writes `.henia/.gitignore` whenever it creates `.henia/`, so committed and
generated content can share the folder without depending on the repository's
root `.gitignore`.

### Global

| XDG base | Henia path | Content |
|---|---|---|
| `$XDG_DATA_HOME` | `henia/sources/<source>/` | an installed source: `skills/` and `henia.toml` |
| `$XDG_CONFIG_HOME` | `henia/` | user configuration and harness profiles |
| `$XDG_CACHE_HOME` | `henia/` | render cache |
| `$XDG_STATE_HOME` | `henia/` | runtime state |

Phora installs sources into `$XDG_DATA_HOME/henia/sources/<source>/`; Henia never
installs. A user's own global skills form a source like any other.

### Identity and resolution

A skill's identity is `<source>:<name>`. The project source is `project`; a global
source is named by its directory.

- `name` resolves to the project skill of that name if one exists, else to the
  only global skill of that name.
- If several global sources define `name` and the project does not, resolution
  fails and lists the qualified identities. Same-named skills never replace each
  other silently.
- `source:name` always resolves to exactly that skill.
- `name#section` and `source:name#section` address a section by its heading
  anchor.

## Catalog

The catalog lists each skill once:

| Field | Value |
|---|---|
| `id` | `<source>:<name>` |
| `name`, `source`, `tier` | `tier` is `project` or `global` |
| `path` | the SKILL.md path |
| `description` | from frontmatter |
| `digest` | SHA-256 of the skill directory's files |
| `sections` | anchor, title and level of each heading |

`henia ls` prints it; `--json` emits it.

## Runtime

```
henia ls [--json]
henia show <skill>[#section] [--harness NAME]
henia context <skill> [--harness NAME] [--global DIR]...
henia slots ...
```

- **Rendering.** `show` renders a skill for the caller through the owning
  source's `henia.toml`: its templates, variables, references and profile for
  that harness. The caller is `--harness`, then `HENIA_HARNESS`, then the
  `AI_AGENT` prefix (`claude-code` is `claude`), then `CLAUDECODE` or
  `CODEX_THREAD_ID`. Without a caller, or for a harness the source does not
  configure, rendering is neutral.
- **References.** A reference to a skill the caller's harness projects renders in
  that harness's syntax; any other skill reference renders as
  `henia show <skill>`, qualified when the bare name is ambiguous.
- **Budget.** Output over the budget prints the sections instead, each
  addressable with `henia show`.
- **Context.** `context` prints only dynamic material for a preload: the
  providers of the slots the skill applies and the sections of the library skills
  it references. It never repeats the skill's own text.
- **Cache.** Renders are cached under `$XDG_CACHE_HOME/henia/render`, keyed by the
  skill's canonical bytes, the harness and its configuration, the projected and
  library skill names and the Henia binary.
- **Failure.** Runtime commands print problems as text and exit 0, so a preload
  never aborts a skill.
- **Slots** resolve over the library and the harness skill directories.

## Projection

`henia build` projects entry skills from a source into each configured harness,
as today. A harness's `include` list selects its entry skills; without one the
harness projects every skill, as before. References from a projected skill to a
skill outside the projection render as `henia show <skill>`.

Guidance for sources: project at most about ten entry skills per harness, and
name the runtime in the always-loaded instructions, for example "Skills beyond
those listed are read with `henia ls` and `henia show`". Harnesses truncate or
drop descriptions well before large listings, and bare skill names are not enough
for an agent to discover what to read.

## Guarding

Library content is read through the runtime. A FAS rule denies direct reads of
`.henia/skills` and `henia/sources/` paths with Read, Grep, Glob and reading Bash
commands. FAS stays stateless; Henia generates nothing for it.

## Fixes this requires

- Global configuration honours an absolute `XDG_CONFIG_HOME`
  (`internal/config/config.go:54` hardcodes `~/.config`).
- Lint skips only `.henia/build`, `.henia/cache` and `.henia/state`, not all of
  `.henia` (`internal/lint/lint.go:230`), so the project library is linted.

## Non-goals

- MCP or any served protocol other than the CLI.
- Trust and approval of library sources (a later spec).
- Search, semantic search and reference graphs (later specs; the catalog's
  sections and digests are their input).

## Validation

- Unit: identity resolution, including ambiguity; catalog digests; `.henia/.gitignore`
  allowlist; XDG paths.
- Scrut: library layout, `ls`/`show`/`context` across project and global sources,
  ambiguity messages, per-harness rendering and reference syntax, projection
  references to unprojected skills, cache invalidation.
- Acceptance: a fixture library with a canary skill, a projected entry skill and
  FAS hooks in isolated homes; Claude Code and Codex read the canary through
  `henia show`, and a direct read is denied.
- Tropos: its source installs as `$XDG_DATA_HOME/henia/sources/tropos`, Claude
  projects only the entry skills, and `henia slots --for code` still resolves
  Loqui.
