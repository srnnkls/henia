# Henia

*ἡνία • rein, bridle*

> From Proto-Indo-European *h₂em- ("to grasp"); akin to Latin *ānsa* ("handle")
>
> Pronunciation: /hɛː.ní.a/ → /iˈni.a/

## About

Henia keeps agent skills in hand. You write a skill once, in Markdown, and
Henia compiles it for each harness that runs it: Claude Code, Codex, Gemini,
Cursor, GitHub Copilot, OpenCode and others. Templates fill in what
differs between them. Tool names, directive syntax, frontmatter keys and
sidecar files change with the target, and the skill does not.

A harness lists only a few skills before its prompt fills up, so Henia also
serves skills at runtime. A project declares skill packages as dependencies,
and an agent reads any skill in that library on demand with `henia show`,
rendered for the agent that asks. `henia query` searches every skill at once,
slots let a project plug its own conventions into shared skills, and
`henia lint` checks the whole library with rules written as queries.
[Phora](https://github.com/srnnkls/phora) does the fetching and pinning.

Reach for it when the same skills have to work in more than one harness, when
there are more skills than a harness will list, or when several projects share
skills but each needs its own house rules.

## Installation

### Go

```sh
go install github.com/srnnkls/henia/cmd/henia@latest
```

This needs Go 1.26.5 or newer.

### Prebuilt binaries

Download an archive from the [releases page](https://github.com/srnnkls/henia/releases).
There are builds for macOS, Linux and Windows on x86_64 and arm64, each listed
with its SHA-256 in `checksums.txt`.

### From source

```sh
git clone https://github.com/srnnkls/henia && cd henia
go build -o henia ./cmd/henia
```

Henia fetches packages with Phora. It uses a `phora` on `PATH`, else downloads
the release it pins, once, and checks it against a checksum built into Henia.

## Getting started

Make a project and add [Tropos](https://github.com/srnnkls/tropos), a public
skill package, as a dependency:

```console
$ mkdir notes && cd notes && git init -q
$ henia add tropos --git https://github.com/srnnkls/tropos
Synced 4 package(s) into …/henia/packages/notes-ebe0bd6cc160: gestalt, limen, loqui, tropos
```

Tropos brings three packages of its own, so the library now holds 26 skills.
`henia ls` lists them. Read one section at a time:

```console
$ henia show git#commits
## Commits

Conventional commit format, imperative and lowercase:
…
One logical change per commit; stage specific paths.
```

Search all of them at once. Which skills do the others point to most?

```console
$ henia query '(skill :id ?s) @t (skill :id (not ?s) (link :target ?s) @l)?
>   (group @t (count @l @n))' --sort '-@n' --limit 3
@t  review  skill
@l  50 nodes
@n  50

@t  peer  skill
@l  29 nodes
@n  29

@t  test  skill
@l  15 nodes
@n  15
henia query: 3 of 26 rows shown
```

Every match carries an address that `henia show` reads:

```console
$ henia query '(skill :id "git" (code :lang "bash" :contains "worktree add") @c)' --limit 1
@c  git#quick-reference  L138-142  code  git worktree add ../feat-auth -b feat/auth
henia query: 1 of 3 rows shown
```

### Your first skill

Write a skill in `skills/`, with a template variable and a directive, and
name the harnesses it builds for:

```sh
mkdir -p skills/changelog
cat > skills/changelog/SKILL.md <<'EOF'
---
name: changelog
description: Write a changelog entry for the change on this branch.
henia:
  variables:
    tone: plain
---

# Changelog entry

Read the diff with `!read`, then write one entry in a {{.tone}} voice.

:::instruction{priority=high}
Name what changed for the user, not how the code changed.
:::
EOF
cat >> henia.toml <<'EOF'
[harness.claude]
directives = "xml"

[harness.claude.tools]
read = "Read"

[harness.codex.tools]
read = "read_file"
EOF
```

Build for both harnesses. The library's skills build with yours, 27 skills
twice:

```console
$ henia build
Built 54 artifact(s) in .henia/build
$ sed -n '/^# /,$p' .henia/build/claude/skills/changelog/SKILL.md
# Changelog entry

Read the diff with `Read`, then write one entry in a plain voice.

<instruction priority="high">
Name what changed for the user, not how the code changed.
</instruction>
$ sed -n '/^# /,$p' .henia/build/codex/skills/changelog/SKILL.md
# Changelog entry

Read the diff with `read_file`, then write one entry in a plain voice.

:::instruction{priority="high"}
Name what changed for the user, not how the code changed.
:::
```

Lint catches what would break once deployed:

```console
$ echo 'Follow the [house rules](reference/house-rules.md).' >> skills/changelog/SKILL.md
$ henia lint skills
skills/changelog/SKILL.md:16:13: warning [broken-link] local reference does not exist: reference/house-rules.md
```

The [example package](examples) shows nested directives, harness variables
and OpenAI sidecar metadata in one skill:

```sh
henia build examples --config examples/henia.toml
henia lint examples --config examples/henia.toml --strict
```

## Commands

| Command | Does |
| --- | --- |
| [`add`](docs/runtime.md#dependencies) | declare a skill package dependency and fetch it |
| [`rm`](docs/runtime.md#dependencies) | remove a dependency and its files |
| [`sync`](docs/runtime.md#dependencies) | fetch the declared packages at their locked commits |
| [`update`](docs/runtime.md#dependencies) | move the declared packages to their latest commits |
| [`install`](docs/runtime.md#installing) | write the global packages into harness directories |
| [`ls`](docs/runtime.md) | list the skills in the library |
| [`show`](docs/runtime.md) | print a skill, section or resource, rendered for the caller |
| [`query`](docs/query.md) | match patterns against every skill and its resources |
| [`context`](docs/runtime.md) | print a skill's runtime context for a preload |
| [`slots`](docs/slots.md) | resolve who provides each slot |
| [`preload`](docs/runtime.md#preloads) | run one of a skill's preloads under Henia's sandbox |
| [`lint`](docs/lint-rules.md) | check metadata, references, structure and duplication |
| [`build`](docs/build.md) | compile canonical skills for each harness |

`henia <command> --help` prints the details of any command.

## Concepts

| Term | Meaning |
| --- | --- |
| *skill* | a directory with a `SKILL.md` and its resources, written once for every harness |
| *package* | a tree of `skills/` with the `henia.toml` that renders them |
| *library* | the project's own skills, its dependencies and the global packages |
| *harness* | an agent runtime such as Claude Code or Codex, with its own skill format |
| *profile* | how one harness's frontmatter, layout and sidecar files look |
| *mode* | per harness, whether a skill is compiled in whole (`static`), read on demand (`dynamic`) or both (`hybrid`) |
| *slot* | a named extension point a skill declares and other skills fill |
| *preload* | a command a skill runs when it loads, sandboxed by Henia |
| *lint module* | a Markdown file of lint rules written as queries, with examples that test them |

## Documentation

- [Skill runtime](docs/runtime.md): dependencies, the library, `show`,
  preloads, skill modes and installing into harnesses.
- [Building](docs/build.md): templates, directives, build output and
  configuration.
- [henia query](docs/query.md): the pattern language, also printed by
  `henia query --grammar`.
- [Lint rules](docs/lint-rules.md) and
  [paragraph similarity](docs/paragraph-similarity.md).
- [Slots](docs/slots.md): types, priorities and lineage.
- [Vendor profiles](docs/vendor-profiles.md) and the
  [vendor contracts](docs/vendor-research.md) behind them.
- [Phora integration](docs/phora.md): Henia as a Phora build hook.

## Development

```sh
mise run test               # Go unit tests, then the Scrut suites in tests/
mise run test:unit          # go test ./internal/... ./cmd/...
mise run test:integration   # Scrut suites against a fresh build
mise run test:acceptance    # live harness runs through claude -p and codex exec
go vet ./...
```

The acceptance suites compose slots in real harnesses; see
[slots](docs/slots.md#acceptance-tests). Design notes live under
[specs/draft](specs/draft).

Benchmarks cover parsing, rendering, `henia build` and `henia show` over a
skill corpus, `$HENIA_BENCH_CORPUS` or `~/projects/tropos`, and skip without
one:

```sh
go test -run '^$' -bench . -benchmem ./internal/markup ./internal/build ./internal/cli
```
