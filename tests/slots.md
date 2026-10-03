# Henia Slot Tests

## Fixture

Global skills live in a directory passed with `--global`; project skills live
under the skill directories of a Git repository.

```scrut
$ T=$(cd "$(mktemp -d)" && pwd -P); G="$T/global skills"; P="$T/my repo"
> mk() { mkdir -p "$(dirname "$1")"; printf -- "$2" > "$1"; }
> slots() { dir=$1; shift; (cd "$dir" && henia slots --global "$G" "$@"; echo "exit $?") 2>&1 | sed "s|$T/||g" | tr '\t' ' '; }
> mk "$G/loqui/SKILL.md"   '---\nname: loqui\nmetadata:\n  provides: "code.style"\n---\n'
> mk "$G/gstyle2/SKILL.md" '---\nname: gstyle2\nmetadata:\n  provides: [code.style, review.criteria]  # two\n---\n'
> mk "$G/gbranch/SKILL.md" '---\nname: gbranch\nmetadata:\n  provides: git.branching\n---\n'
> mk "$G/gcommits/SKILL.md" '---\nname: gcommits\nmetadata:\n  provides: git.commits\n---\n'
> mk "$G/gtests/SKILL.md"  '---\nname: gtests\nmetadata:\n  provides: test.conventions\n---\n'
> mk "$G/gnone/SKILL.md"   '---\nname: gnone\n---\nprovides: code.style\n'
> mk "$G/gcrlf/SKILL.md"   '---\r\nname: gcrlf\r\nmetadata:\r\n  provides: "code.validation"\r\n---\r\n'
> mk "$G/loqui/reference/nested/SKILL.md" '---\nname: nested\nmetadata:\n  provides: code.style\n---\n'
> mk "$G/gpy/SKILL.md"     '---\nname: gpy\nmetadata:\n  provides: code.style.python\n---\n'
> mk "$G/gpylint/SKILL.md" '---\nname: gpylint\nmetadata:\n  provides: code.validation.python\n---\n'
> mk "$G/gstyles/SKILL.md" '---\nname: gstyles\nmetadata:\n  provides: code.styles code.style-guide\n---\n'
> ln -s "$T/nowhere" "$G/broken"
> mkdir -p "$P" && git -C "$P" init -q
> mk "$P/.claude/skills/effect/SKILL.md"    '---\nname: effect\nmetadata:\n  provides: "code.style"\n---\n'
> mk "$P/.agents/skills/effect2/SKILL.md"   "---\nname: effect2\nmetadata:\n  provides: 'code.style'\n---\n"
> mk "$P/.claude/skills/tsval/SKILL.md"     '---\nname: tsval\nmetadata:\n  provides: code.validation.typescript\n---\n'
> mk "$P/.omp/skills/acme-commits/SKILL.md" '---\nname: acme-commits\nmetadata:\n  provides: git.commits\n---\n'
> ln -s "$G/gtests" "$P/.claude/skills/gtests"
> mkdir -p "$P/sub/dir"
```

## Untyped resolution

Without declarations every slot composes as `keyed(list)`: a project provider
shadows global providers of its slot and of its sub-slots.

```scrut
$ slots "$P" git.branching
git.branching global global skills/gbranch/SKILL.md
exit 0
```

```scrut
$ slots "$P" code.style
code.style project my repo/.claude/skills/effect/SKILL.md
code.style project my repo/.agents/skills/effect2/SKILL.md
exit 0
```

```scrut
$ slots "$P" code.style review.criteria
code.style project my repo/.claude/skills/effect/SKILL.md
code.style project my repo/.agents/skills/effect2/SKILL.md
review.criteria global global skills/gstyle2/SKILL.md
exit 0
```

```scrut
$ slots "$P" git.commits
git.commits project my repo/.omp/skills/acme-commits/SKILL.md
exit 0
```

A symlinked global skill reached from the project keeps the project tier.

```scrut
$ slots "$P" test.conventions
test.conventions project my repo/.claude/skills/gtests/SKILL.md
exit 0
```

CRLF frontmatter parses, and a request matches its enclosing slot.

```scrut
$ slots "$P" code.validation.zig
code.validation global global skills/gcrlf/SKILL.md
exit 0
```

```scrut
$ slots "$P" git.nothing
exit 0
```

```scrut
$ slots "$P"
exit 0
```

```scrut
$ slots "$P" git.branching git.branching
git.branching global global skills/gbranch/SKILL.md
exit 0
```

