# Henia Artifact Filtering Tests

## Setup Test Environment

```scrut
$ FIXTURE="$TESTDIR/fixtures/simple-source" && DATADIR=/tmp/henia-filter-data/test-source && rm -rf /tmp/henia-filter-test /tmp/henia-filter-data && mkdir -p "$DATADIR" && cp -r "$FIXTURE"/* "$DATADIR/" && cd "$DATADIR" && git init >/dev/null 2>&1 && git config user.email "test@test.com" && git config user.name "Test" && git add . && git commit -m "Initial commit" >/dev/null 2>&1
```

## Create Filter Config Files

```scrut
$ printf '%s\n' 'artifacts = ["skills", "commands", "agents"]' '[sources.test-source]' 'repo = "local"' 'ref = "main"' '[harness.claude]' 'path = "/tmp/henia-filter-test"' 'structure = "nested"' 'artifacts = ["skills"]' '[harness.claude.variables]' 'model_strong = "opus"' > /tmp/henia-filter-type.toml
```

## Filter By Artifact Type

Only deploy skills, exclude commands and agents.

```scrut
$ henia deploy --config /tmp/henia-filter-type.toml --data-dir /tmp/henia-filter-data 2>&1
Deployed 1 artifact(s)
```

## Verify Only Skills Deployed

```scrut
$ ls /tmp/henia-filter-test
skills
```

```scrut
$ ls /tmp/henia-filter-test/skills/
test-skill
```

## Create Include List Config

```scrut
$ rm -rf /tmp/henia-filter-test && printf '%s\n' 'artifacts = ["skills", "commands", "agents"]' '[sources.test-source]' 'repo = "local"' 'ref = "main"' '[harness.claude]' 'path = "/tmp/henia-filter-test"' 'structure = "nested"' 'include = ["test-skill"]' '[harness.claude.variables]' 'model_strong = "opus"' > /tmp/henia-filter-include.toml
```

## Filter By Include List

```scrut
$ henia deploy --config /tmp/henia-filter-include.toml --data-dir /tmp/henia-filter-data 2>&1
Deployed 1 artifact(s)
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
$ rm -rf /tmp/henia-filter-test && printf '%s\n' 'artifacts = ["skills", "commands", "agents"]' '[sources.test-source]' 'repo = "local"' 'ref = "main"' '[harness.claude]' 'path = "/tmp/henia-filter-test"' 'structure = "nested"' 'exclude = ["test-command"]' '[harness.claude.variables]' 'model_strong = "opus"' > /tmp/henia-filter-exclude.toml
```

## Filter By Exclude List

```scrut
$ henia deploy --config /tmp/henia-filter-exclude.toml --data-dir /tmp/henia-filter-data 2>&1
Deployed 2 artifact(s)
```

## Verify Excluded Artifact Not Deployed

```scrut
$ ls /tmp/henia-filter-test/skills/
test-skill
```

```scrut
$ ls /tmp/henia-filter-test/agents/
test-agent
```

```scrut
$ test -d /tmp/henia-filter-test/commands && echo "Commands directory exists" || echo "No commands directory"
No commands directory
```

## Cleanup

```scrut
$ rm -rf /tmp/henia-filter-test /tmp/henia-filter-data /tmp/henia-filter-*.toml
```
