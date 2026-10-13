# Henia Slot Tests

## Fixture

Slots resolve over the library: global skill packages, and the project's own
skills in `.henia/skills`.

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P); G="$T/data/henia/packages/global"; P="$T/my repo"
> henia() { env HENIA_HARNESS=none XDG_DATA_HOME="$T/data" XDG_CACHE_HOME="$T/cache" XDG_CONFIG_HOME="$T/config" henia "$@"; }
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> skill() { mk "$1/SKILL.md" "---\nname: $(basename "$1")\ndescription: Skill $(basename "$1") for tests.\n$2---\n\n# $(basename "$1")\n$3"; }
> slots() { dir=$1; shift; (cd "$dir" && henia slots "$@"; echo "exit $?") 2>&1 | sed "s|$T/||g" | tr '\t' ' '; }
> skill "$G/tropos/skills/loqui" 'henia:\n  provides:\n    code.style.go:\n    code.style.python: {section: python}\n'
> skill "$G/tropos/skills/gbranch" 'henia:\n  provides:\n    git.branching:\n'
> skill "$G/tropos/skills/gcommits" 'henia:\n  provides:\n    git.commits:\n'
> skill "$G/acme/skills/style" 'henia:\n  provides:\n    code.style:\n    review.criteria:\n'
> skill "$G/acme/skills/crlf" 'henia:\n  provides:\n    code.validation:\n'
> mkdir -p "$P" && git -C "$P" init -q
> skill "$P/.henia/skills/effect" 'henia:\n  provides:\n    code.style:\n'
> skill "$P/.henia/skills/commits" 'henia:\n  provides:\n    git.commits:\n'
> mkdir -p "$P/sub/dir"
```

## Untyped resolution

Without declarations every slot composes as `keyed(list)`. Each provider prints
the command that reads it; a project provider shadows global providers of its
slot and of its sub-slots.

```scrut
$ slots "$P" git.branching
git.branching global henia show tropos:gbranch
exit 0
```

```scrut
$ slots "$P" code.style review.criteria
code.style project henia show project:effect
review.criteria global henia show acme:style
exit 0
```

```scrut
$ slots "$P/sub/dir" git
git.commits project henia show project:commits
git.branching global henia show tropos:gbranch
exit 0
```

Outside the project only the global packages count.

```scrut
$ slots "$T" code.style
code.style global henia show acme:style
code.style.go global henia show tropos:loqui
code.style.python global henia show 'tropos:loqui#python'
exit 0
```

```scrut
$ slots "$P" git.nothing
git.nothing none -
exit 0
```

## Declarations

Any declaration switches validation on.

```scrut
$ skill "$G/tropos/skills/owner" 'henia:\n  slots:\n    code.style:\n    code.validation:\n    git.branching:\n    git.commits: list\n    review.criteria: unique\n'
> skill "$P/.henia/skills/typo" 'henia:\n  provides:\n    code.styles:\n    git.brnching:\n'
> slots "$P" git.branchs code.styles
code.styles unknown my repo/.henia/skills/typo/SKILL.md
git.branchs undeclared -
code.styles undeclared -
exit 0
```

```scrut
$ slots "$P" --check
code.styles unknown my repo/.henia/skills/typo/SKILL.md
git.brnching unknown my repo/.henia/skills/typo/SKILL.md
Error: slots found 2 problem(s)
exit 1
```

```scrut
$ rm -rf "$P/.henia/skills/typo" && slots "$P" --check
exit 0
```

## Types and priorities

A `unique` slot reports more than one surviving provider as a conflict.

```scrut
$ skill "$G/tropos/skills/criteria" 'henia:\n  provides:\n    review.criteria:\n'
> slots "$T" review.criteria
review.criteria global henia show acme:style
review.criteria global henia show tropos:criteria
review.criteria conflict -
exit 0
```

A project provider outranks both and resolves the conflict.

```scrut
$ skill "$P/.henia/skills/criteria" 'henia:\n  provides:\n    review.criteria:\n'
> slots "$P" review.criteria
review.criteria project henia show project:criteria
exit 0
```

`priority: fallback` places a project provider beside the global fallbacks.

```scrut
$ skill "$P/.henia/skills/effect" 'henia:\n  provides:\n    code.style.python: {priority: fallback}\n'
> slots "$P" code.style.python
code.style.python project henia show project:effect
code.style global henia show acme:style
code.style.python global henia show 'tropos:loqui#python'
exit 0
```

`priority: force` lets a global provider outrank the project.

```scrut
$ skill "$G/tropos/skills/gcommits" 'henia:\n  provides:\n    git.commits: {priority: force}\n'
> slots "$P" git.commits
git.commits global henia show tropos:gcommits
exit 0
```

A sub-slot of a slot that is not keyed is unknown; malformed entries and
conflicting declarations are reported.

```scrut
$ skill "$G/acme/skills/bad" 'henia:\n  slots:\n    review.criteria: list\n    lint.rules: bogus\n  provides:\n    git.commits.extra:\n    code.style: {priority: urgent}\n'
> slots "$P" --check
lint.rules invalid data/henia/packages/global/acme/skills/bad/SKILL.md
code.style invalid data/henia/packages/global/acme/skills/bad/SKILL.md
review.criteria conflict data/henia/packages/global/acme/skills/bad/SKILL.md
review.criteria conflict data/henia/packages/global/tropos/skills/owner/SKILL.md
git.commits.extra unknown data/henia/packages/global/acme/skills/bad/SKILL.md
Error: slots found 5 problem(s)
exit 1
```

```scrut
$ henia slots git.commits --check 2>&1
Error: --check takes no slots, --explain, --json, --markdown or --inline
[1]
```

## Configuration

`[slots."<slot>"] disable` in `henia.toml` turns providers off for a slot and
its sub-slots.

```scrut
$ rm -rf "$G/acme/skills/bad" && mk "$P/henia.toml" '[slots."code.style"]\ndisable = ["acme:style"]\n'
> slots "$P" code.style.python
code.style.python project henia show project:effect
code.style.python global henia show 'tropos:loqui#python'
exit 0
```

## Lineage

`--explain` shows each provider's status and the origin of its priority, the
provider that shadows it and the declaration that types its slot.

```scrut
$ slots "$P" --explain code.style.python git.commits
code.style.python
  code.style (keyed(list), declared by data/henia/packages/global/tropos/skills/owner/SKILL.md)
    disabled style [global, fallback from tier] data/henia/packages/global/acme/skills/style/SKILL.md
  code.style.python (keyed(list) of code.style, declared by data/henia/packages/global/tropos/skills/owner/SKILL.md)
    selected effect [project, fallback from entry] my repo/.henia/skills/effect/SKILL.md
    selected loqui [global, fallback from tier] data/henia/packages/global/tropos/skills/loqui/SKILL.md
