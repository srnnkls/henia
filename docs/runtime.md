# Skill runtime

Henia runs skills. Harnesses list a few entry skills, compiled for them; Henia
serves every other skill from a library when an agent asks for it, rendered for
that agent.

| Concept | Meaning |
|---|---|
| Library | Installed canonical skill sources, as authored |
| Source | One tree of `skills/` plus the `henia.toml` that renders them |
| Runtime | Resolves, composes, renders, caches and serves library skills |
| Static skill | Compiled by `henia build` into one harness's skill tree |
| Dynamic skill | Not built for that harness; read with `henia show` |

Whether a skill is static or dynamic is decided per harness in the source's
`henia.toml`, never in the skill.

## Library

A project keeps its source in `.henia/`:

```
.henia/
  .gitignore     # written by Henia; tracks only the entries below
  henia.toml     # or henia.toml at the repository root, not both
  skills/
  harnesses/
  lint/
  sources/       # dependencies; ignored
  build/  cache/  state/    # ignored
```

A source is a project used as a dependency: a folder holding `skills/`, with its
own `henia.toml` and, optionally, lint modules in `.henia/lint/`. A project's dependencies are the sources in
its `.henia/sources/<source>/`, and theirs in turn, so a project sees every
skill it depends on transitively. Anything that places a folder can provide
them: a copy, a symlink, a git submodule, or a deployer such as Phora. Henia
never installs.

`$XDG_DATA_HOME/henia/sources/<source>/` (default `~/.local/share/henia/sources`)
holds sources installed for every project; `--source DIR` adds one for a single
command.

A skill is `<source>:<name>`; the project source is `project`. A bare name
resolves to the project skill, else to a dependency's, else to an installed
one. When several sources of the same rank define a name, the name is
ambiguous and must be qualified; same-named skills never replace each other
silently. `<skill>#<section>` addresses a heading anchor.

`henia build` compiles the project's skills together with its dependencies'. A
dependency's skills render with its own harness `variables`, `tools` and
`references` as defaults; the project's harness of the same name overrides
them key by key and decides everything else;
`henia lint` lints the project's own files and resolves references against
its dependencies, whose `.henia/lint/` modules join the rule set.

## Runtime

```
henia ls [--json]
henia show <skill>[/<resource>][#section] [--harness NAME]
henia query '<pattern>...' [--text | --json | --count] [--limit N] [--canonical]
henia context <skill> [--global DIR]...
henia preload --skill <skill> -- <command>
```

- `ls` prints the catalog; `--json` adds each skill's source, digest and
  sections.
- `show` prints a skill or one section, rendered through its source's
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
  slots it applies and the sections of the library skills it references. It
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

## Static and dynamic skills

A skill is static or dynamic per harness. `henia build` compiles static skills
into the harness's skill tree, where the harness lists and loads them; dynamic
skills are not built and are read with `henia show`. The source's `henia.toml`
decides, per harness, with one of two lists:

```toml
[harness.claude.skills]
dynamic = ["debate", "loqui"]       # these are dynamic; the rest are built
# static = ["code", "implement"]    # or: only these are built

```

Setting both is an error; setting neither builds every skill. References to
dynamic skills render as `henia show` commands.

When a harness has dynamic skills, the build adds a generated `henia` skill, the
catalog skill, that names each one with its description and tells the agent to
read it with `henia show`. Set `catalog = false` in `[harness.<name>.skills]`
when the source's own instructions already describe the runtime.

Harnesses truncate skill listings early, so keep about ten skills static per
harness.

## Guarding

Library content is meant to be read through the runtime. A FAS rule can deny
direct reads of `.henia/skills` and `henia/sources` paths, as
[Tropos's `henia_store.cue`](https://github.com/srnnkls/tropos/blob/main/rules/fas/security/henia_store.cue)
does for Read, Grep, Glob and reading Bash commands.
