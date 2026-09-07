# Henia Build Tests

## Help

```scrut
$ henia --help
Henia compiles canonical Markdown skills into build artifacts and lints their quality.

Usage:
  henia [command]

Available Commands:
  build       Compile local canonical artifacts for multiple harnesses
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  lint        Check skill metadata, references, duplication and freshness

Flags:
      --config string   Config file path (default "henia.toml")
  -h, --help            help for henia

Use "henia [command] --help" for more information about a command.
```

## Build Help

```scrut
$ henia build --help
Compile local canonical artifacts for multiple harnesses

Usage:
  henia build [source-directory] [flags]

Flags:
      --harness strings   Harnesses to build (comma-separated; default all configured)
  -h, --help              help for build
  -o, --output string     Build output directory (overrides [build].output) (default ".henia/build")

Global Flags:
      --config string   Config file path (default "henia.toml")
```

## Lint Help

```scrut
$ henia lint --help
Check skill metadata, references, duplication and freshness

Usage:
  henia lint [paths...] [flags]

Flags:
      --disable strings   Disable lint rules (comma-separated)
      --format string     Diagnostic output: text or json (default "text")
  -h, --help              help for lint
      --strict            Exit unsuccessfully on warnings as well as errors

Global Flags:
      --config string   Config file path (default "henia.toml")
```

## Build To Claude (Nested Structure)

Compile local fixtures without a source fetch or Git repository.

```scrut
$ rm -rf /tmp/henia-test-build && henia build "$TESTDIR/fixtures/simple-source" --config "$TESTDIR/fixtures/henia-claude.toml" --output /tmp/henia-test-build 2>&1
Built 3 artifact(s) in /tmp/henia-test-build
```

## Verify Claude Directory Structure

```scrut
$ ls -R /tmp/henia-test-build/claude
agents
commands
skills

/tmp/henia-test-build/claude/agents:
test-agent

/tmp/henia-test-build/claude/agents/test-agent:
AGENT.md

/tmp/henia-test-build/claude/commands:
test-command

/tmp/henia-test-build/claude/commands/test-command:
COMMAND.md

/tmp/henia-test-build/claude/skills:
test-skill

/tmp/henia-test-build/claude/skills/test-skill:
SKILL.md
reference

/tmp/henia-test-build/claude/skills/test-skill/reference:
guide.md
```

## Verify Claude Skill Content

```scrut
$ cat /tmp/henia-test-build/claude/skills/test-skill/SKILL.md
---
allowed_tools:
  - bash
  - read
description: Test skill for integration tests
model: opus
name: test-skill
user-invocable: true
---

# Test Skill

This is a test skill for henia integration testing.

Use `/code-review` after implementation.
Run `Bash` to execute tests.

## Workflow

1. Write code
2. Review with skill reference
3. Test with tool reference
```

## Verify Claude Skill References Copied

```scrut
$ cat /tmp/henia-test-build/claude/skills/test-skill/reference/guide.md
# Test Skill Guide

This is a reference guide for the test skill.

## Best Practices

- Always test your code
- Use proper naming conventions
- Write clear documentation
```

## Verify Claude Command Content

```scrut
$ cat /tmp/henia-test-build/claude/commands/test-command/COMMAND.md
---
description: Test command using haiku
name: test-command
---

# Test Command

Execute test command using `Bash` tool.
```

## Verify Claude Agent Content

```scrut
$ cat /tmp/henia-test-build/claude/agents/test-agent/AGENT.md
---
description: Test agent for validation
model: haiku
name: test-agent
---

# Test Agent

This agent validates test results.
```

## Build To OpenCode (Flat Structure)

```scrut
$ rm -rf /tmp/henia-test-build/opencode && henia build "$TESTDIR/fixtures/simple-source" --config "$TESTDIR/fixtures/henia-opencode.toml" --output /tmp/henia-test-build 2>&1
Built 3 artifact(s) in /tmp/henia-test-build
```

## Verify OpenCode Directory Structure (Flat)

```scrut
$ ls -R /tmp/henia-test-build/opencode
agents
commands
skills

/tmp/henia-test-build/opencode/agents:
test-agent.md

/tmp/henia-test-build/opencode/commands:
test-command.md

/tmp/henia-test-build/opencode/skills:
test-skill
test-skill.md

/tmp/henia-test-build/opencode/skills/test-skill:
reference

/tmp/henia-test-build/opencode/skills/test-skill/reference:
guide.md
```

## Verify OpenCode Key Mapping (allowed_tools -> tools)

```scrut
$ cat /tmp/henia-test-build/opencode/skills/test-skill.md
---
description: Test skill for integration tests
model: anthropic/claude-sonnet-4-5
name: test-skill
tools:
  - bash
  - read
user-invocable: true
---

# Test Skill

This is a test skill for henia integration testing.

Use `@code-review` after implementation.
Run `bash` to execute tests.

## Workflow

1. Write code
2. Review with skill reference
3. Test with tool reference
```

## Verify OpenCode Command

```scrut
$ cat /tmp/henia-test-build/opencode/commands/test-command.md
---
description: Test command using anthropic/claude-haiku-4-5
name: test-command
---

# Test Command

Execute test command using `bash` tool.
```

## Verify OpenCode Agent

```scrut
$ cat /tmp/henia-test-build/opencode/agents/test-agent.md
---
description: Test agent for validation
model: anthropic/claude-haiku-4-5
name: test-agent
---

# Test Agent

This agent validates test results.
```

## Rebuild Succeeds

```scrut
$ henia build "$TESTDIR/fixtures/simple-source" --config "$TESTDIR/fixtures/henia-opencode.toml" --output /tmp/henia-test-build 2>&1
Built 3 artifact(s) in /tmp/henia-test-build
```

## Cleanup

```scrut
$ rm -rf /tmp/henia-test-build
```
