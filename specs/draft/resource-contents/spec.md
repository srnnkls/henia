---
issue_type: Feature
created: 2026-10-04
status: Proposed
stage: draft
---

# Resource contents in henia show

## Goal

A skill's resources (every file in its directory besides `SKILL.md`, such as
`reference/configuration.md` or `operations/execute.md`) become discoverable
the way skills are: each may carry frontmatter with a `description`, and
`henia show <skill>` ends with a contents list of the skill's resources and
their descriptions. The agent reads the skill, sees what else exists, and
reads only the resources it needs, each again through `henia show`.

## Resource frontmatter

- Optional, YAML, same delimiters as `SKILL.md`. `description` is the only
  field Henia reads; others are kept for the source's own use.
- `henia show` strips it from the printed resource. `henia build` strips it
  from projected copies only when the profile asks (open question below).
- Resources without frontmatter appear in the contents list without a
  description; Markdown resources fall back to their first heading.

## Addressing

`henia show <skill>/<path>` prints one resource, rendered like the skill
(templates, references, preloads); `<skill>/<path>#<section>` one of its
sections. Paths are relative to the skill directory and cannot leave it.

## Contents list

Appended after the skill body (or after a shown resource, for the resources
in that resource's directory and below):

```text
Resources (read with henia show):
  implement/reference/configuration.md  Configuration keys and defaults
  implement/operations/execute.md       How a task runs, gate by gate
```

Nested directories nest the list, so a skill with many resources stays
scannable. The list counts toward the output budget; over it, only
directories and their counts print.

## Turning it off

- Globally: a user or project `henia.toml` setting.
- Per skill: a `henia:` frontmatter setting in `SKILL.md`.

Names to settle: the setting (`[show] contents = false`?) and the frontmatter
key (`henia.contents: false`?).

## Open questions

- Should static (built) skills get the same list written into their
  `SKILL.md` at build time?
- Should projected resources keep or drop their frontmatter?
