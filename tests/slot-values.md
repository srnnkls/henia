# Henia Slot Value Tests

## Fixture

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P); G="$T/global"; P="$T/repo"
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> slots() { (cd "$P" && henia slots --global "$G" "$@"; echo "exit $?") 2>&1 | sed "s|$T/||g" | tr '\t' ' '; }
> mk "$G/owner/SKILL.md" '---\nname: owner\nmetadata:\n  slots: "code.check:keyed(list(command)) docs.template:unique(path) git.style:unique(text) code.style"\n---\n'
> mk "$G/gocheck/SKILL.md" '---\nname: gocheck\nmetadata:\n  provides: code.check.go\n  code.check.go: "go vet ./..."\n---\n'
> mk "$G/pycheck/SKILL.md" '---\nname: pycheck\nmetadata:\n  provides: code.check.python\n---\n'
> mk "$G/tpl/SKILL.md" '---\nname: tpl\nmetadata:\n  provides: docs.template\n  docs.template: template.md\n---\n'
> mk "$G/tpl/template.md" '# Template\n'
> mk "$G/gitstyle/SKILL.md" '---\nname: gitstyle\nmetadata:\n  provides: git.style\n  git.style: conventional\n---\n'
> mk "$G/styled/SKILL.md" '---\nname: styled\nmetadata:\n  provides: code.style.go\n  code.style.go: gofmt\n---\n'
> mkdir -p "$P" && git -C "$P" init -q && echo staged
staged
```

## Values

A value-typed slot prints each provider's value as a fourth column; a path
resolves against the providing skill.

```scrut
$ slots code.check git.style docs.template
git.style global global/gitstyle/SKILL.md conventional
code.check.go global global/gocheck/SKILL.md go vet ./...
docs.template global global/tpl/SKILL.md global/tpl/template.md
code.check.python invalid global/pycheck/SKILL.md
code.style.go invalid global/styled/SKILL.md
exit 0
```

`--explain` shows each value and why a provider is invalid.

```scrut
$ slots --explain code.check code.style
code.check
  code.check (keyed(list(command)), declared by global/owner/SKILL.md)
    no providers
  code.check.go (keyed(list(command)) of code.check, declared by global/owner/SKILL.md)
    selected gocheck [global, fallback from tier] global/gocheck/SKILL.md
      = go vet ./...
  code.check.python (keyed(list(command)) of code.check, declared by global/owner/SKILL.md)
    invalid  pycheck [global, fallback from tier] global/pycheck/SKILL.md
      code.check needs a command
code.style
  code.style (keyed(list), declared by global/owner/SKILL.md)
    no providers
  code.style.go (keyed(list) of code.style, declared by global/owner/SKILL.md)
    invalid  styled [global, fallback from tier] global/styled/SKILL.md
      code.style takes a skill, not a value
exit 0
```

A path that does not exist is invalid.

```scrut
$ mk "$G/tpl/SKILL.md" '---\nname: tpl\nmetadata:\n  provides: docs.template\n  docs.template: missing.md\n---\n'
> slots docs.template | grep docs.template
docs.template invalid global/tpl/SKILL.md
```


```scrut
$ rm -rf "$T"
```