git.commits
  git.commits (list, declared by data/henia/packages/global/tropos/skills/owner/SKILL.md)
    shadowed commits [project, normal from tier] my repo/.henia/skills/commits/SKILL.md
      by gcommits [global, force from entry] for git.commits, data/henia/packages/global/tropos/skills/gcommits/SKILL.md
    selected gcommits [global, force from entry] data/henia/packages/global/tropos/skills/gcommits/SKILL.md
exit 0
```

```scrut
$ (cd "$P" && henia slots --json review.criteria) | jq -c '.providers[] | [.ref, .status, .priority, .explicit, .shadowed_by.ref]'
["project:criteria","selected","normal",false,null]
["acme:style","shadowed","fallback",false,"project:criteria"]
["tropos:criteria","shadowed","fallback",false,"project:criteria"]
```

## Applying

A `:slot[...]` directive in a skill body applies slots. `henia show` and
`henia build` render it as a preload of `henia slots`.

```scrut
$ skill "$G/tropos/skills/review" 'henia:\n  slots:\n    review.lenses:\n' '\nCriteria:\n\n:slot[review.criteria review.lenses]\n'
> (cd "$P" && henia show tropos:review) | sed "s|$T/||g" | tr '\t' ' '
# review

Criteria:

- `review.criteria`: `henia show project:criteria`
- `review.lenses`: no provider
```

```scrut
$ slots "$P" --explain review.criteria
review.criteria
  review.criteria (unique, declared by data/henia/packages/global/tropos/skills/owner/SKILL.md)
    applied by review
    selected criteria [project, normal from tier] my repo/.henia/skills/criteria/SKILL.md
    shadowed style [global, fallback from tier] data/henia/packages/global/acme/skills/style/SKILL.md
      by criteria [project, normal from tier] for review.criteria, my repo/.henia/skills/criteria/SKILL.md
    shadowed criteria [global, fallback from tier] data/henia/packages/global/tropos/skills/criteria/SKILL.md
      by criteria [project, normal from tier] for review.criteria, my repo/.henia/skills/criteria/SKILL.md
exit 0
```

An application of a slot no skill declares is a problem.

```scrut
$ skill "$P/.henia/skills/consumer" '' '\n:slot[lint.rules]\n'
> slots "$P" --check
lint.rules undeclared my repo/.henia/skills/consumer/SKILL.md
Error: slots found 1 problem(s)
exit 1
```

## Specificity

A provider of a `unique` slot's key supersedes a provider of the slot for
requests within that key. Other keys keep the enclosing provider.

```scrut
$ skill "$G/tropos/skills/testing" 'henia:\n  slots:\n    test.approach: keyed(unique)\n' '\n:slot[test.approach.frontend test.approach.backend]\n'
> skill "$G/acme/skills/tdd" 'henia:\n  provides:\n    test.approach:\n'
> skill "$G/acme/skills/frontend" 'henia:\n  provides:\n    test.approach.frontend:\n'
> slots "$P" test.approach.frontend
test.approach.frontend global henia show acme:frontend
exit 0
```

```scrut
$ slots "$P" test.approach.backend
test.approach global henia show acme:tdd
exit 0
```

```scrut
$ slots "$P" test.approach
test.approach.frontend global henia show acme:frontend
test.approach global henia show acme:tdd
exit 0
```

`--explain` and `--json` show the superseded provider as shadowed by the more
specific one.

```scrut
$ slots "$P" --explain test.approach.frontend
test.approach.frontend
  test.approach (keyed(unique), declared by data/henia/packages/global/tropos/skills/testing/SKILL.md)
    shadowed tdd [global, fallback from tier] data/henia/packages/global/acme/skills/tdd/SKILL.md
      by frontend [global, fallback from tier] for test.approach.frontend, more specific, data/henia/packages/global/acme/skills/frontend/SKILL.md
  test.approach.frontend (keyed(unique) of test.approach, declared by data/henia/packages/global/tropos/skills/testing/SKILL.md)
    applied by testing
    selected frontend [global, fallback from tier] data/henia/packages/global/acme/skills/frontend/SKILL.md
