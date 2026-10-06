# Skill runtime

A harness lists a few entry skills, compiled for it. Henia serves every other
skill from a library when an agent asks for it, rendered for that agent.

In the project from [Getting started](../README.md#getting-started), which
added Tropos:

```console
$ henia ls | cut -f1,2 | head -3
gestalt	dependency
limen	dependency
loqui	dependency

$ henia show git.reference

## Resources

Read one with `henia show <address>`:

- `git.reference.branching`: Branch Management
- `git.reference.commands`: Modern Git Commands
- `git.reference.history`: Commit and History Management
- `git.reference.worktree`: Git Worktrees
```

| Term | Meaning |
|---|---|
| *library* | the installed skill packages, as authored |
| *skill package* | one tree of `skills/` plus the `henia.toml` that renders them |
| *static skill* | compiled by `henia build` into one harness's skill tree |
| *dynamic skill* | not built for that harness; read with `henia show` |
| *hybrid skill* | a short head in the harness, the rest read with `henia show` |

A package's `henia.toml` decides per harness which skills are static, dynamic
or hybrid. The skill itself never does.

## Library

A project keeps its own package in `.henia/`. The `changelog` skill above is
one file:

```bash
mkdir -p .henia/skills/changelog
cat > .henia/skills/changelog/SKILL.md <<'EOF'
---
name: changelog
description: Draft release notes from the commits since the last tag.
---

# Changelog

Last tag: !`git describe --tags --abbrev=0`

Group the commits by type and follow `$git` for the commit format.
EOF
```

````console
$ git add .henia && git commit -qm 'Add changelog skill' && git tag v0.3.0
$ henia show changelog
# Changelog

Last tag:
```text
$ git describe --tags --abbrev=0
v0.3.0
```

Group the commits by type and follow `henia show git` for the commit format.
````

The full layout:

```
.henia/
  .gitignore     # written by Henia; tracks only the entries below
  henia.toml     # or henia.toml at the repository root, not both
  henia.lock
  skills/
  harnesses/
  lint/
  build/  cache/  state/    # ignored
```

A skill package is a project used as a dependency: a folder holding `skills/`,
its own `henia.toml` and, optionally, lint modules in `.henia/lint/`.

A skill's full name is `<package>:<name>`, and the project's own package is
`project`. A bare name resolves to the project's skill, else a dependency's,
else a global one. Two packages of the same rank that define one name make it
ambiguous, and Henia asks for the qualified name. Same-named skills never
replace each other silently. `<skill>#<section>` addresses a heading anchor,
as in `henia show 'tropos:git#commits'`. Quote it in a shell: zsh treats `#` as
a glob operator. Every address Henia prints comes quoted when it needs it.

A dependency hides a global package of the same name, so a project that pins
its own Tropos reads only that one.

`henia build` compiles the project's skills together with its dependencies'. A
dependency's skills render with its own harness `variables`, `tools` and
`references` as defaults. The project's harness of the same name overrides them
key by key and decides everything else. `henia lint` lints the project's own
files and resolves references against its dependencies, whose `.henia/lint/`
modules join the rule set.

Runtime commands take `--project DIR` to read another project's library, and
`--package DIR` to add a package for one command.

## Dependencies

```
henia add <name> (--git URL | --path DIR) [--branch B | --tag T | --rev R] [--root DIR] [--skill S]... [--global] [--no-sync]
henia rm <name> [--global]
henia sync [--global]
henia update [--global]
```

`henia add` records the package in `henia.toml` and fetches it. `--no-sync`
only records it.

```toml
[dependencies.gestalt]
git = "https://github.com/srnnkls/gestalt.git"
branch = "main"
skills = ["gestalt"]   # optional; every skill when omitted
```

`sync` fetches the declared packages at their locked commits, along with the
packages they declare. `update` moves them to the latest commits. `build` and
`lint` sync first when a declared package is missing.

[Phora](https://github.com/srnnkls/phora) does the fetching, from a
`phora.toml` that Henia generates under `$XDG_STATE_HOME/henia/packages/<project>/`.
Henia runs `HENIA_PHORA` when it is set, else a `phora` 0.4.1 or newer on PATH,
else the Phora release it pins. That release downloads once into
`$XDG_CACHE_HOME/henia/tools/` and is checked against a SHA-256 built into
Henia. Pinned releases exist for macOS and Linux. On other platforms, install
Phora or set `HENIA_PHORA`. Phora's lock is `henia.lock`, next to
`henia.toml`; commit it.

`henia.local.toml` sits next to `henia.toml`, stays out of version control, and
layers local settings over it. Its `[dependencies]` entries replace the
declared ones by name. `link = true` deploys a `path` dependency live, so edits
to the checkout show without another sync:

```toml
# henia.local.toml
[dependencies.loqui]
path = "~/projects/loqui"
link = true
```

While a local entry is active, its lock goes to `henia.local.lock` and
`henia.lock` keeps the declared revision for everyone else. Ignore both local
files in Git.

Packages live in a store under
`$XDG_DATA_HOME/henia/packages/<project>/<name>/`, where `<project>` is the
project directory's name plus a hash of its path. `--global` uses the user
`henia.toml` in `$XDG_CONFIG_HOME/henia/` and installs into
`$XDG_DATA_HOME/henia/packages/global/`, which every project reads. Unset
`XDG_*` variables fall back to the platform's directories: `~/.config`,
`~/.local/share` and `~/.local/state` on Linux, `~/Library/Application Support`
on macOS.

## Installing

`henia install` writes the global packages into harness directories:

```console
$ henia add tropos --global --git https://github.com/srnnkls/tropos
Synced 4 package(s) into ~/.local/share/henia/packages/global: gestalt, limen, loqui, tropos
Installed 24 file(s) for claude into ~/.claude
```

Each harness gets its static skills, hybrid heads, agents and harness files,
rendered by the package that configures that harness, with the other global
packages as its dependencies. Henia installs every harness that an installed
package configures and whose home exists on this machine:

| Harness | Home |
|---|---|
| `claude` | `$CLAUDE_CONFIG_DIR`, else `~/.claude` |
| `codex` | `$CODEX_HOME`, else `~/.codex` |
| `pi` | `~/.pi/agent` |
| `omp` | `~/.omp/agent` |

`--harness claude,codex` narrows the run. The user `henia.toml` moves a home,
adds a harness Henia knows no home for, or turns one off, which removes what
Henia installed there:

```toml
[install.codex]
path = "~/work/codex"

[install.omp]
enabled = false
```

Every harness also gets Henia's own `henia` skill. It explains how to read the
library through `henia show`, `ls` and `query`, then lists the dynamic skills
of all installed packages, and it takes the place of the packages' catalog
skills. To keep each package's catalog:

```toml
install_henia_skill = false
```

`henia sync --global`, `update --global`, `add --global` and `rm --global`
install afterwards, so a harness copy and the library it reads come from the
same `henia.lock`. Henia records what it wrote in
`$XDG_STATE_HOME/henia/install/<harness>.json` and removes files a previous
install wrote that the packages no longer produce. It never writes through a
symlink, and it leaves alone any file it did not write or that was edited
since:

```console
$ echo '# mine' >> ~/.claude/skills/git/SKILL.md
$ henia install
Error: install claude: 1 file(s) in ~/.claude were not installed by henia or were edited since, among them skills/git/SKILL.md; move them away or pass --force
```

## Reading skills

```
henia ls [--json]
henia show <skill>[.<module>…][#section] [--head | --toc] [--harness NAME]
henia query '<pattern>...' [--text | --json | --count] [--sort KEY] [--limit N] [--canonical]
henia context <skill>
henia preload --skill <skill> -- <command>
```

`ls` prints the catalog: name, tier and description. `--json` adds each
skill's package, path, digest and sections.

`show` prints a skill or one section, rendered for the caller through its
package's `henia.toml`. The same skill reads differently in two harnesses:

```console
$ henia show review --harness none | grep 'Commit SHA'
| Commit SHA | Invoke `henia show code` with `review --rev <sha>` |

$ henia show review --harness codex | grep 'Commit SHA'
| Commit SHA | Invoke `$code` with `review --rev <sha>` |
```

A reference to a skill the harness projects and lets the model invoke keeps
that harness's syntax. Every other reference renders as `henia show <skill>`,
including projected skills with `auto_invoke: false`.

The caller is, in order:

1. `--harness`;
2. `HENIA_HARNESS`, where `none` renders neutrally;
3. the nearest agent among Henia's ancestor processes;
4. the `AI_AGENT` prefix (`claude-code` is `claude`), then `CODEX_THREAD_ID` or
   `CLAUDECODE`.

Environment markers come last because outer agents leak them into nested ones,
and Codex does not set `AI_AGENT`. Without a caller, rendering is neutral.
Output over the budget prints the sections instead, each readable on its own.

`query` matches S-expression patterns against every skill and its resources,
and prints each capture with an address `show` reads. See
[henia query](query.md).

`context` prints a skill's runtime context for a preload: the providers of the
[slots](slots.md) it applies and the sections of the library skills it
references. It never repeats the skill's own text:

```console
$ henia context code | head -4
Skill implement: Strict delegated RED → GREEN → review workflow. Use only when explicitly invoked for a task, scope, verification, or debugging.
- Pre-loaded Context: `henia show 'implement#pre-loaded-context'`
- Strict Implementation: `henia show 'implement#strict-implementation'`
  - Routes: `henia show 'implement#routes'`
```

Renders are cached by content under `$XDG_CACHE_HOME/henia/render`; preload
output never is. Runtime commands print problems as text and exit 0, so a
preload never aborts.

## Resources

Files next to a skill's `SKILL.md` are its resources, addressed like modules in
a programming language. The skill is the root, directories are namespaces, and
Markdown files are modules without the extension:

| Address | Reads |
|---|---|
| `git` | the skill |
| `git.reference` | the list of `reference/` |
| `git.reference.worktree` | `reference/worktree.md`, without frontmatter |
| `git.reference.worktree#steps` | one section of it |
| `tropos:git.reference.worktree` | the same, from the `tropos` package |

A Markdown resource may carry frontmatter with a `description`; otherwise its
first heading describes it. `henia show <skill>` ends with the skill's
resources and their descriptions. Large skills list directories with file
counts, each readable the same way.

A name that is both a file and a directory, such as `reference.md` beside
`reference/`, is ambiguous and reported. Files that are not modules, such as
scripts or names with dots, are read by path: `henia show pr/scripts/pr-context`.

To turn the list off, set `resources.disclosure = false` under `[resources]` in
a user or project `henia.toml`, or under `henia:` in one skill's frontmatter.

## Preloads

A skill embeds commands with Claude Code's preload syntax: `` !`cmd` `` inline,
or a fenced block whose info string is `!`:

````markdown
Branch: !`git --no-optional-locks branch --show-current`

```!
gh pr list --limit 5
```
````

`henia show` runs each preload in the project directory and prints the command
above its output, as for `changelog` [above](#library). Code spans and blocks
that only show the syntax never run. An inline preload's `!` must start a line
or follow whitespace, as in Claude Code.

Preloads have no side effects, and Henia enforces that itself:

- A sandbox denies file writes: `sandbox-exec` on macOS, Landlock or else
  `bwrap` on Linux. Reads and network stay open. Without a working sandbox,
  preloads do not run, unless the user's own `henia.toml` sets
  `unsandboxed = "run"`.
- Every preload has a timeout.
- Commands that change state the sandbox cannot see are refused before they
  run: `git push`, `gh pr merge`, non-GET/POST HTTP methods in `curl`, `wget`
  and `gh api`, GraphQL mutations, package installs, `ssh`, `kill`, `docker`,
  redirection into files, and more. So are commands Henia cannot inspect, such
  as a shell fed by a pipe or a non-literal command name.

A refusal prints in place of the output. Given a skill that tries both:

```bash
mkdir -p .henia/skills/ship
cat > .henia/skills/ship/SKILL.md <<'EOF'
---
name: ship
description: Merge the release PR.
---

# Ship

!`gh pr merge 12`

!`echo hi > notes.txt`
EOF
```

````console
$ henia show ship
# Ship

```text
$ gh pr merge 12
henia: blocked by henia/gh: gh pr merge changes GitHub state
```

```text
$ echo hi > notes.txt
henia: blocked by henia/redirect: redirects output into notes.txt
```
````

Use read-only forms of commands that write as a side effect, such as
`git --no-optional-locks status`.

A user or project `henia.toml` tunes the limits and adds refusals. Nothing
removes a built-in one:

```toml
[preload]
timeout = "10s"      # per preload
output = 8000

[[preload.refuse]]
command = "kubectl"
subcommands = ["apply", "delete"]

[[preload.refuse]]
pattern = "api\\.example\\.com/admin"
reason = "admin endpoints change state"
```

Projected skills keep the same guarantees. `henia build` rewrites each preload
to `henia preload --skill <skill> -- '<command>'`, which runs only commands that
skill declares:

````console
$ henia preload --skill changelog -- 'git push'
```text
$ git push
henia: blocked by henia/not-a-preload: changelog declares no such preload
```
````

Henia's own read commands stay as they are, with no wrapper:
`henia slots --markdown|--inline …`, `henia show <skill> --head|--toc` and
`henia context <skill>`. They are what `:slot[…]`, `:contents[]`, `:related[]`
and a hybrid head render to.

Claude runs preloads natively in `!` syntax, and the build adds the matching
`Bash(henia preload *)`, `Bash(henia slots *)`, `Bash(henia show *)` or
`Bash(henia context *)` to a declared `allowed-tools`. Harnesses without native
preloads get a run-first line instead, or "the output of" a command that sits
inside a sentence. `henia show` resolves all three cases itself.

macOS cannot nest one sandbox inside another, so Henia's sandbox cannot start
inside Codex's. Let Codex run Henia's commands outside its sandbox, and Henia
sandboxes each preload itself:

```
# ~/.codex/rules/henia.rules
prefix_rule(pattern=["henia", "show"], decision="allow")
prefix_rule(pattern=["henia", "slots"], decision="allow")
prefix_rule(pattern=["henia", "context"], decision="allow")
prefix_rule(pattern=["henia", "preload"], decision="allow")
```

The user `henia.toml` can name a policy command that Henia asks after its own
check, so your own rules refuse more:

```toml
[preload]
policy = "gate check --caller henia"
policy_timeout = "10s"
```

The command is split into words like a shell would, without running a shell,
and reads `{command, skill, package, tier, caller, cwd}` as JSON on stdin. It
answers `{"decision": "allow" | "deny", "command", "rule", "reason"}`; a deny
prints as `blocked by gate/<rule>: <reason>`, and a rewritten command passes
Henia's check again. A policy that fails, times out or answers anything else
blocks the preload. Without `policy`, Henia asks no one. A project `henia.toml`
cannot set one.

## Skill modes

Each skill has a mode per harness:

| Mode | In the harness | In the library |
|---|---|---|
| `static` | the whole compiled skill | the whole skill |
| `dynamic` | nothing; the catalog skill lists it | the whole skill |
| `hybrid` | a head: frontmatter and its `:::static` blocks | the whole skill |

The package's `henia.toml` sets them:

```toml
[harness.claude.skills]
default = "dynamic"                 # static, dynamic or hybrid
hybrid = ["code", "implement"]
static = ["peer"]
dynamic = []
```

A skill is listed in at most one mode. Without `default`, listing `static` or
`hybrid` skills makes the rest dynamic, and listing none builds every skill
static. References to dynamic skills render as `henia show` commands.

### Hybrid skills

`:::static` blocks mark what a hybrid head carries upfront: routes, hard rules,
the context it preloads. Everything else stays in the library, and a skill read
in full keeps the blocks' content without the markers.

```bash
mkdir -p .henia/skills/release
cat > .henia/skills/release/SKILL.md <<'EOF'
---
name: release
description: Cut a release. Use when tagging a version or publishing notes.
---

# Release

:::static
## Routes

| Argument | Action |
|---|---|
| `notes` | Draft notes with `$changelog` |
| `tag <version>` | Tag after the checks in [Checklist](#checklist) pass |
:::

## Checklist

1. CI is green on `main`.
2. The changelog entry is merged.
EOF
cat >> henia.toml <<'EOF'

[harness.claude.skills]
hybrid = ["release"]

[harness.codex.skills]
hybrid = ["release"]
EOF
henia build .henia --output build
```

The head renders from the library each time the skill is used, so it never
drifts from it. Claude Code runs commands when a skill loads, so its head is a
single preload:

```console
$ sed -n '/^---$/,$p' build/claude/skills/release/SKILL.md
---
allowed-tools: Bash(henia show *)
description: Cut a release. Use when tagging a version or publishing notes.
name: release
---

!`henia show release --head`
```

Elsewhere the head holds the rendered `:::static` blocks, then run-first lines
for `henia show <skill> --toc` and `henia context <skill>` unless the blocks
place them:

```console
$ sed -n '/^## Routes/,$p' build/codex/skills/release/SKILL.md
## Routes

| Argument | Action |
|---|---|
| `notes` | Draft notes with `henia show changelog` |
| `tag <version>` | Tag after the checks in Checklist (`henia show 'release#checklist'`) pass |

run first: `henia show release --toc`

run first: `henia context release`
```

`henia show <skill> --head` prints the `:::static` blocks, the skill's contents
as `henia show '<skill>#<section>'` commands, its resources and the contents of
the skills it references. `--toc` prints only the contents:

```console
$ henia show release --toc
Read the sections of release as the task needs them:

- Release: `henia show 'release#release'`
  - Routes: `henia show 'release#routes'`
  - Checklist: `henia show 'release#checklist'`
```

A skill places these lists itself with the `:contents[]` and `:related[]`
directives, and `--head` then appends neither. A skill name in the brackets,
as in `:contents[tropos:git]`, lists that skill instead:

```md
:::static
## Routes

…

Sections:

:contents[]
:::
```

Both report a harness copy that `henia install` wrote from an older revision of
the skill. A hybrid skill without `:::static` blocks is a launcher: its head is
the contents and related contents alone.

Relative links to a skill's resources and to other skills render as the
`henia show` commands that read them, wherever the files are not beside the
text: in heads, in `henia show` output, and in static skills that link into a
skill this harness serves dynamically. The `[Checklist](#checklist)` link above
became `henia show 'release#checklist'`. In the same way,
`[review](reference/review.md)` in `scope` becomes
``review (`henia show scope/reference/review.md`)``, and `../git/SKILL.md#slots`
becomes `henia show 'git#slots'`.

### Catalog

When a harness has dynamic skills, the build adds the `henia` catalog skill:
Henia's own skill, from
[internal/defaults/henia/SKILL.md](../internal/defaults/henia/SKILL.md),
followed by each dynamic skill with its description. Set `catalog = false` in
`[harness.<name>.skills]` when the package's own instructions already describe
the runtime.

Harnesses truncate skill listings early, so keep about ten skills static or
hybrid per harness.
