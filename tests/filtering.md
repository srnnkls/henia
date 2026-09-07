# Henia Artifact Filtering Tests

## Setup Test Environment

```scrut
$ rm -rf /tmp/henia-filter-test
```

## Create Filter Config Files

```scrut
$ printf '%s\n' 'artifacts = ["skills", "commands", "agents"]' '[harness.claude]' 'structure = "nested"' 'artifacts = ["skills"]' '[harness.claude.variables]' 'model_strong = "opus"' > /tmp/henia-filter-type.toml
```

## Filter By Artifact Type

Only build skills, exclude commands and agents.

```scrut
$ henia build "$TESTDIR/fixtures/simple-source" --config /tmp/henia-filter-type.toml --output /tmp/henia-filter-test 2>&1
Built 1 artifact(s) in /tmp/henia-filter-test
```

## Verify Only Skills Built

```scrut
$ ls /tmp/henia-filter-test/claude
skills
```

```scrut
$ ls /tmp/henia-filter-test/claude/skills/
test-skill
```

## Create Include List Config

```scrut
$ rm -rf /tmp/henia-filter-test && printf '%s\n' 'artifacts = ["skills", "commands", "agents"]' '[harness.claude]' 'structure = "nested"' 'include = ["test-skill"]' '[harness.claude.variables]' 'model_strong = "opus"' > /tmp/henia-filter-include.toml
```

## Filter By Include List

```scrut
$ henia build "$TESTDIR/fixtures/simple-source" --config /tmp/henia-filter-include.toml --output /tmp/henia-filter-test 2>&1
Built 1 artifact(s) in /tmp/henia-filter-test
```

## Verify Only Included Artifacts

```scrut
$ find /tmp/henia-filter-test -name "*.md" -type f | grep SKILL.md | wc -l | tr -d ' '
1
```

```scrut
$ find /tmp/henia-filter-test -name "*.md" -type f | grep COMMAND.md | wc -l | tr -d ' '
0
```

## Create Exclude List Config

```scrut
$ rm -rf /tmp/henia-filter-test && printf '%s\n' 'artifacts = ["skills", "commands", "agents"]' '[harness.claude]' 'structure = "nested"' 'exclude = ["test-command"]' '[harness.claude.variables]' 'model_strong = "opus"' > /tmp/henia-filter-exclude.toml
```

## Filter By Exclude List

```scrut
$ henia build "$TESTDIR/fixtures/simple-source" --config /tmp/henia-filter-exclude.toml --output /tmp/henia-filter-test 2>&1
Built 2 artifact(s) in /tmp/henia-filter-test
```

## Verify Excluded Artifact Not Built

```scrut
$ ls /tmp/henia-filter-test/claude/skills/
test-skill
```

```scrut
$ ls /tmp/henia-filter-test/claude/agents/
test-agent
```

```scrut
$ test -d /tmp/henia-filter-test/claude/commands && echo "Commands directory exists" || echo "No commands directory"
No commands directory
```

## Cleanup

```scrut
$ rm -rf /tmp/henia-filter-test /tmp/henia-filter-*.toml
```
