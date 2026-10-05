# Slot composition in Codex

Codex loads Tropos from an isolated `CODEX_HOME`, runs the `code` skill's slot
context through `henia slots`, and reads the providers that survive. The
project's `house-go` shadows the global `acme-go`; `house-review` provides at
`fallback` priority beside the global `acme-review`. Skipped without `codex`,
`phora` or Codex credentials.

```scrut
$ command -v codex phora > /dev/null && [ -f "${CODEX_AUTH:-$HOME/.codex/auth.json}" ] || exit 80
> W=$("$TESTDIR/setup-slots") && ln -s "${CODEX_AUTH:-$HOME/.codex/auth.json}" "$W/home/codex/auth.json" && echo staged
staged
```

```scrut
$ cd "$W/project" && XDG_DATA_HOME="$W/data" XDG_CACHE_HOME="$W/cache" XDG_CONFIG_HOME="$W/config" henia slots code.style.go review.criteria | cut -f1,2
code.style.go	project
review.criteria	project
review.criteria	global
```

```scrut
$ cd "$W/project" && ZDOTDIR="$W/zdotdir" XDG_DATA_HOME="$W/data" XDG_CACHE_HOME="$W/cache" XDG_CONFIG_HOME="$W/config" CODEX_HOME="$W/home/codex" codex exec --skip-git-repo-check --sandbox read-only -o "$W/codex.txt" 'Use the $code skill: run its context commands, then run every henia show command listed as a slot provider. Reply with each canary token you found, one per line, and nothing else.' > "$W/codex.log" 2>&1; grep -o 'CANARY-[A-Z][A-Z-]*' "$W/codex.txt" | sort -u
CANARY-ACME-REVIEW
CANARY-HOUSE-GO
CANARY-HOUSE-REVIEW
```

```scrut
$ rm -rf "$W"
```
