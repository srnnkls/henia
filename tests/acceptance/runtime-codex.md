# Skill runtime in Codex

Codex lists only the projected entry skill; the library skill it references is
read through `henia show`. A library skill's preload runs inside `henia show`.
Henia's sandbox cannot nest
inside Codex's, so Codex rules let `henia show` and `henia preload` run outside
it; Henia then sandboxes each preload itself.
Skipped without `codex` or Codex credentials.

```scrut
$ command -v codex > /dev/null && [ -f "${CODEX_AUTH:-$HOME/.codex/auth.json}" ] || exit 80
> W=$("$TESTDIR/setup-runtime") && ln -s "${CODEX_AUTH:-$HOME/.codex/auth.json}" "$W/home/codex/auth.json"
> codex_run() { (cd "$W/project" && env XDG_DATA_HOME="$W/data" XDG_CACHE_HOME="$W/cache" MISE_DATA_DIR="$HOME/.local/share/mise" ZDOTDIR="$W/zdotdir" CODEX_HOME="$W/home/codex" codex exec --skip-git-repo-check --sandbox read-only -o "$W/$1.txt" "$2" < /dev/null > "$W/$1.log" 2>&1); }
> ls "$W/home/codex/skills"
entry
henia
```

```scrut
$ codex_run entry 'Use the $entry skill.'; grep -o 'CANARY-RUNTIME' "$W/entry.txt" | head -n 1; grep -q 'henia show canary-guide' "$W/entry.log" && echo 'read through henia show'
CANARY-RUNTIME
read through henia show
```

```scrut
$ codex_run preload 'Run exactly: henia show preload-canary, then reply with the token it shows, and nothing else.'
> grep -o 'CANARY-PRELOAD' "$W/preload.txt" | head -n 1
CANARY-PRELOAD
```

```scrut
$ rm -rf "$W"
```
