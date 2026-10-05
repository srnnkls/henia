# Skill runtime

Henia runs skills. Harnesses list a few entry skills, compiled for them; Henia
serves every other skill from a library when an agent asks for it, rendered for
that agent.

| Concept | Meaning |
|---|---|
| Library | Installed canonical skill packages, as authored |
| Skill package | One tree of `skills/` plus the `henia.toml` that renders them |
| Runtime | Resolves, composes, renders, caches and serves library skills |
| Static skill | Compiled by `henia build` into one harness's skill tree |
| Dynamic skill | Not built for that harness; read with `henia show` |

Whether a skill is static or dynamic is decided per harness in the package's
`henia.toml`, never in the skill.

## Library

A project keeps its own package in `.henia/`:

```
.henia/
  .gitignore     # written by Henia; tracks only the entries below
  henia.toml     # or henia.toml at the repository root, not both
  skills/
  harnesses/
  lint/
  build/  cache/  state/    # ignored
```

A skill package is a project used as a dependency: a folder holding `skills/`,
with its own `henia.toml` and, optionally, lint modules in `.henia/lint/`.

A skill is `<package>:<name>`; the project's own package is `project`. A bare
name resolves to the project skill, else to a dependency's, else to a global
one. When several packages of the same rank define a name, the name is
ambiguous and must be qualified; same-named skills never replace each other
silently. `<skill>#<section>` addresses a heading anchor.

`henia build` compiles the project's skills together with its dependencies'. A
dependency's skills render with its own harness `variables`, `tools` and
`references` as defaults; the project's harness of the same name overrides
them key by key and decides everything else;
`henia lint` lints the project's own files and resolves references against
its dependencies, whose `.henia/lint/` modules join the rule set.

`--package DIR` adds a package for a single runtime command.

## Dependencies

```
henia add <name> (--git URL | --path DIR) [--branch B | --tag T | --rev R] [--root DIR] [--skill S]... [--global]
henia rm <name> [--global]
henia sync [--global]
henia update [--global]
```

`henia add` declares a package in `henia.toml`:

```toml
[dependencies.gestalt]
git = "https://github.com/srnnkls/gestalt.git"
branch = "main"
skills = ["gestalt"]   # optional; every skill when omitted
```

