---
created: 2026-11-13
status: done
issue_type: Task
---

# query-paths

## Goal

`henia query --path <dir|file>` runs hq over an arbitrary Markdown tree instead of the library, so any documentation tree can be queried with the same patterns and output.

## Non-Goals

- A standalone `hq` binary or a public Go package.
- Mixing `--path` documents with library skills in one corpus.
- Rendering `--path` files for a harness; they are read as authored.
- Changing which files `henia lint` collects or how its corpus resolves skill ids.
- Moving the capture address out of `internal/cli`.

## Invariants

| ID | Invariant | Check |
|----|-----------|-------|
| INV-1 | Without `--path`, `henia query` reads the library corpus exactly as today | existing `internal/cli/cli_test.go` query tests pass unchanged |
| INV-2 | `henia query` always exits successfully; problems print as text | `TestQueryPath*` asserts a nil error for every message case |
| INV-3 | `henia lint` collects the same files and resolves the same skill ids for the same arguments | existing `internal/lint` tests pass unchanged |
| INV-4 | One corpus builder: `--path` documents go through `library.Documents` | `TestQueryPathSkillGrouping` matches a skill-owned resource, which only `library.Documents` builds |

## Conventions

| Axis | Resolution | Exemplar |
|------|------------|----------|
| Data & types | `library.Document` per collected file; `Kind: "skill"` for `SKILL.md`, empty otherwise; optional `ID` overrides the skill id | `internal/lint/module.go:503` |
| Errors | Query problems print as `henia query: …` on stdout and return nil | `internal/cli/query.go:50-54` |
| Effects | Directory walk builds on `artifact.Walk`; Git ignore lookup moves with it | `internal/artifact/walk.go:14` |
| State | n/a | n/a |
| Dependencies & boundaries | The walker and `gitIgnored` live in `internal/artifact`; `internal/lint` and `internal/cli` both call it | `internal/artifact/walk.go:14` |
| Naming | Flag `--path`, repeatable, like `--package` | `internal/cli/runtime.go:35-52` |
| Config & tooling | n/a | n/a |
| Testing | CLI tests through `newQueryCommand` with fixture trees under `t.TempDir()`; walker tests in `internal/artifact` | `internal/cli/cli_test.go:139-150` |
| Prose & output | Docs follow `docs/query.md` voice; one console example | `docs/query.md:1-30` |

## Decisions

- AD-1: Input is a repeatable `--path` flag; rejected positional paths: the pattern stays the only positional argument.
- AD-2: `--path` replaces the library corpus and skips opening the library; rejected adding paths beside library skills: skill ids from two sources would collide.
- AD-3: A directory yields every `.md` file, skipping dot entries, git-ignored files and the directories `collect` skips, and a file argument is always read; rejected lint's artifact filter: it drops the plain Markdown this command exists to query.
- AD-4: `SKILL.md` directories become `skill` nodes owning their resources, other files corpus-level `kind: document` files; rejected a flat tree: skill-scoped patterns would stop matching.
- AD-5: The walk moves from `collect` into `internal/artifact` as one exported walker taking a directory filter and a file filter, with `gitIgnored` beside it, passing each filter the root and applying it to the root too; rejected exporting it from `internal/lint`: query would depend on the lint package for filesystem traversal.
- AD-6: Skills whose directory names collide get their collected directory path as id and ref; rejected rejecting the run: a tree with two same-named skills is ordinary input.
- AD-7: The caller sets a colliding skill's id through `library.Document.ID`; rejected qualifying inside `Documents`: lint's corpus would change its skill ids.
- AD-8: Library-only flags beside `--path` and any missing `--path` stop the run with a message; rejected ignoring them: a silently ignored flag or path reads as an empty result.
- AD-9: A root skill's name comes from `library.Document.Name`, which replaces only the directory-name key; rejected reusing `Document.ID`: a set ID drops the frontmatter-name key, so links to the root skill stop resolving.

## Context

### Key Files

| File | Lines | Role |
|------|-------|------|
| `internal/cli/query.go` | 28-100 | Command, flags, corpus choice |
| `internal/cli/query.go` | 295-308 | `address`: empty outside a skill today |
| `internal/library/corpus.go` | 59-64 | `Document`: gains `ID` |
| `internal/library/corpus.go` | 98-157 | `Documents`: corpus from supplied paths, skills keyed by id |
| `internal/library/corpus.go` | 221-225 | `Corpus.file`: size and encoding limits |
| `internal/lint/lint.go` | 101-198 | `collect`, `artifactKind`, `gitIgnored`; walk and `gitIgnored` move out |
| `internal/artifact/walk.go` | 14-43 | `Walk`: home of the shared walker |
| `docs/query.md` | 1-60 | Query documentation |

### Constraints

- Files over 1 MiB, invalid UTF-8 or containing NUL stay out of the corpus, as in `Corpus.file`.

## Requirements

### Functional Requirements

