---
name: henia
description: Skills served by Henia beyond those installed here, read on demand with henia show. Use when no installed skill fits the task, or when a skill or instruction names a henia show command.
---

# Henia

Henia serves skills from a library. Read them through Henia, never as files:

- `henia show <skill>` prints a skill rendered for this harness and lists its resources; `--toc` lists its sections.
- Resources are addressed like modules: `henia show git.reference.worktree` reads `reference/worktree.md` of the `git` skill, and `henia show git.reference` lists that folder. These are library addresses, not file paths.
- `#<section>` reads one section of a skill or resource, as in `henia show git.reference.worktree#steps`.
- `henia ls` lists every skill in the library, including other packages'.
- `henia query '<pattern>'` finds headings, paragraphs, code and links across skills and their resources; `henia query --grammar` prints the language.
{{if .}}
These skills are not installed in this harness. When a task fits one, read it with `henia show <skill>` and follow it as if it were installed:

{{range .}}- `{{.Name}}`: {{.Description}}
{{end}}{{end}}
