# Henia Component Tests

## Fixture

`proj` starts a fresh project with the `claude` and `codex` harnesses; `skill`
writes a skill into it; `comp` copies a component from
`tests/fixtures/components` into the project; `out` builds the project and
prints one skill as built for one harness, or the build error.

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P); F="$TESTDIR/fixtures/components"
> henia() { env HENIA_HARNESS=none XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" XDG_CONFIG_HOME="$T/config" henia "$@"; }
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> proj() { P="$T/$1"; mkdir -p "$P" && git -C "$P" init -q && cd "$P" && printf 'artifacts = ["skills"]\n\n[harness.claude]\nartifacts = ["skills"]\n\n[harness.codex]\nartifacts = ["skills"]\n' > henia.toml; }
> skill() { mk "skills/$1/SKILL.md" "---\nname: $1\ndescription: The $1 skill.\n$2---\n$3"; }
> comp() { mkdir -p .henia/components && cp "$F/$1.md" .henia/components/; }
> built() { echo "$T/build/$(basename "$P")/$1/skills/$2/SKILL.md"; }
> out() { henia build --output "$T/build/$(basename "$P")" > /dev/null 2> "$T/err" && sed '1,/^---$/d' "$(built "$1" "$2")" | sed '1{/^$/d;}' || sed "s|$P/||g" "$T/err"; }
> echo staged
staged
```

## Errors

Every source error in a skill prints, and the skill does not render.

```scrut
$ proj errors && comp menu
> skill broken '' '# Broken\n\n:::menu\nItems.\n\n:::rotues\nx\n:::\n'
> henia build --output "$T/build/errors" > /dev/null 2> "$T/err"
> grep -o -E 'unclosed-directive|unknown-component' "$T/err" | sort -u
> test -e "$(built claude broken)" || echo "not rendered"
unclosed-directive
unknown-component
not rendered
```

## Components

A container component renders its template around the call's body. A boolean
attribute that is left out holds false.

```scrut
$ proj routes && comp routes
> skill routed '' '# Routed\n\n:::routes{ordered}\n| Argument | Route |\n|---|---|\n| fix | debug |\n:::\n'
> out claude routed | grep -v '^$'
# Routed
## Routes
Apply the first row that matches `$ARGUMENTS`:
| Argument | Route |
|---|---|
| fix | debug |
```

```scrut
$ out codex routed | grep -c -E 'Protocol|dispatch'
0
[1]
```

## Conditions

`:::when{target=…}` keeps its body only for the listed harnesses.

```scrut
$ proj when && skill gated '' '# Gated\n\nShared.\n\n:::when{target="claude"}\nClaude only.\n:::\n'
> out claude gated | grep -v '^$'; echo ---; out codex gated | grep -v '^$'
# Gated
Shared.
Claude only.
---
# Gated
Shared.
```

## Includes

`::include{ref=…}` grafts a section of another skill as that skill renders for
the same harness.

```scrut
$ proj include
> skill b '' '# B\n\n## Usage\n\n:::when{target="codex"}\nCodex usage.\n:::\n\n:::when{target="claude"}\nClaude usage.\n:::\n\n## Other\n\nOther text.\n'
> skill a '' '# A\n\n::include{ref="b#usage"}\n'
> out codex a | grep -v '^$'; echo ---; out claude a | grep -v '^$'
# A
## Usage
Codex usage.
---
# A
## Usage
Claude usage.
```

## Template actions

A Go template action in a skill body is an error; frontmatter strings stay
templated.

```scrut
$ proj action && skill valued 'henia:\n  variables:\n    x: Valued.\n' '# Valued\n\nValue {{.x}}.\n'
> out claude valued | grep -o template-action | sort -u
> test -e "$(built claude valued)" || echo "not rendered"
template-action
not rendered
```

```scrut
$ proj frontmatter && mk skills/described/SKILL.md '---\nname: described\ndescription: "{{.x}}"\nhenia:\n  variables:\n    x: Valued.\n---\n# Described\n'
> out claude described > /dev/null; grep '^description:' "$(built claude described)"
description: Valued.
```

## Related

`:related[name]` expands as before; a `:::related` container renders a
`## Related` heading followed by its body.

```scrut
$ proj related && skill git '' '# Git\n\nCommit.\n'
> skill linked '' '# Linked\n\n:related[git]\n\n:::related\n- Read the guide.\n:::\n'
> out codex linked
# Linked

run first: `henia context git`

## Related

- Read the guide.
```

## Directive-free skills

A skill without directives builds exactly as before components, whatever
Markdown it uses.

```scrut
$ proj plain && mk skills/plain/SKILL.md '---\nname: plain\ndescription: Plain.\n---\n\nPlain\n=====\n\n* one\n* two\n\n+ three\n\n1) first\n\n\tindented code\n\nA hard break   \nand a \\* star and [a link](../plain/SKILL.md).\n\n***\n\n| a|b |\n|-|:-:|\n|1|2|\n\n~~~go\nx := 1\n~~~\n'
> out codex plain | cat -et
Plain$
=====$
$
* one$
* two$
$
+ three$
$
1) first$
$
^Iindented code$
$
A hard break   $
and a \* star and [a link](../plain/SKILL.md).$
$
***$
$
| a|b |$
|-|:-:|$
|1|2|$
$
~~~go$
x := 1$
~~~$
```

## Harness selection

`henia build --harness` writes the selected harnesses; conditions still name
every configured harness.

```scrut
$ proj custom && printf '\n[harness.extra]\nartifacts = ["skills"]\n' >> henia.toml
> skill gated '' '# Gated\n\nShared.\n\n:::when{target="extra"}\nExtra only.\n:::\n'
> henia build --harness claude --output "$T/build/custom" > /dev/null 2>&1 && sed '1,/^---$/d' "$(built claude gated)" | grep -v '^$'
# Gated
Shared.
```

A skill the harness does not build still renders for its includes.

```scrut
$ proj dynamic && printf '\n[harness.claude.skills]\nstatic = ["a"]\n' >> henia.toml
> skill b '' '# B\n\n## Usage\n\nB usage.\n'
> skill a '' '# A\n\n::include{ref="b#usage"}\n'
> out claude a | grep -v '^$'
# A
## Usage
B usage.
```

## Error scope

A failed skill does not stop an unrelated one from building.

```scrut
$ proj partial && skill a '' '# A\n\n::include{ref="missing"}\n' && skill c '' '# C\n\nFine.\n'
> henia build --output "$T/build/partial" > /dev/null 2> "$T/err" || grep -o missing-include "$T/err" | sort -u
> test -e "$(built claude c)" && echo "c written"
> test -e "$(built claude a)" || echo "a not rendered"
missing-include
c written
a not rendered
```

A source error in a called component stops its caller, even in a branch the
template does not execute.

```scrut
$ proj field && mk .henia/components/typo.md '---\nform: leaf\nbody:\n  kind: none\n---\n{{if false}}{{.typo}}{{end}}OK\n'
> skill a '' '# A\n\n::typo\n'
> out claude a | grep -o -E '[^ ]+ error \[unknown-field\]' | sort -u
.henia/components/typo.md:6:15: error [unknown-field]
```

An included skill's error stops the including skill, even when the include
expands to nothing for the harness.

```scrut
$ proj empty && skill b '' '# B\n\n:::when{target="claude"}\n::rotues\n:::\n' && skill a '' '# A\n\n::include{ref="b"}\n'
> out codex a | sed -n '/transform a for codex/,+1p'
skills/a: transform a for codex: skills/b/SKILL.md:8:1: error [unknown-component] unknown component rotues
  note: included from skills/a/SKILL.md:7:1
```

The real error of an included skill is reported, not a missing include.

```scrut
$ proj hidden && mk skills/b/SKILL.md '---\nname: b\ndescription: "{{"\n---\n# B\n\n## Usage\n\nB.\n' && skill a '' '# A\n\n::include{ref="b#usage"}\n'
> out codex a | sed -n '/transform a for codex/,+1p'
skills/a: transform a for codex: skills/b/SKILL.md: error transform frontmatter "description": parse template: template: content:1: unclosed action
  note: included from skills/a/SKILL.md:7:1
```

## Show

`henia show` renders a library skill through the component pipeline. A project
`head` component renders the `:::head` block that `--head` prints.

```scrut
$ mkdir -p "$T/repo/.henia/components" && cd "$T/repo" && git init -q && cp "$F/head.md" .henia/components/
> mk .henia/skills/hybrid/SKILL.md '---\nname: hybrid\ndescription: Hybrid.\n---\n\n# Hybrid\n\n:::head{title="X"}\nHead body.\n:::\n\n## Detail\n\nMore.\n'
> henia show hybrid --head | sed -n 1,3p
## X

Head body.
```

A render that includes another skill's section is cached until that section
changes, and fails once the included skill gains an error.

```scrut
$ mk .henia/skills/b/SKILL.md '---\nname: b\ndescription: B.\n---\n\n# B\n\n## Usage\n\nOld usage.\n'
> mk .henia/skills/a/SKILL.md '---\nname: a\ndescription: A.\n---\n\n# A\n\n::include{ref="b#usage"}\n'
> henia show a | grep 'usage\.'
> mk .henia/skills/b/SKILL.md '---\nname: b\ndescription: B.\n---\n\n# B\n\n## Usage\n\nNew usage.\n'
> henia show a | grep 'usage\.'
> printf '\n:::rotues\nx\n:::\n' >> .henia/skills/b/SKILL.md
> henia show a 2>&1 | grep -o unknown-component | sort -u
Old usage.
New usage.
unknown-component
```

The cache serves a render only for the harness it was rendered for.

```scrut
$ mk .henia/skills/gated/SKILL.md '---\nname: gated\ndescription: Gated.\n---\n\n# Gated\n\n:::when{target="claude"}\nClaude only.\n:::\n'
> henia show gated --harness claude | grep -c 'Claude only'
> henia show gated --harness codex | grep -c 'Claude only'
1
0
[1]
```

A cached render is replaced once a value it reads is no longer known in its
package.

```scrut
$ mk .henia/components/opt.md '---\nform: leaf\nbody:\n  kind: none\n---\n[{{.optional}}]\n'
> mk .henia/skills/sib/SKILL.md '---\nname: sib\ndescription: Sib.\noptional: x\n---\n\n# Sib\n'
> mk .henia/skills/user/SKILL.md '---\nname: user\ndescription: User.\n---\n\n# User\n\n::opt\n'
> henia show user | grep -c '^\[\]$'
> mk .henia/skills/sib/SKILL.md '---\nname: sib\ndescription: Sib.\n---\n\n# Sib\n'
> henia show user | grep -o unknown-field | sort -u
1
unknown-field
```

An include a component template generates resolves like an authored one.

```scrut
$ mk .henia/components/inc.md '---\nform: leaf\nbody:\n  kind: none\n---\n{{if true}}::include{ref="c#usage"}{{end}}\n'
> mk .henia/skills/c/SKILL.md '---\nname: c\ndescription: C.\n---\n\n# C\n\n## Usage\n\nC usage.\n'
> mk .henia/skills/gen/SKILL.md '---\nname: gen\ndescription: Gen.\n---\n\n# Gen\n\n::inc\n'
> henia show gen | grep -v '^$'
# Gen
## Usage
C usage.
```

A section, the contents and the head of a skill that does not render print its
diagnostics.

```scrut
$ mk .henia/skills/bad/SKILL.md '---\nname: bad\ndescription: Bad.\n---\n\n# Bad\n\n## Usage\n\n::rotues\n'
> for mode in 'bad#usage' 'bad --toc' 'bad --head'; do henia show $mode | grep -o -E 'stopped|unknown-component' | tr '\n' ' '; echo; done
stopped unknown-component 
stopped unknown-component 
stopped unknown-component 
```

An included skill keeps its own package's harness variables.

```scrut
$ D="$T/dep"; mk "$D/skills/dep/SKILL.md" '---\nname: dep\ndescription: Dep.\n---\n\n# Dep\n\n## Usage\n\n:::when{flavor="dep"}\nDep flavored.\n:::\n'
> printf '[harness.codex]\nartifacts = ["skills"]\n\n[harness.codex.variables]\nflavor = "dep"\n' > "$D/henia.toml"
> printf '[harness.codex]\nartifacts = ["skills"]\n\n[harness.codex.variables]\nflavor = "own"\n' > henia.toml
> mk .henia/skills/own/SKILL.md '---\nname: own\ndescription: Own.\n---\n\n# Own\n\n::include{ref="dep#usage"}\n'
> henia show own --harness codex --package "$D" | grep -v '^$'
# Own
## Usage
Dep flavored.
```

```scrut
$ rm -rf "$T"
```
