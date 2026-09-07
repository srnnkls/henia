---
name: review
description: Review a code change for correctness and missing coverage.
priority: high
review_mode: focused
checks:
  - Correctness
  - Error handling
  - Relevant test coverage
---

# Code review

Perform a {{.review_mode}} review using `!read` to inspect changed files.

:::instruction{priority={{.priority}}}
Explain each finding with evidence and a concrete failure scenario.

:::note{#scope .guidance}
Concentrate on :term[behavior]{meaning="observable outcomes"}.
:::
:::

## Checks

{{range .checks}}- {{.}}
{{end}}

See the [review guide](reference/guide.md) for reporting conventions.
