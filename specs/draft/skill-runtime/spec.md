---
issue_type: Feature
created: 2026-10-03
updated: 2026-10-04
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
henia preload --skill <skill> -- <command>
henia slots ...
```

- **Rendering.** `show` renders a skill for the caller through the owning
  source's `henia.toml`: its templates, variables, references and profile for
  that harness. The caller is `--harness`, then `HENIA_HARNESS` (`none` is
  neutral), then the nearest agent among Henia's ancestor processes, then the
  `AI_AGENT` prefix (`claude-code` is `claude`), `CODEX_THREAD_ID` or
  `CLAUDECODE`. Environment markers come last: outer agents leak them into
  nested ones, and Codex sets no `AI_AGENT`. Without a caller, or for a harness
  the source does not configure, rendering is neutral.
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
  library skill names and the Henia binary. Preload output is never cached.
- **Failure.** Runtime commands print problems as text and exit 0, so a preload
  never aborts a skill.
- **Slots** resolve over the library and the harness skill directories.

## Preloads

A skill embeds shell commands with Claude Code's preload syntax: `` !`cmd` ``
inline, or a fenced block whose info string is `!`. `show` runs them while
rendering and prints each in place as a `text` block: the command after `$ `
(continuation lines after `> `), then its combined output.

### Invariant

A preload has no side effects. Henia guarantees this itself, independent of
FAS, Tropos or any rule set:

1. Write sandbox. Every preload runs in an OS sandbox that denies file writes
   except to `/dev/null`; reads and network stay open. macOS uses
   `sandbox-exec` (`(deny file-write*)`), Linux `bwrap` with a read-only root.
   Henia probes the sandbox once per process; where none works, preloads do
   not run unless the user's own `henia.toml` sets `unsandboxed = "run"`.
2. Timeout. Every preload is killed with its process group after
   `preload.timeout` (default 10s).
3. Refusal. Before running, Henia parses the command with `mvdan.cc/sh` and
   checks every simple command in pipelines, lists, subshells, functions and
   command or process substitutions. It refuses what changes state the sandbox
   cannot see, and fails closed on what it cannot inspect.
4. Henia writes nothing on a preload's behalf. Preload output is never cached:
   the render cache holds the template render, and preloads run on every
   `show`.

Commands that look read-only can still write; the sandbox catches those
(`git status` refreshes `.git/index`, so skills use
`git --no-optional-locks status`; Henia also sets `GIT_OPTIONAL_LOCKS=0`).

### Built-in refusals

Each refusal prints as `henia: blocked by henia/<rule>: <reason>` in place of
the output.

| Rule | Refuses |
|---|---|
| `file-write` | `rm`, `mv`, `cp`, `tee`, `touch`, `mkdir`, `ln`, `chmod`, `dd`, …; `sed -i`, `perl -i`; `find -delete`, `-fprint` |
| `redirect` | output redirection into anything but `/dev/null`, `/dev/stdout`, `/dev/stderr` or a descriptor |
| `git` | `git push`, `commit`, `reset`, `checkout`, `switch`, `restore`, `merge`, `rebase`, `pull`, `fetch`, `clone`, `add`, `rm`, `clean`, `stash` (but `list`, `show`), … |
| `gh` | mutating `gh` subcommands: `pr merge/create/edit/comment/…`, `issue create/comment/…`, `release`, `repo`, `secret`, `workflow run`, … |
| `http-method` | `gh api`, `curl`, `wget`, HTTPie and xh requests whose method is not GET, HEAD, OPTIONS, POST or QUERY, or not literal; uploads (`curl -T`) |
| `graphql` | `gh api graphql` documents containing a mutation, or that cannot be read (`-f`/`-F query=`, `@file`, `--input`, heredoc stdin) |
| `package` | installs, removals and publishes (`npm install`, `pip install`, `brew upgrade`, `cargo publish`, `go install`, `npx`, …) |
| `remote` | `ssh`, `scp`, `sftp`, `rsync`, `mosh`, `telnet` |
| `process-control` | `kill`, `pkill`, `shutdown`, …; service changes through `systemctl`, `launchctl`, `service` |
| `daemon` | changes made by a daemon on the command's behalf: `docker`, `podman`, `kubectl`, `tmux`, `defaults write`, keychain `security`, `osascript`, `emacsclient --eval` |
| `privilege` | `sudo`, `doas`, `su`, `pkexec`, `run0` |
| `uninspectable` | code Henia cannot read: `bash script.sh`, a shell fed by a pipe, `eval` or `sh -c` of a non-literal string, `source`, `curl -K` |
| `dynamic-command` | a command or subcommand name that is not literal (`$TOOL status`, globs) |
| `unparseable` | commands the shell parser rejects |

POST is a read for many APIs (GraphQL queries, search endpoints), so it runs.
Wrappers (`env`, `timeout`, `nice`, `xargs`, `command`, …) are unwrapped and the
wrapped command is checked; `sh -c`, `eval` and heredoc-fed shells are parsed
recursively. General interpreters (`python -c`, `node -e`) are not inspected;
their writes still hit the sandbox, and a project can refuse them by name.

### Configuration

```toml
[preload]
timeout = "10s"        # per preload
output = 8000          # bytes of output kept per preload
unsandboxed = "skip"   # "run" only in the user henia.toml

