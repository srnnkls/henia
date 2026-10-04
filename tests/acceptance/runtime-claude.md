# Skill runtime in Claude Code

Claude Code lists only the projected entry skill; the library skill it
references is read through `henia show`, and a FAS hook blocks reading the
library directly, steering the agent back to `henia show`. A library skill's
preload runs inside `henia show`, so its output reaches the agent, while a
preload the project's FAS rules deny shows the rule instead of running. Needs `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`;
skipped without them, `claude` or `fas`.

```scrut
$ command -v claude fas > /dev/null && [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}${ANTHROPIC_API_KEY:-}" ] || exit 80
> W=$("$TESTDIR/setup-runtime")
> claude_run() { name=$1; shift; (cd "$W/project" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u CLAUDE_CODE_SESSION_ID XDG_DATA_HOME="$W/data" XDG_CACHE_HOME="$W/cache" MISE_DATA_DIR="$HOME/.local/share/mise" ZDOTDIR="$W/zdotdir" CLAUDE_CONFIG_DIR="$W/home/claude" claude -p "$@" --model "${CLAUDE_MODEL:-sonnet}" --output-format stream-json --verbose < /dev/null > "$W/$name.jsonl" 2>&1); }
> ls "$W/home/claude/skills"
entry
henia
```

```scrut
$ claude_run entry 'Use the entry skill.' --allowedTools 'Bash(henia show *)'; jq -r 'select(.type == "result") | .result' "$W/entry.jsonl" | grep -o 'CANARY-RUNTIME' | head -n 1; grep -q 'henia show canary-guide' "$W/entry.jsonl" && echo 'read through henia show'
CANARY-RUNTIME
read through henia show
```

```scrut
$ claude_run deny "Use the Read tool on $W/data/henia/sources/fixture/skills/canary-guide/SKILL.md and reply with the canary token it contains, or BLOCKED if you cannot read it." --allowedTools Read
> grep -q 'Read hook error: Library skills are served by henia' "$W/deny.jsonl" && echo 'direct read blocked'
> jq -r 'select(.type == "result") | .result' "$W/deny.jsonl" | grep -q 'CANARY-RUNTIME' && ! grep -q 'henia show canary-guide' "$W/deny.jsonl" && echo 'canary leaked' || echo 'canary only through henia show'
direct read blocked
canary only through henia show
```

```scrut
$ claude_run preload 'Run `henia show preload-canary` and reply with the token it shows, and nothing else.' --allowedTools 'Bash(henia show *)'
> jq -r 'select(.type == "result") | .result' "$W/preload.jsonl" | grep -o 'CANARY-PRELOAD' | head -n 1
> grep -q 'blocked by fas/no-clock-preload: Clock preloads are off in this fixture' "$W/preload.jsonl" && echo 'denied preload shows the FAS rule'
CANARY-PRELOAD
denied preload shows the FAS rule
```

```scrut
$ rm -rf "$W"
```