```scrut
$ slots "$P/sub/dir" git.commits
git.commits project my repo/.omp/skills/acme-commits/SKILL.md
exit 0
```

```scrut
$ slots "$T" git.commits
git.commits global global skills/gcommits/SKILL.md
exit 0
```

```scrut
$ slots "$P" git
git.commits project my repo/.omp/skills/acme-commits/SKILL.md
git.branching global global skills/gbranch/SKILL.md
exit 0
```

```scrut
$ slots "$P" code.style.python
code.style project my repo/.claude/skills/effect/SKILL.md
code.style project my repo/.agents/skills/effect2/SKILL.md
exit 0
```

A narrower project provider keeps the broader global one.

```scrut
$ slots "$P" code.validation
code.validation.typescript project my repo/.claude/skills/tsval/SKILL.md
code.validation global global skills/gcrlf/SKILL.md
code.validation.python global global skills/gpylint/SKILL.md
exit 0
```

```scrut
$ slots "$P" code.validation.python
code.validation global global skills/gcrlf/SKILL.md
code.validation.python global global skills/gpylint/SKILL.md
exit 0
```

```scrut
$ slots "$T" code.style
code.style.python global global skills/gpy/SKILL.md
code.style global global skills/gstyle2/SKILL.md
code.style global global skills/loqui/SKILL.md
exit 0
```

## Declarations

Any declaration switches validation on.

```scrut
$ mk "$G/gowner/SKILL.md" '---\nname: gowner\nmetadata:\n  slots: "code.style code.validation git.branching git.commits review.criteria test.conventions"\n---\n'
> mk "$P/.claude/skills/typo/SKILL.md" '---\nname: typo\nmetadata:\n  provides: "code.styles git.brnching"\n---\n'
> slots "$P" git.commits
git.commits project my repo/.omp/skills/acme-commits/SKILL.md
code.styles unknown my repo/.claude/skills/typo/SKILL.md
git.brnching unknown my repo/.claude/skills/typo/SKILL.md
code.styles unknown global skills/gstyles/SKILL.md
code.style-guide unknown global skills/gstyles/SKILL.md
exit 0
```

```scrut
$ slots "$P" git.branchs
code.styles unknown my repo/.claude/skills/typo/SKILL.md
git.brnching unknown my repo/.claude/skills/typo/SKILL.md
code.styles unknown global skills/gstyles/SKILL.md
code.style-guide unknown global skills/gstyles/SKILL.md
git.branchs undeclared -
exit 0
```

```scrut
$ slots "$P" --check
code.styles unknown my repo/.claude/skills/typo/SKILL.md
git.brnching unknown my repo/.claude/skills/typo/SKILL.md
code.styles unknown global skills/gstyles/SKILL.md
code.style-guide unknown global skills/gstyles/SKILL.md
Error: slots found 4 problem(s)
exit 1
```

```scrut
$ rm -rf "$P/.claude/skills/typo" "$G/gstyles" && slots "$P" --check
exit 0
```

## Types and priorities

A `unique` slot reports more than one surviving provider as a conflict.

```scrut
$ mk "$G/gowner/SKILL.md" '---\nname: gowner\nmetadata:\n  slots: "code.style:keyed(list) code.validation git.branching git.commits:list review.criteria:unique test.conventions"\n---\n'
> mk "$G/gcriteria/SKILL.md" '---\nname: gcriteria\nmetadata:\n  provides: review.criteria\n---\n'
> slots "$P" review.criteria
review.criteria global global skills/gcriteria/SKILL.md
review.criteria global global skills/gstyle2/SKILL.md
review.criteria conflict -
exit 0
```

A project provider outranks both defaults and resolves the conflict.

```scrut
$ mk "$P/.claude/skills/criteria/SKILL.md" '---\nname: criteria\nmetadata:\n  provides: review.criteria\n---\n'
> slots "$P" review.criteria
review.criteria project my repo/.claude/skills/criteria/SKILL.md
exit 0
```

`@fallback` places a project provider beside the global fallbacks.

```scrut
$ mk "$P/.claude/skills/effect/SKILL.md" '---\nname: effect\nmetadata:\n  provides: "code.style.python@fallback"\n---\n'
> rm -rf "$P/.agents/skills/effect2" && slots "$P" code.style.python
code.style.python project my repo/.claude/skills/effect/SKILL.md
code.style.python global global skills/gpy/SKILL.md
code.style global global skills/gstyle2/SKILL.md
code.style global global skills/loqui/SKILL.md
exit 0
```

`@force` lets a global provider outrank the project.