[[preload.refuse]]
command = "kubectl"
subcommands = ["get"]  # omit to refuse the whole command
reason = "cluster reads stay out of skills"

[[preload.refuse]]
pattern = "api\\.example\\.com/admin"   # regular expression over the preload
reason = "admin endpoints change state"
```

Settings come from the user `henia.toml` (`$XDG_CONFIG_HOME/henia/`) and the
project's `henia.toml`; the project wins for `timeout` and `output`. Refusal
rules from both add to the built-ins; none removes one. Invalid settings or
rules stop all preloads with a note.

### FAS

FAS is optional user policy consulted after Henia's own check. When `fas` is on
PATH, Henia runs `fas eval --harness henia` in the project directory with
`{command, skill, source, tier, caller, cwd}`. A deny prints as
`blocked by fas/<rule>: <reason>`; `ask` arrives as deny; a rewritten command
passes Henia's check again. A failing or unreadable `fas` blocks the preload.

### Rendering

Output keeps the preload's indentation, so a preload in a list item stays in
the item. Notes follow the output: `henia: timed out after 10s`,
`henia: output truncated at 8000 bytes`, `henia: exit status 1`. The output
budget applies to the skill before preloads run.

Code spans and code blocks that merely show the syntax never run: a preload is
only a code span directly after `!` in text, or a fence whose info string is
exactly `!`.

### Projection

A projected skill is loaded by its harness, not by `henia show`, so `henia build`
rewrites each preload into `henia preload --skill <name> -- '<command>'`, which
applies the same check, FAS consultation, sandbox and timeout and prints the
same block. Profiles that run preloads natively (`preloads = true`; Claude) keep
the `!` syntax around that invocation and gain `Bash(henia preload *)` in a
declared `allowed-tools`. Other profiles get a run-first instruction: inline
`` run first: `henia preload …` ``, or a `Run first:` paragraph over a `bash`
block. Source templates no longer branch on the harness to choose between the
two forms.

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
- Unit and scrut: preload detection, including examples that must not run;
  every built-in refusal with its message; GraphQL queries and search POSTs
  that run; a write blocked by the sandbox; timeout, output budget and exit
  notes; configured refusals; FAS deny, rewrite and failure through a stub.
- Acceptance: a fixture library with a canary skill, a projected entry skill and
  FAS hooks in isolated homes; Claude Code and Codex read the canary through
  `henia show`, and a direct read is denied. A library skill's preload prints
  a canary only its command produces, and a preload the project's FAS rules
  deny shows the rule instead of running.
- Tropos: its source installs as `$XDG_DATA_HOME/henia/sources/tropos`, Claude
  projects only the entry skills, and `henia slots --for code` still resolves
  Loqui.
