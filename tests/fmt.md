# Henia Fmt Tests

## Fixture

`mk` writes a file with `printf` escapes.

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P) && cd "$T"
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> mk nested.md ':::a\nouter\n\n:::b\ninner\n:::\n\n:::\n'
> mk canonical.md '# Title\n\n::::a\ntext  with  spacing\n\n:::b\ninner\n:::\n\n::::\n'
> cp canonical.md canonical.orig
> echo staged
staged
```

## Check

`--check` names each file whose fences would change, leaves it as written and
exits non-zero.

```scrut
$ henia fmt --check nested.md canonical.md
nested.md
[1]
```

```scrut
$ cat nested.md
:::a
outer

:::b
inner
:::

:::
```

A file whose fences are already canonical passes the check.

```scrut
$ henia fmt --check canonical.md
```

## Rewrite

Each outer fence becomes one colon longer than the longest fence it contains.

```scrut
$ henia fmt nested.md && cat nested.md
::::a
outer

:::b
inner
:::

::::
```

A second run changes nothing.

```scrut
$ henia fmt nested.md && henia fmt --check nested.md && cat nested.md
::::a
outer

:::b
inner
:::

::::
```

A canonical file stays byte-identical.

```scrut
$ henia fmt canonical.md && cmp canonical.md canonical.orig && echo same
same
```

## Code span link labels

A link label that starts with, ends with, or consists of a code span, plain or
inside emphasis, renders and formats as written.

```scrut
$ L="$T/data/henia/packages/global/pkg"
> henia() { HENIA_HARNESS=none XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" XDG_STATE_HOME="$T/state" command henia "$@"; }
> mk "$L/skills/links/SKILL.md" '---\nname: links\ndescription: Code span labels.\n---\n\n# Links\n\nSee [`peer path`](../peer/SKILL.md#x), [a `b`](y.md), [`c` d](z.md), [``e`f``](v.md) and [*`g` h*](w.md).\n'
> printf '[harness.claude]\nartifacts = ["skills"]\n' > "$L/henia.toml"
> cp "$L/skills/links/SKILL.md" links.orig
> henia build "$L" --output "$T/build" > /dev/null
> grep -h '^See' "$T/build/claude/skills/links/SKILL.md"
> henia show links | grep '^See'
> henia fmt --check "$L/skills/links/SKILL.md" && henia fmt "$L/skills/links/SKILL.md" && cmp "$L/skills/links/SKILL.md" links.orig && echo same
See [`peer path`](../peer/SKILL.md#x), [a `b`](y.md), [`c` d](z.md), [``e`f``](v.md) and [*`g` h*](w.md).
See `peer path` (`henia show 'peer#x'`), a `b` (`henia show links.y`), `c` d (`henia show links.z`), ``e`f`` (`henia show links.v`) and *`g` h* (`henia show links.w`).
same
```
