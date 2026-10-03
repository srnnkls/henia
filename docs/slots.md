# Slots

A skill declares named extension points, *slots*, and other skills fill them.
Installed skills stay open for extension: any skill in a harness's skills
directory or a project's skill directories may declare or provide a slot, so
`henia slots` resolves them when the skill runs, not when Henia builds it. Both
live in the spec's `metadata` map as strings.

## Declaring

`metadata.slots` lists `<slot>[:<type>]` entries, space-separated or as a list.
The type composes the providers that survive priority filtering:

| Type | Providers | Nix |
|---|---|---|
| `list` | every surviving provider | `types.listOf` |
| `unique` | one; several produce a `conflict` row | `types.uniq` |
| `keyed(list)` | every surviving provider of each dotted sub-slot, such as `code.style.python` | `types.attrsOf (types.listOf …)` |
| `keyed(unique)` | one per dotted sub-slot | `types.attrsOf (types.uniq …)` |
| `list(command)`, … | values instead of skills | `types.listOf types.str`, … |

A type may name what providers contribute: `list(command)`, `unique(text)`,
`keyed(list(path))`. Without one they contribute `skill`s, whose SKILL.md the
consumer reads; a `command` is run as is, a `text` used as is, and a `path`
resolves against the providing skill's directory and must exist. A provider
sets the value under the name of the slot it provides:

```yaml
metadata:
  provides: "code.check.go"
  code.check.go: "go vet ./... && go test ./..."
```

A missing value, a value for a `skill` slot or a missing path makes the
provider `invalid`. An untyped slot is `keyed(list)`. A sub-slot of a slot that is not keyed is
unknown. Two declarations of one slot with different types conflict.

```yaml
metadata:
  slots: "code.style:keyed(list) review.criteria:unique"
```

## Providing

`metadata.provides` lists `<slot>[@<priority>]` entries. A provider of a sub-slot
covers only that part; a provider of a slot also covers its sub-slots.

| Priority | Rank | Implied for | Nix |
|---|---|---|---|
| `force` | 50 | none | `lib.mkForce` |
| `normal` | 100 | project skills | a plain definition |
| `fallback` | 1000 | global skills | `lib.mkDefault` |

A provider shadows every provider of its slot, and of the slot's sub-slots, with
a worse (higher) rank; providers of equal rank compose by the slot's type. A
project provider at `fallback` sits beside the global fallbacks rather than
replacing them.

```yaml
metadata:
  provides: "code.style.python review.criteria@fallback"
```

## Applying

`metadata.applies` lists the slots a skill preloads providers for, so owners,
providers and consumers form one graph. A skill preloads its providers with
`henia slots --for <skill>`, which resolves the slots that skill applies; the
command names no slot itself.

```yaml
metadata:
  applies: "code.style code.validation review.criteria"
```

Applying a slot that no declaration covers is an `undeclared` problem naming the
applying skill.

## Lineage

Slots follow the [Nix module system](https://nixos.org/manual/nixos/stable/#sec-writing-modules):
a declaration is an option, a provider is a definition, a type carries the
merge, and [priorities](https://nixos.org/manual/nixos/stable/#sec-option-definitions-setting-priorities)
filter definitions before they merge. The ranks are Nix's override priorities;
`fallback` renames `mkDefault`, whose name reads as the implied priority here.
Slots differ in three ways: definitions are skill references rather than values,
a definition of a slot also competes for its dotted sub-slots, and resolution
runs over whatever is installed when a skill runs.

## Resolving

```
henia slots [--global DIR]... [--project ROOT] [--for SKILL]... <slot>...
henia slots [--global DIR]... --check
```

Global skills are `DIR/<skill>/SKILL.md` for each `--global`. Project skills are
`<skill>/SKILL.md` under `.claude/skills`, `.agents/skills`, `.agent/skills`,
`.codex/skills`, `.pi/skills`, `.omp/skills` and `.github/skills` of the project
root: `--project`, else the Git top level, else the working directory. A skill
reached twice, such as a symlinked global skill inside a project, keeps its
first tier.

Each output line is `<slot>\t<tier>\t<SKILL.md>`, with a fourth `\t<value>`
for value-typed slots, for a provider of a requested slot, a sub-slot of it or an enclosing slot, project providers first. Problem
rows follow:

| Tier | Meaning | Path |
|---|---|---|
| `invalid` | malformed entry | declaring or providing skill |
| `unknown` | provided slot that no declaration covers | providing skill |
| `undeclared` | requested slot without an owner | `-` |
| `undeclared` | applied slot without an owner | applying skill |
| `conflict` | slot declared with different types | each declaring skill |
| `conflict` | `unique` slot or key with several providers | `-` |

Without any declaration every slot is known. `--check` prints only problem rows
of every provider and exits 1 when any exist; lookups exit 0.

## Lineage at runtime

`--explain` shows where each provider and each override comes from, per
requested slot, or for every slot without any:

```
$ henia slots --global ~/.claude/skills --explain code.style.go
code.style.go
  code.style (keyed(list), declared by ~/.claude/skills/code/SKILL.md)
    no providers
  code.style.go (keyed(list) of code.style, declared by ~/.claude/skills/code/SKILL.md)
    selected house-go [project, normal from tier] ~/repo/.agents/skills/house-go/SKILL.md
    shadowed loqui [global, fallback from tier] ~/.claude/skills/loqui/SKILL.md
      by house-go [project, normal from tier] for code.style.go, ~/repo/.agents/skills/house-go/SKILL.md
```

Each provider shows its status (`selected`, `shadowed`, `unknown` or `invalid`), its tier,
its priority and whether that priority comes from the tier or the entry. A
shadowed provider names the provider that shadows it, at the best priority, and
the slot it provides; an unknown one names why no declaration covers it. Each
slot names the declaration that types it and the skills that apply it. `--json` emits the declarations,
providers (with `status`, `priority`, `explicit` and `shadowed_by`), consumers
and problems of the requested slots, or of all.

## Harness wiring

A skill preloads its providers by running `henia slots` with its harness's
global skills directory, for example as a Henia variable per harness:

```toml
[harness.claude.variables]
resolve_slots = "henia slots --global ${CLAUDE_SKILL_DIR}/.."

[harness.codex.variables]
resolve_slots = 'henia slots --global "${CODEX_HOME:-$HOME/.codex}/skills"'
```

## Linting

The `invalid-slot` rule reports malformed entries and conflicting declarations
among the scanned documents. Whether every provided slot is declared depends on
what is installed; check it over the scanned set with a
[registry](lint-rules.md#name-registries) that strips types and priorities:

```toml
[[lint.registries]]
id = "unknown-slot"
declare = 'map(split(frontmatter.metadata?.slots ?? "", " "), split(#, ":")[0])'
reference = 'map(split((frontmatter.metadata?.provides ?? "") + " " + (frontmatter.metadata?.applies ?? ""), " "), split(#, "@")[0])'
match = "dotted"
```

## Acceptance tests

`mise run test:acceptance` stages [the fixture](../tests/acceptance/slots) with
Phora: a pinned Tropos built by the henia under test, third-party global skills
and a project. Claude Code and Codex then run Tropos's `code` skill in isolated
homes and report the canaries of the providers they read. `TROPOS=<checkout>`
links a local Tropos working tree instead of the pin. The Claude Code document
needs `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`; each document is skipped
when its harness or credentials are missing.
