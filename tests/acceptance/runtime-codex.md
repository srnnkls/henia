# Skill runtime in Codex

Codex lists only the projected entry skill; the library skill it references is
read through `henia show`, and a FAS hook blocks reading the library directly,
steering the agent back to `henia show`. A library skill's preload runs inside
`henia show` and a FAS-denied one shows the rule. Codex's read-only sandbox
already denies writes and Henia's own sandbox cannot nest inside it, so the
isolated user configuration opts into `unsandboxed = "run"`.
Skipped without `codex`, `fas` or Codex credentials.

```scrut
$ command -v codex fas > /dev/null && [ -f "${CODEX_AUTH:-$HOME/.codex/auth.json}" ] || exit 80
> W=$("$TESTDIR/setup-runtime") && ln -s "${CODEX_AUTH:-$HOME/.codex/auth.json}" "$W/home/codex/auth.json"
> codex_run() { (cd "$W/project" && env XDG_DATA_HOME="$W/data" XDG_CACHE_HOME="$W/cache" XDG_CONFIG_HOME="$W/codex-config" MISE_DATA_DIR="$HOME/.local/share/mise" MISE_GLOBAL_CONFIG_FILE="$HOME/.config/mise/config.toml" ZDOTDIR="$W/zdotdir" CODEX_HOME="$W/home/codex" codex exec --skip-git-repo-check --sandbox read-only --dangerously-bypass-hook-trust -o "$W/$1.txt" "$2" < /dev/null > "$W/$1.log" 2>&1); }
> ls "$W/home/codex/skills"
entry
```

```scrut
$ codex_run entry 'Use the $entry skill.'; grep -o 'CANARY-RUNTIME' "$W/entry.txt" | head -n 1; grep -q 'henia show canary-guide' "$W/entry.log" && echo 'read through henia show'
CANARY-RUNTIME
read through henia show
```

```scrut
$ codex_run deny "Run exactly: cat $W/data/henia/sources/fixture/skills/canary-guide/SKILL.md and reply with the canary token it contains, or BLOCKED if the command is not allowed."
> grep -q 'blocked by PreToolUse hook: Library skills are served by henia' "$W/deny.log" && echo 'direct read blocked'
> grep -q 'CANARY-RUNTIME' "$W/deny.txt" && ! grep -q 'henia show canary-guide' "$W/deny.log" && echo 'canary leaked' || echo 'canary only through henia show'
direct read blocked
canary only through henia show
```

```scrut
$ codex_run preload 'Run exactly: henia show preload-canary, then reply with the token it shows, and nothing else.'
> grep -o 'CANARY-PRELOAD' "$W/preload.txt" | head -n 1
> grep -q 'blocked by fas/no-clock-preload: Clock preloads are off in this fixture' "$W/preload.log" && echo 'denied preload shows the FAS rule'
CANARY-PRELOAD
denied preload shows the FAS rule
```

```scrut
$ rm -rf "$W"
```
