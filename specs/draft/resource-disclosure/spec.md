---
issue_type: Feature
created: 2026-10-04
status: Accepted
stage: draft
---

# Resource disclosure in henia show

## Goal

A skill's resources (every file in its directory besides `SKILL.md`, such as
`reference/configuration.md`) become discoverable the way skills are: each may
carry frontmatter with a `description`, and `henia show <skill>` ends with a
list of the skill's resources and their descriptions. The agent reads the
skill, sees what else exists, and reads only what it needs, again through
`henia show`.

## Resource frontmatter

- Optional YAML frontmatter, as in `SKILL.md`. `description` is the only field
  Henia reads.
- Without it, a Markdown resource is described by its first `#` heading; other
  files have no description.
- A directory is described by its `README.md` or `index.md`.

## Addressing

- `henia show <skill>/<path>` prints one resource verbatim, without its
  frontmatter, as builds copy resources; `<skill>/<path>#<section>` prints one
  section of a Markdown resource.
- `henia show <skill>/<dir>/` lists the resources under a directory.
- Paths stay inside the skill directory. A symlinked resource may resolve into
  the directory the skill's `SKILL.md` resolves to (sources deployed as links)
  and nowhere else.

## The list

Appended after the full skill (not after a section):

```text
## Resources

Read one with `henia show implement/<path>`:

- `reference/configuration.md`: Implementation Agent Configuration
- `operations/execute.md`: Scope Execution
```

Up to 30 files are listed individually. Beyond that the list shows the files
directly in the requested directory and one entry per subdirectory with its
file count and description, and each subdirectory is listed the same way with
`henia show <skill>/<dir>/`. A level holding a single directory and no files is
skipped, so nesting never costs an empty step.

## Turning it off

`resources.disclosure = false`:

- globally, in the user or project `henia.toml` (`[resources]`; the project
  wins);
- per skill, under `henia:` in `SKILL.md` (`resources: {disclosure: false}`),
  which wins over the global setting in both directions.

Profiles accept `henia.resources.disclosure` and never emit it.

## Not covered

- Built skills keep their resources as files; their `SKILL.md` gets no list.
- Resources are not rendered: no templates, reference rewriting or preloads.
