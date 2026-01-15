# Henia Sync Tests

## Help

```scrut
$ henia --help
Henia syncs skills, commands, and agents from phora sources to harness targets.

Usage:
  henia [command]

Available Commands:
  add         Add source to config and sync
  completion  Generate the autocompletion script for the specified shell
  deploy      Deploy to harnesses (assumes sources already fetched)
  help        Help about any command
  sync        Fetch sources and deploy to harnesses
  update      Fetch sources and deploy (alias for sync)

Flags:
      --config string     Config file path
      --data-dir string   Data directory for sources
  -h, --help              help for henia

Use "henia [command] --help" for more information about a command.
```

## Sync Help

```scrut
$ henia sync --help
Fetch all configured sources via phora and deploy artifacts to harnesses.

Usage:
  henia sync [flags]

Flags:
  -h, --help   help for sync

Global Flags:
      --config string     Config file path
      --data-dir string   Data directory for sources
```

## Deploy Help

```scrut
$ henia deploy --help
Deploy artifacts from already-fetched sources to configured harnesses.

Usage:
  henia deploy [flags]

Flags:
  -h, --help   help for deploy

Global Flags:
      --config string     Config file path
      --data-dir string   Data directory for sources
```

## Deploy To Claude (Nested Structure)

Set up test environment with git repo and deploy.

```scrut
$ FIXTURE="$TESTDIR/fixtures/simple-source" && DATADIR=/tmp/henia-test-data/test-source && rm -rf /tmp/henia-test-claude /tmp/henia-test-data && mkdir -p "$DATADIR" && cp -r "$FIXTURE"/* "$DATADIR/" && cd "$DATADIR" && git init >/dev/null 2>&1 && git config user.email "test@test.com" && git config user.name "Test" && git add . && git commit -m "Initial commit" >/dev/null 2>&1 && cd "$TESTDIR/fixtures" && henia deploy --config henia-claude.toml --data-dir /tmp/henia-test-data 2>&1
Deployed 3 artifact(s)
```

## Verify Claude Directory Structure

```scrut
$ ls -R /tmp/henia-test-claude
agents
commands
skills

/tmp/henia-test-claude/agents:
test-agent

/tmp/henia-test-claude/agents/test-agent:
AGENT.md

/tmp/henia-test-claude/commands:
test-command

/tmp/henia-test-claude/commands/test-command:
COMMAND.md

/tmp/henia-test-claude/skills:
test-skill

/tmp/henia-test-claude/skills/test-skill:
SKILL.md
reference

/tmp/henia-test-claude/skills/test-skill/reference:
guide.md
```

## Verify Claude Skill Content

```scrut
$ cat /tmp/henia-test-claude/skills/test-skill/SKILL.md
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
$ cat /tmp/henia-test-claude/skills/test-skill/reference/guide.md
# Test Skill Guide

This is a reference guide for the test skill.

## Best Practices

- Always test your code
- Use proper naming conventions
- Write clear documentation
```

## Verify Claude Command Content

```scrut
$ cat /tmp/henia-test-claude/commands/test-command/COMMAND.md
---
description: Test command using haiku
name: test-command
---

# Test Command

Execute test command using `Bash` tool.
```

## Verify Claude Agent Content

```scrut
$ cat /tmp/henia-test-claude/agents/test-agent/AGENT.md
---
description: Test agent for validation
model: haiku
name: test-agent
---

# Test Agent

This agent validates test results.
```

## Deploy To OpenCode (Flat Structure)

```scrut
$ rm -rf /tmp/henia-test-opencode && cd "$TESTDIR/fixtures" && henia deploy --config henia-opencode.toml --data-dir /tmp/henia-test-data 2>&1
Deployed 3 artifact(s)
```

## Verify OpenCode Directory Structure (Flat)

```scrut
$ ls -R /tmp/henia-test-opencode
agents
commands
skills

/tmp/henia-test-opencode/agents:
test-agent.md

/tmp/henia-test-opencode/commands:
test-command.md

/tmp/henia-test-opencode/skills:
test-skill
test-skill.md

/tmp/henia-test-opencode/skills/test-skill:
reference

/tmp/henia-test-opencode/skills/test-skill/reference:
guide.md
```

## Verify OpenCode Key Mapping (allowed_tools -> tools)

```scrut
$ cat /tmp/henia-test-opencode/skills/test-skill.md
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
$ cat /tmp/henia-test-opencode/commands/test-command.md
---
description: Test command using anthropic/claude-haiku-4-5
name: test-command
---

# Test Command

Execute test command using `bash` tool.
```

## Verify OpenCode Agent

```scrut
$ cat /tmp/henia-test-opencode/agents/test-agent.md
---
description: Test agent for validation
model: anthropic/claude-haiku-4-5
name: test-agent
---

# Test Agent

This agent validates test results.
```

## Re-deploy Succeeds

```scrut
$ cd "$TESTDIR/fixtures" && henia deploy --config henia-opencode.toml --data-dir /tmp/henia-test-data 2>&1
Deployed 3 artifact(s)
```

## Cleanup

```scrut
$ rm -rf /tmp/henia-test-claude /tmp/henia-test-opencode /tmp/henia-test-data
```
