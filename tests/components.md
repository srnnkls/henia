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

## Templates

A skill body templates the skill's values, as frontmatter strings do. Code
spans and blocks stay literal, and a field no skill or harness knows fails.

```scrut
$ proj action && skill valued 'henia:\n  variables:\n    x: Valued.\n' '# Valued\n\nValue {{.x}} in `{{.x}}`.\n'
> out claude valued
# Valued

Value Valued. in `{{.x}}`.
```

```scrut
$ skill unknown '' '# Unknown\n\nValue {{.nowhere}}.\n'
> out claude unknown | grep -o invalid-template | sort -u
invalid-template
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

## Lint

Source lint reports a call to an undeclared component and a project component
nothing calls; a component only another component's template includes counts
as called.

```scrut
$ proj lint-source && lint() { henia lint "$@" 2>&1 | sed "s|$P/||g"; }
> mk .henia/components/note.md '---\nform: container\nbody:\n  kind: markdown\n---\n> Note\n\n::body\n'
> mk .henia/components/part.md '---\nform: leaf\nbody:\n  kind: none\n---\nPart.\n'
> mk .henia/components/box.md '---\nform: leaf\nbody:\n  kind: none\n---\n::include{component="part"}\n'
> mk .henia/components/stale.md '---\nform: leaf\nbody:\n  kind: none\n---\nOld.\n'
> skill deploy '' ':::note\nNote.\n:::\n\n::box\n\n:::gone\nText.\n:::\n'
> lint skills --strict
.henia/components/stale.md: warning [unused-component] component stale is never called
skills/deploy/SKILL.md:11:1: error [unknown-component] unknown component gone
Error: lint found 2 diagnostic(s)
```

Expanded lint checks every target. A finding in a component's output points at
the skill's call, with a note naming the component line.

```scrut
$ proj lint-expanded
> mk .henia/components/title.md '---\nform: leaf\nattributes:\n  title:\n    type: string\nbody:\n  kind: none\n---\n# {{.title}}\n'
> mk .henia/components/bad.md '---\nform: leaf\nbody:\n  kind: none\n---\n::include{component="title" title="X" nope=1}\n'
> skill gen '' '::bad\n\n:::when{target="clod"}\nx\n:::\n\n:::when{nope=true}\ny\n:::\n'
> lint skills | sed 's/; want one of .*//'
skills/gen/SKILL.md:5:1: error [unknown-attribute] title does not declare attribute nope
  note: expanded from component .henia/components/bad.md:6
skills/gen/SKILL.md:7:1: error [unknown-target] unknown target clod
skills/gen/SKILL.md:11:1: error [unknown-variable] no harness declares variable nope
Error: lint found 3 diagnostic(s)
```

A slot a component's output applies counts as applied, rendered with the
skill's values and the harness variables.

```scrut
$ proj lint-partial && printf 'artifacts = ["skills"]\n\n[harness.claude]\nartifacts = ["skills"]\n\n[harness.claude.variables]\nlabel = "Providers"\n' > henia.toml
> mk .henia/components/context.md '---\nform: leaf\nattributes:\n  slots:\n    type: list\n    of: slot\nbody:\n  kind: none\n---\n{{.label}}:\n\n:slot[{{range .slots}}{{.}} {{end}}]\n'
> skill git 'henia:\n  slots:\n    git.commits:\n    git.branching:\n' '::context{slots="git.commits"}\n\n# Git\n'
> lint skills --strict
skills/git/SKILL.md:7:5: warning [unused-slot] slot git.branching is declared but no scanned skill applies it
Error: lint found 1 diagnostic(s)
```

`missing-section` reports a slot offer whose section the skill does not
render. A heading that comes from a component counts.

```scrut
$ proj lint-sections && mk .henia/components/article.md '---\nform: leaf\nbody:\n  kind: none\n---\n# {{.name}}\n\n## Commits\n'
> skill git 'henia:\n  slots:\n    git.commits:\n    git.branching:\n  provides:\n    git.commits: {section: commits}\n    git.branching: {section: branching}\n' '::article\n\n:slot[git.commits git.branching]\n'
> lint skills
skills/git/SKILL.md:10:30: error [missing-section] slot git.branching offers section #branching, which the skill does not render
Error: lint found 1 diagnostic(s)
```

Rendered lint reports an authored link at its source position with the target
as written.

```scrut
$ proj lint-links && skill gamma '' '# Gamma\n\n## Use\nSee [x](gone.md)\n'
> lint skills | grep broken-link
skills/gamma/SKILL.md:8:6: warning [broken-link] local reference does not exist: gone.md
```

A link into a served skill keeps its authored target and position.

```scrut
$ proj lint-served && printf '\n[harness.claude.skills]\nstatic = ["gamma"]\n\n[harness.codex.skills]\nstatic = ["gamma"]\n' >> henia.toml
> skill delta '' '# Delta\n\nDelta body.\n'
> skill gamma '' '# Gamma\n\n## Use\nSee [x](../delta/gone.md)\n'
> lint skills | grep broken-link
skills/gamma/SKILL.md:8:6: warning [broken-link] local reference does not exist: ../delta/gone.md
```

## Removed composition

A `.henia/templates` directory raises `removed-composition` at its path on
build, lint and `henia show`, and no skill renders through its templates.

```scrut
$ proj removed-dir && mk .henia/templates/head.md.tmpl 'Project context.\n'
> skill notes '' '{{template "head" .}}\n# Notes\n'
> out claude notes | grep -o -E '[^ ]+ error \[removed-composition\]' | sort -u
> cat "$(built claude notes)" 2>/dev/null | grep -c 'Project context'
> lint skills | grep -o -E '^[^ ]+ error \[removed-composition\]' | sort -u
> mk .henia/skills/hybrid/SKILL.md '---\nname: hybrid\ndescription: Hybrid.\n---\n\n# Hybrid\n'
> henia show hybrid 2>&1 | sed "s|$P/||g" | grep -o -E '[^ ]+ error \[removed-composition\]' | sort -u
.henia/templates: error [removed-composition]
0
.henia/templates: error [removed-composition]
.henia/templates: error [removed-composition]
```

A `henia.layout` key raises `removed-composition` at the key's line and column.

```scrut
$ proj removed-layout && skill deploy 'henia:\n  layout: article\n' '# Deploy\n'
> lint skills | grep -o -E '^[^ ]+ error \[removed-composition\]'
> out claude deploy | grep -o -E '[^ ]+ error \[removed-composition\]' | sort -u
skills/deploy/SKILL.md:5:3: error [removed-composition]
skills/deploy/SKILL.md:5:3: error [removed-composition]
```

```scrut
$ rm -rf "$T"
```
