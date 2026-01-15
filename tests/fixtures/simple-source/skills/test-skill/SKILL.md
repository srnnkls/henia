---
name: test-skill
description: Test skill for integration tests
model: "{{.model_strong}}"
allowed_tools:
  - bash
  - read
user-invocable: true
---

# Test Skill

This is a test skill for henia integration testing.

Use `$code-review` after implementation.
Run `!bash` to execute tests.

## Workflow

1. Write code
2. Review with skill reference
3. Test with tool reference
