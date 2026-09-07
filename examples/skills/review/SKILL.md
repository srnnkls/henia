---
name: review
description: Review a code change for correctness and missing coverage.
henia:
  variables:
    priority: high
    review_mode: focused
    checks:
      - Correctness
      - Error handling
      - Relevant test coverage
  targets:
    claude:
      auto_invoke: false
    codex:
      auto_invoke: false
      openai:
        interface:
          display_name: Code review
          short_description: Review correctness and coverage
          default_prompt: "Use $review to review the current changes."
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
