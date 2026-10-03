# Henia Runtime Tests

## Fixture

A global source with an entry skill and two library skills, a project source,
and projections for two harnesses.

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P); L="$T/data/henia/sources"; P="$T/repo"
> henia() { env HENIA_HARNESS=none XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" henia "$@"; }
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> mk "$L/tropos/skills/code/SKILL.md" '---\nname: code\ndescription: Code workflows.\nmetadata:\n  slots: "code.style code.check:keyed(list(command))"\n  applies: "code.style code.check"\n---\n\n# Code\n\nFollow `$style-guide` and `$checks`.\n'
> mk "$L/tropos/skills/style-guide/SKILL.md" '---\nname: style-guide\ndescription: House style by language.\nmetadata:\n  provides: "code.style.go"\n---\n\n# Style guide\n\nShared rules.\n\n## Go\n\nRun gofmt. Canary: CANARY-GO.\n\n### Errors\n\nWrap with %%w.\n\n## Python\n\nUse ruff.\n'
> mk "$L/tropos/skills/checks/SKILL.md" '---\nname: checks\ndescription: Validation commands.\nmetadata:\n  provides: "code.check.go"\n  code.check.go: "go vet ./..."\n---\n\n# Checks\n\nSee `$style-guide` and `$code`.{{with .flavor}} Flavor: {{.}}.{{end}}\n'
> printf '[harness.claude]\nartifacts = ["skills"]\ninclude = ["code"]\n\n[harness.claude.references.skill]\noutput = "/{{.Name}}"\n\n[harness.pi]\nartifacts = ["skills"]\n\n[harness.pi.variables]\nflavor = "pi"\n\n[harness.pi.references.skill]\noutput = "/skill:{{.Name}}"\n' > "$L/tropos/henia.toml"
> henia build "$L/tropos" --output "$T/build" > /dev/null
> mkdir -p "$P" && git -C "$P" init -q && cd "$P"
> find "$T/build" -name SKILL.md | sed "s|$T/build/||" | sort
claude/skills/code/SKILL.md
pi/skills/checks/SKILL.md
pi/skills/code/SKILL.md
pi/skills/style-guide/SKILL.md
```

## Projection

A harness's `include` selects its entry skills; references to skills it does not
project render as `henia show` commands.

```scrut
$ tail -n 1 "$T/build/claude/skills/code/SKILL.md"; tail -n 1 "$T/build/pi/skills/code/SKILL.md"
Follow `henia show style-guide` and `henia show checks`.
Follow `/skill:style-guide` and `/skill:checks`.
```

## Catalog

```scrut
$ henia ls
checks	global	Validation commands.
code	global	Code workflows.
style-guide	global	House style by language.
```

```scrut
$ henia ls --json | jq -c '.[] | [.id, .source, .tier, (.digest | length), [.sections[].anchor]]'
["tropos:checks","tropos","global",64,["checks"]]
["tropos:code","tropos","global",64,["code"]]
["tropos:style-guide","tropos","global",64,["style-guide","go","errors","python"]]
```

## Showing

```scrut
$ henia show style-guide#go
## Go

Run gofmt. Canary: CANARY-GO.

### Errors

Wrap with %w.

```

```scrut
$ henia show style-guide#nope
henia: style-guide has no section "nope"
style-guide#style-guide  Style guide
  style-guide#go  Go
    style-guide#errors  Errors
  style-guide#python  Python
```

```scrut
$ henia show nothing; echo "exit $?"
henia: no skill named "nothing" in the library; henia ls lists them
exit 0
```

## Rendering

`henia show` renders through the owning source's configuration for the caller:
`--harness`, then `HENIA_HARNESS` (`none` is neutral), then the nearest agent
process, then `AI_AGENT` or a vendor marker. References
to skills the caller's harness projects keep its syntax; the rest render as
`henia show`. Without a caller, every reference is a `henia show`.

```scrut
$ henia show checks | tail -n 1
See `henia show style-guide` and `henia show code`.
```

```scrut
$ henia show checks --harness claude | tail -n 1
See `henia show style-guide` and `/code`.
```

```scrut
$ henia show checks --harness pi | tail -n 1
See `/skill:style-guide` and `/skill:code`. Flavor: pi.
```

```scrut
$ env HENIA_HARNESS=pi XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" henia show checks | tail -n 1
See `/skill:style-guide` and `/skill:code`. Flavor: pi.
```

Renders are cached by content; changing a skill renders it again.

```scrut
$ find "$T/cache/henia/render" -type f | wc -l | tr -d ' '
> printf 'Updated.\n' >> "$L/tropos/skills/checks/SKILL.md"
> henia show checks | tail -n 1
4
Updated.
```

A source renders with its own harness profiles from `.henia/harnesses`.

```scrut
$ mk "$L/tropos/.henia/harnesses/mini/transform.toml" 'fields = ["name", "description"]\n'
> printf '\n[harness.mini]\nprofile = "mini"\nartifacts = ["skills"]\n' >> "$L/tropos/henia.toml"
> henia show checks --harness mini | head -n 1
# Checks
```

## Resolution

The project source shadows global sources by name; `source:name` names any
skill exactly.

```scrut
$ mk "$P/.henia/skills/style-guide/SKILL.md" '---\nname: style-guide\ndescription: Repository style.\n---\n\n# Repo style\n'
> henia ls; henia show style-guide; henia show tropos:style-guide | head -n 1
style-guide	project	Repository style.
checks	global	Validation commands.
code	global	Code workflows.
tropos:style-guide	global	House style by language.
# Repo style
# Style guide
```

Two global sources defining a name make it ambiguous; it never resolves silently.

```scrut
$ mk "$L/extra/skills/checks/SKILL.md" '---\nname: checks\ndescription: Other checks.\n---\n\n# Other\n'
> henia show checks; henia show extra:checks; henia ls | grep checks
henia: "checks" is ambiguous; name one of extra:checks, tropos:checks
# Other
extra:checks	global	Other checks.
tropos:checks	global	Validation commands.
```

## Context

A skill's context lists the providers of the slots it applies, library providers
as the `henia show` command that reads them, and the sections of the library
skills it references; it never repeats the skill's own text.

```scrut
$ rm -rf "$L/extra" "$P/.henia"
> henia context code --global "$T/build/claude/skills" | sed "s|$T/||g"
Slot providers:
code.check.go	global	henia show tropos:checks	go vet ./...
code.style.go	global	henia show tropos:style-guide

Skill style-guide: House style by language.
style-guide#style-guide  Style guide
  style-guide#go  Go
    style-guide#errors  Errors
  style-guide#python  Python

Skill checks: Validation commands.
checks#checks  Checks
```

```scrut
$ henia context nothing; echo "exit $?"
henia: no skill named "nothing"
exit 0
```

```scrut
$ rm -rf "$T"
```
