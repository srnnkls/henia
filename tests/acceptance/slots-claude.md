# Slot composition in Claude Code

Claude Code loads Tropos from an isolated `CLAUDE_CONFIG_DIR`, preloads the
`code` skill's slot context through `henia slots`, and reads the providers that
survive. The project's `house-go` shadows the global `acme-go`; `house-review`
provides at `fallback` priority beside the global `acme-review`. An isolated
configuration reads no keychain login, so this needs `CLAUDE_CODE_OAUTH_TOKEN`
(from `claude setup-token`) or `ANTHROPIC_API_KEY`; skipped without them.

```scrut
$ command -v claude phora > /dev/null && [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}${ANTHROPIC_API_KEY:-}" ] || exit 80
> W=$("$TESTDIR/setup-slots") && echo staged
staged
```

```scrut
$ cd "$W/project" && env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT -u CLAUDE_CODE_SESSION_ID ZDOTDIR="$W/zdotdir" CLAUDE_CONFIG_DIR="$W/home/claude" claude -p --model "${CLAUDE_MODEL:-sonnet}" --allowedTools Read --add-dir "$W/home/claude" 'Use the code skill, then read every SKILL.md listed in its slot providers context. Reply with each canary token you found, one per line, and nothing else.' > "$W/claude.txt" 2>&1; grep -o 'CANARY-[A-Z][A-Z-]*' "$W/claude.txt" | sort -u
CANARY-ACME-REVIEW
CANARY-HOUSE-GO
CANARY-HOUSE-REVIEW
```

```scrut
$ trash "$W"
```
