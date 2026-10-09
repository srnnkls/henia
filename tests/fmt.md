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
