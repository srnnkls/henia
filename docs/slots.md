# Slots

A *slot* is a named extension point in a skill. One skill declares it, other
skills fill it, and Henia resolves the providers whenever the skill is read, so
a project can extend skills it never wrote.

Tropos's `code` skill declares `code.style` and `code.validation`, and Loqui
provides them for each language. In the project from
[Getting started](../README.md#getting-started):

```console
$ henia slots code.style code.validation
code.style.bash	dependency	henia show loqui:loqui
code.style.elisp	dependency	henia show loqui:loqui
code.style.go	dependency	henia show loqui:loqui
code.style.python	dependency	henia show loqui:loqui
code.style.rust	dependency	henia show loqui:loqui
code.style.zig	dependency	henia show loqui:loqui
code.validation.go	dependency	henia show loqui:loqui	go vet ./... && go build ./...
```

Each row names a slot, the tier its provider comes from, and the command that
reads it. A value-typed slot such as `code.validation` adds the value.

To use your own Go conventions, add a project skill that provides them:

```bash
mkdir -p .henia/skills/house-go
cat > .henia/skills/house-go/SKILL.md <<'EOF'
---
name: house-go
description: Go conventions for this repository.
henia:
  provides:
    code.style.go:
    code.validation.go: {command: go test ./...}
---

# House Go

Wrap errors with `%w`. Table tests only.
EOF
```

```console
$ henia slots code.style.go code.validation.go
code.style.go	project	henia show project:house-go
code.validation.go	project	henia show project:house-go	go test ./...
```

The project's skill shadows Loqui for Go. Every other language still reads
Loqui.

## Declaring

`henia.slots` in a skill's frontmatter maps each slot to its type:

```yaml
# tropos:code
henia:
  slots:
    code.style:                            # keyed(list)
    code.validation: keyed(list(command))
```

The type decides how the providers that survive [priorities](#providing)
combine:

| Type | Keeps | Nix |
|---|---|---|
| `list` | every provider | `types.listOf` |
| `unique` | one; several produce a `conflict` row | `types.uniq` |
| `keyed(list)` | every provider of each dotted sub-slot, such as `code.style.python` | `types.attrsOf (types.listOf …)` |
| `keyed(unique)` | one provider per sub-slot | `types.attrsOf (types.uniq …)` |
| `list(command)`, … | values in place of skills | `types.listOf types.str`, … |

An untyped slot is `keyed(list)`. A type can also name what providers contribute:
`list(command)`, `unique(text)` or `keyed(list(path))`. Without that, providers
contribute skills, which the consumer reads with `henia show`. A `command` runs
as written, a `text` is used verbatim, and a `path` resolves against the
providing skill's directory and must exist.

A sub-slot of a slot that is not keyed is unknown. Two declarations of one slot
with different types conflict.

## Providing

`henia.provides` maps each slot a skill fills to an offer. The offer may be empty:

```yaml
# loqui:loqui
henia:
  provides:
    code.style.go: {section: go}
    code.validation.go: {command: go vet ./... && go build ./...}
```

| Key | Meaning |
|---|---|
| `section` | offer one section, read as `henia show <package>:<skill>#<section>` |
| `command`, `text`, `path` | the value for a slot of that value type |
| `priority` | `force`, `normal` or `fallback`, overriding the one the tier implies |

A provider of a sub-slot covers only that part. A provider of a slot also covers
its sub-slots. The provider is `invalid` when its offer misses the value a
value-typed slot needs, gives a value to a skill slot, or names a path that does
not exist.

Priorities filter providers before the type combines them. Each tier implies
one:

| Priority | Rank | Implied for | Nix |
|---|---|---|---|
| `force` | 50 | none | `lib.mkForce` |
| `normal` | 100 | the project's skills | a plain definition |
| `dependency` | 500 | the project's skill packages | |
| `fallback` | 1000 | global packages | `lib.mkDefault` |

A provider shadows every worse-ranked provider of its slot and of the slot's
sub-slots. Providers of equal rank combine by the slot's type. So the project
beats its packages, and its packages beat the global ones. A project provider at
`fallback` sits beside the global fallbacks and replaces none of them.

## Applying

A skill uses its slots with a `:slot[…]` directive in its body:

```md
Language guidance and checks:

:slot[code.style code.validation]
```

When the skill renders, the directive becomes a preload of
`henia slots code.style code.validation`. `henia show` runs it and prints the
providers in place:

````console
$ henia show code#runtime-context
## Runtime Context
…
Slot providers:


```text
$ henia slots code.style code.validation test.conventions review.criteria
code.style.go	project	henia show project:house-go
code.validation.go	project	henia show project:house-go	go test ./...
code.style.bash	dependency	henia show loqui:loqui
…
test.conventions	none	-
review.criteria	none	-
```
````

`henia build` projects the directive into the harness's preload syntax. A
built skill therefore resolves its providers in whichever project it runs.

## Configuring

A project or user `henia.toml` turns providers off for a slot and its
sub-slots. Each entry is a `<package>:<skill>` reference:

```toml
[slots."code.style.python"]
disable = ["loqui:loqui"]
```

```console
$ henia slots code.style.python
code.style.python	none	-
```

## Resolving

```
henia slots [--project ROOT] [--package DIR]... <slot>...
henia slots --check
henia slots --explain [<slot>...]
henia slots --json [<slot>...]
```

Henia prints one row, `<slot>\t<tier>\thenia show <package>:<skill>`, for each
provider of a requested slot, of its sub-slots or of an enclosing slot. Project
providers come first, and value-typed slots add a fourth `\t<value>` column.
Problem rows follow:

| Tier | Meaning | Path |
|---|---|---|
| `none` | a requested slot that is declared but has no providers | `-` |
| `invalid` | a malformed declaration or offer | the declaring or providing skill |
| `unknown` | a provided slot no declaration covers | the providing skill |
| `undeclared` | a requested slot without an owner | `-` |
| `undeclared` | an applied slot without an owner | the applying skill |
| `conflict` | a slot declared with different types | each declaring skill |
| `conflict` | a `unique` slot or key with several providers | `-` |

A `none` row is informational. If no skill declares any slot, every slot is
known. `--check` prints the problem rows of every provider and exits 1 when
there are any. Lookups always exit 0.

`--explain` shows where each provider and each override comes from:

```console
$ henia slots --explain code.style.go
code.style.go
  code.style (keyed(list), declared by …/tropos/skills/code/SKILL.md)
    applied by code, continue, implement, loop
    no providers
  code.style.go (keyed(list) of code.style, declared by …/tropos/skills/code/SKILL.md)
    selected house-go [project, normal from tier] …/.henia/skills/house-go/SKILL.md
    shadowed loqui [dependency, dependency from tier] …/loqui/skills/loqui/SKILL.md
      by house-go [project, normal from tier] for code.style.go, …/.henia/skills/house-go/SKILL.md
```

Each provider line shows the provider's status, then its tier and priority, and
whether that priority comes from the tier or from the offer. The status is one of
`selected`, `shadowed`, `disabled`, `unknown` or `invalid`. A shadowed provider
names the provider that shadows it and the slot that provider fills. Each slot
names the declaration that types it and the skills that apply it. With no slots
given, `--explain` covers every slot.

`--json` emits the same data. That covers `declarations`, `providers` (with
`ref`, `status`, `priority`, `explicit` and `shadowed_by`), `consumers` and
`problems`.

## Lineage

Slots follow the [Nix module system](https://nixos.org/manual/nixos/stable/#sec-writing-modules).
In Nix terms, a declaration is an option and a provider is a definition. The type
carries the merge, and
[priorities](https://nixos.org/manual/nixos/stable/#sec-option-definitions-setting-priorities)
filter definitions before they merge. The ranks are Nix's override priorities.
`fallback` is Nix's `mkDefault`, renamed because here it reads as the implied
priority.

Slots differ from Nix in three ways:

- A definition is a skill reference where Nix has a value.
- A definition of a slot also competes for its dotted sub-slots.
- Resolution runs over the library each time a skill is read.

## Linting

`invalid-slot` reports malformed declarations, offers and directives, and
conflicting declarations. `unknown-slot` reports a provided or applied slot that
nothing declares, neither a scanned skill nor one of the project's skill
packages.

## Acceptance tests

`mise run test:acceptance` uses Phora to stage [the fixture](../tests/acceptance/slots):

- a pinned Tropos, built by the Henia under test;
- an isolated library holding Tropos and a third-party global package;
- a project with its own providers.

Claude Code and Codex then run Tropos's `code` skill in isolated homes. Each
reports the canaries of the providers it read. To use a local Tropos working
tree in place of the pin, set `TROPOS=<checkout>`. The Claude Code document
needs `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`. A document is skipped
when its harness or credentials are missing.