`sync` fetches the declared packages at their locked commits, and the packages
they declare in turn; `update` moves them to the latest commits. Henia hands the
fetching to [Phora](https://github.com/srnnkls/phora) through a phora.toml it
generates under `$XDG_STATE_HOME/henia/packages/<project>/`. It runs
`HENIA_PHORA` when set, else a `phora` on PATH of at least 0.4.1, else the
Phora release it pins, downloaded once into `$XDG_CACHE_HOME/henia/tools/` and
checked against the SHA-256 built into Henia. Releases exist for macOS and
Linux; elsewhere, install Phora or set `HENIA_PHORA`. Phora's lock is kept as
`henia.lock` next to `henia.toml`; commit it. `build` and `lint` sync first when
a declared package is missing.

`henia.local.toml`, next to `henia.toml` and kept out of version control, layers
local settings over it. Its `[dependencies]` entries replace the declared ones
by name, and `link = true` deploys a `path` dependency live, so edits to the
checkout show without another sync:

```toml
# henia.local.toml
[dependencies.loqui]
path = "~/projects/loqui"
link = true
```

While a local entry is active, its lock goes to `henia.local.lock`, and
`henia.lock` keeps the declared revision for everyone else. Ignore both local
files in Git.

Packages land in a store shared by every project, under
`$XDG_DATA_HOME/henia/packages/<project>/<name>/`. `--global` uses the user
`henia.toml` in `$XDG_CONFIG_HOME/henia/` and installs into
`$XDG_DATA_HOME/henia/packages/global/`, which every project reads. Unset `XDG_*`
variables fall back to the platform's directories: `~/.config`, `~/.local/share`
and `~/.local/state` on Linux, `~/Library/Application Support` on macOS.

## Installing

`henia install` writes the global packages into harness directories: static
skills, hybrid heads, agents and harness files, each rendered by the package
that configures that harness, with the other global packages as its
dependencies. It installs every harness an installed package configures whose
home exists on this machine:

| Harness | Home |
|---|---|
| `claude` | `$CLAUDE_CONFIG_DIR`, else `~/.claude` |
| `codex` | `$CODEX_HOME`, else `~/.codex` |
| `pi` | `~/.pi/agent` |
| `omp` | `~/.omp/agent` |

The user `henia.toml` overrides a home, adds a harness Henia knows no home for,
or turns one off, which removes what Henia installed there:

```toml
[install.codex]
path = "~/work/codex"

[install.omp]
enabled = false
```

Every harness also gets Henia's own `henia` skill: how to read the library
through `henia show`, `ls` and `query`, followed by the dynamic skills of all
installed packages. It takes the place of the packages' catalog skills. To leave
it out and keep each package's catalog instead:

```toml
install_henia_skill = false
```

`henia sync --global`, `update --global`, `add --global` and `rm --global`
install afterwards, so a harness copy and the library it reads from come from
the same `henia.lock`. Henia records what it wrote under
`$XDG_STATE_HOME/henia/install/<harness>.json`. It never overwrites a file it
did not write, or one edited since, without `--force`, never writes through a
symlink, and removes files a previous install wrote that the packages no longer
produce.


```
henia ls [--json]
henia show <skill>[/<resource>][#section] [--harness NAME]
henia query '<pattern>...' [--text | --json | --count] [--limit N] [--canonical]
henia context <skill> [--global DIR]...
henia preload --skill <skill> -- <command>
```

- `ls` prints the catalog; `--json` adds each skill's package, digest and
  sections.
- `show` prints a skill or one section, rendered through its package's
  `henia.toml` for the caller: `--harness`, then `HENIA_HARNESS` (`none`
  renders neutrally), then the nearest agent among Henia's ancestor processes,
  then the `AI_AGENT` prefix (`claude-code` is `claude`), `CODEX_THREAD_ID` or
  `CLAUDECODE`. Environment markers come last because outer agents leak them
  into nested ones, and Codex does not set `AI_AGENT`. References to skills that harness projects and the model may
  invoke keep its syntax; others, including projected skills with
  `auto_invoke: false`, render as `henia show <skill>`. Without a caller, rendering is neutral.
  Output over the budget prints the sections instead.
- `query` matches S-expression patterns against every skill and its resources
  and prints each capture with an address `show` reads; see
  [henia query](query.md).
- `context` prints a skill's runtime context for a preload: the providers of the
  [slots](slots.md) it applies and the sections of the library skills it references. It
  never repeats the skill's own text.
- Renders are cached by content under `$XDG_CACHE_HOME/henia/render`;
  preload output never is.
- Runtime commands print problems as text and exit 0, so a preload never aborts.

## Resources

Files next to a skill's `SKILL.md` are its resources. A Markdown resource may
carry frontmatter with a `description` (else its first heading describes it).
`henia show <skill>` ends with a list of the skill's resources and their
descriptions, and `henia show <skill>/<path>` reads one, without frontmatter;
`<skill>/<dir>/` lists a directory. Large skills list directories with file
counts, each readable the same way.

Turn the list off with `resources.disclosure = false`, under `[resources]` in a
user or project `henia.toml`, or for one skill under `henia:` in its frontmatter.

## Preloads

A skill can embed commands with Claude Code's preload syntax, `` !`cmd` ``
inline or a fenced block with the info string `!`:

````markdown
Branch: !`git --no-optional-locks branch --show-current`

```!
gh pr list --limit 5
```
````

`henia show` runs each preload in the project directory and prints the command
above its output. Code spans and blocks that only show the syntax never run, and
an inline preload's `!` must start a line or follow whitespace, as in Claude
Code.

Preloads have no side effects, and Henia enforces that itself:

- a sandbox denies file writes (macOS `sandbox-exec`; Linux Landlock, else
  `bwrap`); reads and network stay open. Without a working sandbox preloads do
  not run, unless the user's own `henia.toml` sets `unsandboxed = "run"`.
- every preload has a timeout.
- commands that change state the sandbox cannot see are refused before they
  run: `git push`, `gh pr merge`, non-GET/POST HTTP methods in `curl`, `wget`
  and `gh api`, GraphQL mutations, package installs, `ssh`, `kill`, `docker`,
  redirection into files, and more. Commands Henia cannot inspect, such as a
  shell fed by a pipe or a non-literal command name, are refused too.

A refusal prints instead of the output:

```text
$ gh pr merge 12
henia: blocked by henia/gh: gh pr merge changes GitHub state
```

Use read-only forms of commands that write as a side effect, for example
`git --no-optional-locks status`.

`henia.toml` (user or project) tunes limits and adds refusals; nothing removes
a built-in one:

```toml
[preload]
timeout = "10s"      # per preload
fas_timeout = "10s"  # per FAS consultation
output = 8000

[[preload.refuse]]
command = "kubectl"
subcommands = ["apply", "delete"]

[[preload.refuse]]
pattern = "api\\.example\\.com/admin"
reason = "admin endpoints change state"
```

Projected skills keep the same guarantees: `henia build` rewrites each preload
to `henia preload --skill <skill> -- '<command>'`. Claude runs that natively in
`!` syntax (and the build adds `Bash(henia preload *)` to a declared
`allowed-tools`); harnesses without native preloads get a run-first `bash`
block instead.

macOS cannot nest one sandbox inside another, so inside Codex's sandbox
Henia's cannot start. Let Codex run `henia show` and `henia preload` outside its
sandbox; Henia then sandboxes each preload itself, and `henia preload` runs only
commands a library skill declares:

```
# ~/.codex/rules/henia.rules
prefix_rule(pattern=["henia", "show"], decision="allow")
prefix_rule(pattern=["henia", "preload"], decision="allow")
```

When [FAS](https://github.com/srnnkls/fas) is on PATH, Henia also asks
`fas eval --harness henia` after its own check, so existing rule sets can
refuse more; its rule name appears as `blocked by fas/<rule>`.

## Skill modes

Each skill has a mode per harness:

| Mode | In the harness | In the library |
|---|---|---|
| `static` | the whole compiled skill | the whole skill |
| `dynamic` | nothing; the catalog skill lists it | the whole skill |
| `hybrid` | a head: frontmatter and its `:::static` blocks | the whole skill |

The package's `henia.toml` sets them per harness:

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

`:::static` blocks in a skill's body mark what a hybrid head carries upfront:
routes, hard rules, the context it preloads. Everything else stays in the
library. A skill read in full keeps the blocks' content without the markers.

```md
# Code

:::static
## Routes

| Argument | Action |
|---|---|
| `review <target>` | Invoke `$review` |
:::

## Review roles

Reference material read on demand.
```

The head renders in the library when the skill is used, so it never drifts
from it:

- In a harness that runs commands when a skill loads, such as Claude Code, the
  head is a single preload of `henia show <skill> --head --digest <revision>`.
- Elsewhere the head holds the rendered `:::static` blocks followed by
  "run first" lines for `henia show <skill> --toc` and `henia context <skill>`.

`henia show <skill> --head` prints the `:::static` blocks, the skill's contents
as `henia show <skill>#<section>` addresses, its resources and the contents of
the skills it references. `--toc` prints only the contents. `--digest` names
the revision the head was built from, and `show` reports when the library holds
another one. A hybrid skill without `:::static` blocks is a launcher: contents
and related contents only.

### Catalog

When a harness has dynamic skills, the build adds the `henia` skill, the
catalog skill: Henia's own skill, from
[internal/defaults/henia/SKILL.md](../internal/defaults/henia/SKILL.md), followed
by each dynamic skill with its description. Set `catalog = false` in `[harness.<name>.skills]`
when the package's own instructions already describe the runtime.

Harnesses truncate skill listings early, so keep about ten skills static or
hybrid per harness.

## Guarding

Library content is meant to be read through the runtime. A FAS rule can deny
direct reads of `.henia/skills` and `henia/packages` paths, as
[Tropos's `henia_library.cue`](https://github.com/srnnkls/tropos/blob/main/rules/fas/security/henia_library.cue)
does for Read, Grep, Glob and reading Bash commands.
