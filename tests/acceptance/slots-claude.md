# Slot composition in Claude Code

Claude Code loads Tropos from an isolated `CLAUDE_CONFIG_DIR`, preloads the
`code` skill's slot context through `henia slots` over an isolated Henia
library, and reads the providers that survive with `henia show`. The project's
`house-go` shadows the global package's `acme-go`; `house-review` provides at
`fallback` priority beside the global `acme-review`. An isolated
configuration reads no keychain login, so this needs `CLAUDE_CODE_OAUTH_TOKEN`
(from `claude setup-token`) or `ANTHROPIC_API_KEY`; skipped without them.

```scrut
$ command -v claude phora > /dev/null && [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}${ANTHROPIC_API_KEY:-}" ] || exit 80
> W=$("$TESTDIR/setup-slots") && echo staged
staged
```

```scrut
$ cd "$W/project" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u CLAUDE_CODE_SESSION_ID ZDOTDIR="$W/zdotdir" XDG_DATA_HOME="$W/data" XDG_CACHE_HOME="$W/cache" XDG_CONFIG_HOME="$W/config" CLAUDE_CONFIG_DIR="$W/home/claude" claude -p 'Use the code skill, then run every henia show command listed in its slot providers context. Reply with each canary token you found, one per line, and nothing else.' --model "${CLAUDE_MODEL:-sonnet}" --permission-mode default --allowedTools 'Skill(code)' 'Bash(henia show *)' > "$W/claude.txt" 2>&1; grep -o 'CANARY-[A-Z][A-Z-]*' "$W/claude.txt" | sort -u
CANARY-ACME-REVIEW
CANARY-HOUSE-GO
CANARY-HOUSE-REVIEW
```

```scrut
$ rm -rf "$W"
```