- FR-1: `henia query PATTERN --path P…` MUST build its corpus only from the files collected under each `P`, without opening the library.
- FR-2: A directory `P` MUST yield every regular `.md` file below it; dot entries, git-ignored paths, `.git`, `node_modules` and `vendor` MUST be skipped below `P`, never `P` itself.
- FR-3: A file `P` MUST be read regardless of extension or location; when the size or encoding limits drop it, `henia query: <P>: omitted: over 1 MiB, invalid UTF-8 or contains NUL` MUST print, and when it cannot be read, `henia query: <P>: omitted: <read error>`; the query runs over the rest. The message prints whichever argument collected the file.
- FR-4: A directory holding `SKILL.md` MUST appear as a `skill` node owning the Markdown below it as `resource` files; its id and ref are the directory name, for a skill at the root the base name of the absolute root, keeping its frontmatter-name key.
- FR-5: A file's collected path MUST be its root as given, cleaned, joined with the path below the root, slash-separated; a capture outside any skill prints that path as its address, plus `#section` when inside a section.
- FR-6: When two or more collected skills share a directory name, each of them MUST take its collected directory path (FR-5 form) as id and ref, and neither is keyed by the bare name.
- FR-7: A file reached through several `--path` arguments MUST be collected once, keyed by absolute path, with the collected path of the first argument in command-line order.
- FR-8: Any missing `--path` MUST print `henia query: <path>: no such file or directory` and run no query.
- FR-9: `--path` beside `--project`, `--package`, `--harness` or `--canonical` MUST print `henia query: --path replaces the library; drop --<flag>` and run no query.

### Technical Requirements

- TR-1: `collect` MUST keep its current results, root handling included, by delegating to the shared walker with its artifact filters; query's directory filter exempts the root.
- TR-2: `library.Documents` MUST use `Document.ID` as skill id and ref when set, keying the skill only by it; without `ID` it keeps today's directory-name and frontmatter-name keys.
- TR-3: `library.Documents` MUST count the top directory of a relative path (`.`) when it searches a document's owning skill.
- TR-4: `library.Documents` MUST use `Document.Name` in place of the directory name when set, keeping the frontmatter-name key; `library.Corpus` MUST report each file it drops with its cause, and query's omitted messages come from that report.

## Acceptance Criteria

- [ ] AC-1 (FR-1, FR-2): Given `notes/a.md` with `# Alpha` and `notes/.hidden/b.md` with `# Beta`, when `henia query '(heading) @h' --path notes` runs, then one row prints, for `Alpha`, and none for `Beta`.
- [ ] AC-2 (FR-2): Given the working directory holds `a.md` with `# Alpha`, when `henia query '(heading) @h' --path .` runs, then the `Alpha` row prints.
- [ ] AC-3 (FR-4): Given `tree/tool/SKILL.md` and `tree/tool/ref/x.md`, when `henia query '(skill :id "tool" (file :path "ref/x.md")) @s' --path tree` runs, then one row prints.
- [ ] AC-4 (FR-5): Given `docs/guide.md` with section `## Usage` holding a bash block, when `henia query '(code :lang "bash") @c' --path ./docs` runs, then the row's address is `docs/guide.md#usage`.
- [ ] AC-5 (FR-3): Given `misc/log.txt`, when `henia query '(paragraph) @p' --path misc/log.txt` runs, then its paragraphs match; given a named file over 1 MiB, the omitted message prints.
- [ ] AC-6 (FR-6): Given `tree/a/tool/SKILL.md` and `tree/b/tool/SKILL.md`, when `henia query '(skill) @s' --path tree` runs, then two rows print with ids `tree/a/tool` and `tree/b/tool`, and `(skill :id "tool")` matches nothing.
- [ ] AC-7 (FR-7): Given `docs/a.md`, when `henia query '(file) @f' --path docs --path docs/a.md` runs, then one row prints.
- [ ] AC-8 (FR-8, FR-9): Given one missing path among several, or `--project`, `--package`, `--harness` or `--canonical` beside `--path`, then the stated message prints, no rows print, and the command returns nil.
- [ ] AC-9 (INV-1, INV-3, TR-1): Existing query and lint tests pass unchanged, and a lint test pins that `henia lint vendor` still yields nothing.
- [ ] AC-10 (FR-4, TR-3): Given the working directory `tool/` holds `SKILL.md` and `ref/x.md`, when `henia query '(skill :id "tool" (file :path "ref/x.md")) @s' --path .` runs from it, then one row prints.
- [ ] AC-11 (FR-3, TR-4): Given `docs/a.md` and `docs/big.md` over 1 MiB, when `henia query '(heading) @h' --path docs --path docs/big.md` runs, then the omitted message for `docs/big.md` prints.
- [ ] AC-12 (FR-4, AD-9, TR-4): Given the working directory `tool/` holds `SKILL.md` with frontmatter `name: hammer` and `../other/SKILL.md` links the `hammer` skill, when `henia query '(skill :id "other" (reaches (skill) @t))' --path . --path ../other` runs from it, then one row prints.

## Verification

- `go test ./internal/artifact/... ./internal/cli/... ./internal/lint/... ./internal/library/...`: walker, path tests and existing behavior.
- `henia query '(heading :level 1) @h' --path docs`: manual run over this repo's docs.
- `henia lint scopes/draft/query-paths/scope.md`: scope structure, run from the tropos project.
