---
name: henia
description: Skills served by Henia beyond those installed here, read on demand with henia show. Use when no installed skill fits the task, or when a skill or instruction names a henia show command.
---

# Henia

Henia serves skills from a library. Read them through Henia, never as files:

- `henia show <skill>` prints a skill rendered for this harness; `<skill>#<section>` reads one section, `<skill>/<path>` one of its resources, and `--toc` lists its sections.
- `henia ls` lists every skill in the library, including other packages'.
- `henia query '<pattern>'` finds headings, paragraphs, code and links across skills and their resources; `henia query --grammar` prints the language.
{{if .}}
These skills are not installed in this harness. When a task fits one, read it with `henia show <skill>` and follow it as if it were installed:

{{range .}}- `{{.Name}}`: {{.Description}}
{{end}}{{end}}
