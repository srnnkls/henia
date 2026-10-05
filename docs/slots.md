# Slots

A skill declares named extension points, *slots*, and other skills fill them.
Slots resolve over the [library](runtime.md): the project's own skills, its
skill packages and the global packages. Resolution happens when a skill is
read, so a project or package composes into skills it did not author, and
skills stay open for extension after Henia builds them.

```yaml
# tropos:code declares and applies slots
henia:
  slots:
    code.style:                                 # keyed(list)
    code.validation: keyed(list(command))

# tropos:loqui provides them
henia:
  provides:
    code.style.go: {section: go}
    code.validation.go: {command: go vet ./...}
```

```md
<!-- in tropos:code's body -->
Language guidance and checks:

:slot[code.style code.validation]
```

## Declaring

`henia.slots` maps each slot to its type. The type composes the providers that
survive priority filtering:

| Type | Providers | Nix |
|---|---|---|
| `list` | every surviving provider | `types.listOf` |
| `unique` | one; several produce a `conflict` row | `types.uniq` |
| `keyed(list)` | every surviving provider of each dotted sub-slot, such as `code.style.python` | `types.attrsOf (types.listOf …)` |
| `keyed(unique)` | one per dotted sub-slot | `types.attrsOf (types.uniq …)` |
| `list(command)`, … | values instead of skills | `types.listOf types.str`, … |

A slot without a type is `keyed(list)`. A type may name what providers
contribute: `list(command)`, `unique(text)`, `keyed(list(path))`. Without one
they contribute skills, which the consumer reads with `henia show`. A
`command` is run as is, a `text` used as is, and a `path` resolves against the
providing skill's directory and must exist. A sub-slot of a slot that is not
keyed is unknown, and two declarations of one slot with different types
conflict.

## Providing

`henia.provides` maps each provided slot to an offer, which may be empty:

| Key | Meaning |
|---|---|
| `section` | offer one section of the skill, read as `henia show <package>:<skill>#<section>` |
| `command`, `text`, `path` | the value for a slot of that value type |
| `priority` | `force`, `normal` or `fallback`, instead of the one the tier implies |

A provider of a sub-slot covers only that part; a provider of a slot also
covers its sub-slots. An offer without a value for a value-typed slot, a value
for a skill slot, or a missing path makes the provider `invalid`.

Priorities filter providers before the type composes them. Each tier implies
one:

| Priority | Rank | Implied for | Nix |
|---|---|---|---|
| `force` | 50 | none | `lib.mkForce` |
| `normal` | 100 | the project's skills | a plain definition |
| `dependency` | 500 | skill packages the project depends on | |
| `fallback` | 1000 | global packages | `lib.mkDefault` |

A provider shadows every provider of its slot, and of the slot's sub-slots, with
a worse (higher) rank; providers of equal rank compose by the slot's type. So
the project overrides its packages, and its packages override global ones. A
project provider at `fallback` sits beside the global fallbacks rather than
replacing them.

## Applying

A `:slot[<slot> ...]` directive in a skill's body applies slots where the skill
uses them. Rendering turns it into a preload of `henia slots <slot> ...`:
`henia show` runs it and prints the providers in place, and `henia build`
projects it into the harness's preload syntax, so a built skill resolves its
providers in whichever project it runs.

## Configuring

A project or user `henia.toml` turns providers off for a slot and its
sub-slots:

```toml
[slots."code.style.python"]
disable = ["tropos:loqui"]
```

## Resolving

```
henia slots [--project ROOT] [--package DIR]... <slot>...
henia slots --check
henia slots --explain [<slot>...]
henia slots --json [<slot>...]
```

Each output line is `<slot>\t<tier>\thenia show <package>:<skill>`, with a
fourth `\t<value>` for value-typed slots, for each provider of a requested
slot, a sub-slot of it or an enclosing slot, project providers first. Problem
rows follow:

| Tier | Meaning | Path |
|---|---|---|
| `invalid` | malformed declaration or offer | declaring or providing skill |
| `unknown` | provided slot that no declaration covers | providing skill |
| `undeclared` | requested slot without an owner | `-` |
| `undeclared` | applied slot without an owner | applying skill |
| `conflict` | slot declared with different types | each declaring skill |
| `conflict` | `unique` slot or key with several providers | `-` |

Without any declaration every slot is known. `--check` prints only problem rows
of every provider and exits 1 when any exist; lookups exit 0.

`--explain` shows where each provider and each override comes from, per
requested slot, or for every slot without any:

```
$ henia slots --explain code.style.go
code.style.go
  code.style (keyed(list), declared by ~/.local/share/henia/packages/global/tropos/skills/code/SKILL.md)
    no providers
  code.style.go (keyed(list) of code.style, declared by …/tropos/skills/code/SKILL.md)
    selected house-go [project, normal from tier] ~/repo/.henia/skills/house-go/SKILL.md
    shadowed loqui [global, fallback from tier] …/tropos/skills/loqui/SKILL.md
      by house-go [project, normal from tier] for code.style.go, ~/repo/.henia/skills/house-go/SKILL.md
```

Each provider shows its status (`selected`, `shadowed`, `disabled`, `unknown` or
`invalid`), its tier, its priority and whether that priority comes from the
tier or the offer. A shadowed provider names the provider that shadows it, at
the best priority, and the slot it provides. Each slot names the declaration
that types it and the skills that apply it. `--json` emits the declarations,
providers (with `ref`, `status`, `priority`, `explicit` and `shadowed_by`),
consumers and problems of the requested slots, or of all.

## Lineage

Slots follow the [Nix module system](https://nixos.org/manual/nixos/stable/#sec-writing-modules):
a declaration is an option, a provider is a definition, a type carries the
merge, and [priorities](https://nixos.org/manual/nixos/stable/#sec-option-definitions-setting-priorities)
filter definitions before they merge. The ranks are Nix's override priorities;
`fallback` renames `mkDefault`, whose name reads as the implied priority here.
Slots differ in three ways: definitions are skill references rather than values,
a definition of a slot also competes for its dotted sub-slots, and resolution
runs over the library when a skill is read.

## Linting

The `invalid-slot` rule reports malformed declarations, offers and directives
and conflicting declarations, and `unknown-slot` reports a provided or applied
slot that neither a scanned skill nor one of the project's skill packages
declares.

## Acceptance tests

`mise run test:acceptance` stages [the fixture](../tests/acceptance/slots) with
Phora: a pinned Tropos built by the henia under test, an isolated Henia
library holding Tropos and a third-party global package, and a project with its
own providers. Claude Code and Codex then run Tropos's `code` skill in isolated
homes and report the canaries of the providers they read. `TROPOS=<checkout>`
links a local Tropos working tree instead of the pin. The Claude Code document
needs `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`; each document is skipped
when its harness or credentials are missing.
