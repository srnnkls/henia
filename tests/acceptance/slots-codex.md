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
$ cd "$W/project" && henia slots --global "$W/home/codex/skills" code.style.go review.criteria | cut -f1,2
code.style.go	project
review.criteria	project
review.criteria	global
```

```scrut
$ cd "$W/project" && ZDOTDIR="$W/zdotdir" CODEX_HOME="$W/home/codex" codex exec --skip-git-repo-check --sandbox read-only -o "$W/codex.txt" 'Use the $code skill: run its context commands, then read every SKILL.md listed as a slot provider. Reply with each canary token you found, one per line, and nothing else.' > "$W/codex.log" 2>&1; grep -o 'CANARY-[A-Z][A-Z-]*' "$W/codex.txt" | sort -u
CANARY-ACME-REVIEW
CANARY-HOUSE-GO
CANARY-HOUSE-REVIEW
```

```scrut
$ trash "$W"
```