```scrut
$ mk "$G/gcommits/SKILL.md" '---\nname: gcommits\nmetadata:\n  provides: git.commits@force\n---\n'
> slots "$P" git.commits
git.commits global global skills/gcommits/SKILL.md
exit 0
```

A sub-slot of a slot that is not keyed is unknown; malformed entries and
conflicting declarations are reported.

```scrut
$ mk "$G/bad/SKILL.md" '---\nname: bad\nmetadata:\n  slots: "review.criteria:list lint.rules:bogus"\n  provides: "git.commits.extra code.style@urgent"\n---\n'
> slots "$P" --check
lint.rules:bogus invalid global skills/bad/SKILL.md
review.criteria conflict global skills/bad/SKILL.md
review.criteria conflict global skills/gowner/SKILL.md
git.commits.extra unknown global skills/bad/SKILL.md
code.style@urgent invalid global skills/bad/SKILL.md
Error: slots found 5 problem(s)
exit 1
```

```scrut
$ henia slots git.commits --check 2>&1
Error: --check takes no slots, --explain or --json
[1]
```

## Lineage

`--explain` shows each provider's status and the origin of its priority, the
provider that shadows it and the declaration that types its slot.

```scrut
$ rm -rf "$G/bad" && slots "$P" --explain code.style.python git.commits
code.style.python
  code.style (keyed(list), declared by global skills/gowner/SKILL.md)
    selected gstyle2 [global, fallback from tier] global skills/gstyle2/SKILL.md
    selected loqui [global, fallback from tier] global skills/loqui/SKILL.md
  code.style.python (keyed(list) of code.style, declared by global skills/gowner/SKILL.md)
    selected effect [project, fallback from entry] my repo/.claude/skills/effect/SKILL.md
    selected gpy [global, fallback from tier] global skills/gpy/SKILL.md
git.commits
  git.commits (list, declared by global skills/gowner/SKILL.md)
    shadowed acme-commits [project, normal from tier] my repo/.omp/skills/acme-commits/SKILL.md
      by gcommits [global, force from entry] for git.commits, global skills/gcommits/SKILL.md
    selected gcommits [global, force from entry] global skills/gcommits/SKILL.md
exit 0
```

```scrut
$ (cd "$P" && henia slots --global "$G" --json review.criteria) | jq -c '.providers[] | [.skill, .status, .priority, .explicit, .shadowed_by.skill]'
["criteria","selected","normal",false,null]
["gcriteria","shadowed","fallback",false,"criteria"]
["gstyle2","shadowed","fallback",false,"criteria"]
```

A declared slot without providers is listed as such.

```scrut
$ mk "$G/docs/SKILL.md" '---\nname: docs\nmetadata:\n  slots: "docs.style:unique"\n---\n'
> slots "$P" --explain docs
docs
  docs.style (unique, declared by global skills/docs/SKILL.md)
    no providers
exit 0
```

## Consumers

`metadata.applies` names the slots a skill preloads; `--for` resolves them.

```scrut
$ mk "$G/gconsumer/SKILL.md" '---\nname: gconsumer\nmetadata:\n  applies: "review.criteria git.commits"\n---\n'
> slots "$P" --for gconsumer
review.criteria project my repo/.claude/skills/criteria/SKILL.md
git.commits global global skills/gcommits/SKILL.md
exit 0
```

```scrut
$ slots "$P" --explain review.criteria
review.criteria
  review.criteria (unique, declared by global skills/gowner/SKILL.md)
    applied by gconsumer
    selected criteria [project, normal from tier] my repo/.claude/skills/criteria/SKILL.md
    shadowed gcriteria [global, fallback from tier] global skills/gcriteria/SKILL.md
      by criteria [project, normal from tier] for review.criteria, my repo/.claude/skills/criteria/SKILL.md
    shadowed gstyle2 [global, fallback from tier] global skills/gstyle2/SKILL.md
      by criteria [project, normal from tier] for review.criteria, my repo/.claude/skills/criteria/SKILL.md
exit 0
```

An application of a slot no skill declares is a problem.

```scrut
$ mk "$G/gconsumer/SKILL.md" '---\nname: gconsumer\nmetadata:\n  applies: "review.criteria lint.rules"\n---\n'
> slots "$P" --check
lint.rules undeclared global skills/gconsumer/SKILL.md
Error: slots found 1 problem(s)
exit 1
```

```scrut
$ slots "$P" --for nosuch
Error: no installed skill named "nosuch"
exit 1
```
