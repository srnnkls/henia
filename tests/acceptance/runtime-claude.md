# Skill runtime in Claude Code

Claude Code lists only the projected entry skill; the library skill it
references is read through `henia show`. A library skill's preload runs inside
`henia show`, so its output reaches the agent. Needs `CLAUDE_CODE_OAUTH_TOKEN`
or `ANTHROPIC_API_KEY`; skipped without them or `claude`.

```scrut
$ command -v claude > /dev/null && [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}${ANTHROPIC_API_KEY:-}" ] || exit 80
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
$ claude_run preload 'Run `henia show preload-canary` and reply with the token it shows, and nothing else.' --allowedTools 'Bash(henia show *)'
> jq -r 'select(.type == "result") | .result' "$W/preload.jsonl" | grep -o 'CANARY-PRELOAD' | head -n 1
CANARY-PRELOAD
```

```scrut
$ rm -rf "$W"
```