exit 0
```

```scrut
$ (cd "$P" && henia slots --json test.approach.frontend) | jq -c '.providers[] | [.ref, .status, .shadowed_by.ref]'
["acme:frontend","selected",null]
["acme:tdd","shadowed","acme:frontend"]
```

`henia query` follows a slot to each provider it selects, as it follows a link.

```scrut
$ (cd "$P" && henia query '(skill :id "testing" (reaches (skill) @t))'; henia query '(skill :id "tdd" (inbound (skill) @from))')
@t  frontend  skill
@t  tdd  skill
@from  testing  skill
```

```scrut
$ (cd "$P" && henia show tropos:testing) | sed "s|$T/||g"
# testing

- `test.approach.frontend`: `henia show acme:frontend`
- `test.approach`: `henia show acme:tdd`
```

## Configured providers

`[slots."<slot>"] provide` in `henia.toml` makes a library skill provide a
slot its frontmatter does not name. A reference to no skill is invalid.

```scrut
$ skill "$G/acme/skills/pony" '' ''
> mk "$P/henia.toml" '[slots."test.approach.backend"]\nprovide = ["acme:pony", "acme:ghost"]\n'
> slots "$P" test.approach.backend
test.approach.backend global henia show acme:pony
test.approach.backend invalid my repo/henia.toml
exit 0
```

```scrut
$ (cd "$P" && henia show tropos:testing) | sed "s|$T/||g"
# testing

- `test.approach.frontend`: `henia show acme:frontend`
- `test.approach.backend`: `henia show acme:pony`
- `test.approach.backend`: invalid in `my repo/henia.toml`
```

A package's own `henia.toml` provides for every project that uses the package.

```scrut
$ skill "$G/acme/skills/lazy" '' ''
> mk "$G/acme/henia.toml" '[slots."test.approach.mobile"]\nprovide = ["acme:lazy", "acme:absent"]\n'
> slots "$P" test.approach.mobile
test.approach.mobile global henia show acme:lazy
test.approach.mobile invalid data/henia/packages/global/acme/henia.toml
exit 0
```

A provide the project repeats from a package counts once.

```scrut
$ mk "$P/henia.toml" '[slots."test.approach.mobile"]\nprovide = ["acme:lazy"]\n'
> slots "$P" test.approach.mobile
test.approach.mobile global henia show acme:lazy
test.approach.mobile invalid data/henia/packages/global/acme/henia.toml
exit 0
```

## Harness files

A Markdown harness file renders its slots when it is built, since nothing
preloads it at runtime. An inlined slot becomes its providers' content, a
pointer the list of providers, and a slot without a provider nothing.
`replace` applies afterwards.

```scrut
$ skill "$P/skills/demo" '' ''
> mk "$P/AGENTS.md" '# Agents\n\nCommits:\n\n:slot[git.commits]{.inline}\n\nBranching:\n\n:slot[git.branching]\n\n:slot[review.lenses]{.inline}\n\nDone.\n'
> printf '\n[harness.claude.files]\n"CLAUDE.md" = { source = "AGENTS.md", replace = { "Done." = "Finished." } }\n' >> "$P/henia.toml"
> (cd "$P" && henia build >/dev/null && cat .henia/build/claude/CLAUDE.md)
# Agents

Commits:

# gcommits

Branching:

- `git.branching`: `henia show tropos:gbranch`

Finished.
```

`henia install` renders the harness files of a global package the same way,
against the global packages and the user `henia.toml`.

```scrut
$ mk "$G/tropos/AGENTS.md" 'Commits:\n\n:slot[git.commits]{.inline}\n\nBranching:\n\n:slot[git.branching]{.inline}\n'
> mk "$G/tropos/henia.toml" '[harness.claude.files]\n"CLAUDE.md" = { source = "AGENTS.md" }\n'
> mk "$T/config/henia/henia.toml" "[install.claude]\npath = \"$T/claude\"\n"
> henia install --harness claude >/dev/null && cat "$T/claude/CLAUDE.md"
Commits:

# gcommits

Branching:

# gbranch
```
