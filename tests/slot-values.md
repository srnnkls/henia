# Henia Slot Value Tests

## Fixture

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P); G="$T/data/henia/packages/global/acme/skills"; P="$T/repo"
> henia() { env HENIA_HARNESS=none XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" XDG_CONFIG_HOME="$T/config" henia "$@"; }
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> slots() { (cd "$P" && henia slots "$@"; echo "exit $?") 2>&1 | sed "s|$T/data/henia/packages/global/||g" | tr '\t' ' '; }
> mk "$G/owner/SKILL.md" '---\nname: owner\nhenia:\n  slots:\n    code.check: keyed(list(command))\n    docs.template: unique(path)\n    git.style: unique(text)\n    code.style:\n---\n'
> mk "$G/gocheck/SKILL.md" '---\nname: gocheck\nhenia:\n  provides:\n    code.check.go: {command: go vet ./...}\n---\n'
> mk "$G/pycheck/SKILL.md" '---\nname: pycheck\nhenia:\n  provides:\n    code.check.python:\n---\n'
> mk "$G/tpl/SKILL.md" '---\nname: tpl\nhenia:\n  provides:\n    docs.template: {path: template.md}\n---\n'
> mk "$G/tpl/template.md" '# Template\n'
> mk "$G/gitstyle/SKILL.md" '---\nname: gitstyle\nhenia:\n  provides:\n    git.style: {text: conventional}\n---\n'
> mk "$G/styled/SKILL.md" '---\nname: styled\nhenia:\n  provides:\n    code.style.go: {text: gofmt}\n---\n'
> mkdir -p "$P" && git -C "$P" init -q && echo staged
staged
```

## Values

A value-typed slot prints each provider's value as a fourth column; a path
resolves against the providing skill.

```scrut
$ slots code.check git.style docs.template
git.style global henia show acme:gitstyle conventional
code.check.go global henia show acme:gocheck go vet ./...
docs.template global henia show acme:tpl acme/skills/tpl/template.md
code.check.python invalid acme/skills/pycheck/SKILL.md
exit 0
```

`--explain` shows each value and why a provider is invalid.

```scrut
$ slots --explain code.check code.style
code.check
  code.check (keyed(list(command)), declared by acme/skills/owner/SKILL.md)
    no providers
  code.check.go (keyed(list(command)) of code.check, declared by acme/skills/owner/SKILL.md)
    selected gocheck [global, fallback from tier] acme/skills/gocheck/SKILL.md
      = go vet ./...
  code.check.python (keyed(list(command)) of code.check, declared by acme/skills/owner/SKILL.md)
    invalid  pycheck [global, fallback from tier] acme/skills/pycheck/SKILL.md
      code.check needs a command
code.style
  code.style (keyed(list), declared by acme/skills/owner/SKILL.md)
    no providers
  code.style.go (keyed(list) of code.style, declared by acme/skills/owner/SKILL.md)
    invalid  styled [global, fallback from tier] acme/skills/styled/SKILL.md
      code.style takes a skill, not a text
exit 0
```

A path that does not exist is invalid.

```scrut
$ mk "$G/tpl/SKILL.md" '---\nname: tpl\nhenia:\n  provides:\n    docs.template: {path: missing.md}\n---\n'
> slots docs.template | grep docs.template
docs.template none -
docs.template invalid acme/skills/tpl/SKILL.md
```

```scrut
$ rm -rf "$T"
```
