# Vendor profiles and metadata expressions

Enable a profile explicitly in a project configuration:

```toml
[harness.claude]
profile = "claude"
path = ".claude"
format = "xml"
strict = true
```

Canonical metadata follows Agent Skills (`name`, `description`, `license`,
`compatibility`, string-valued `metadata`) and adds `model_tier`, `tools`,
`tools_policy`, `enabled` and `user_invocable` for transforms. Unknown author fields
remain available to templates. A profile diagnoses and omits fields it cannot
represent. `strict = true` turns these warnings into build failures. Canonical
type errors and malformed target metadata always fail compilation.

`henia.variables` holds template-only data. `henia.auto_invoke` and
`henia.user_invocable` express portable invocation preferences where supported.
Profiles without corresponding behavior issue a diagnostic. Top-level
`auto_invoke`/`user_invocable` are also accepted. A target-specific value wins;
setting it to YAML `null` suppresses an inherited preference for that target.

```yaml
name: review
description: Review code changes when the user requests a review.
henia:
  variables:
    priority: high
  targets:
    claude:
      auto_invoke: false
      frontmatter:
        argument-hint: "[file or PR]"
    codex:
      auto_invoke: false
      openai:
        interface:
          display_name: Code review
          short_description: Review correctness and coverage
          default_prompt: "Use $review to review these changes."
```

This emits `disable-model-invocation: true` for Claude and
`policy.allow_implicit_invocation: false` in Codex's `agents/openai.yaml`.
OpenAI native policy values override computed defaults. Native frontmatter
overrides apply last; YAML `null` removes a field. These fields never leak into
another target's output. Sidecars are generated alongside the skill resources.

## Customizing profiles

Profile definitions merge from:

1. Bundled `internal/vendor/profiles/<profile>.toml`.
2. `~/.config/henia/harnesses/<profile>/transform.toml`.
3. `<project>/.henia/harnesses/<profile>/transform.toml`.

The project root is the directory containing the selected `henia.toml`.
Tables merge recursively, arrays concatenate, and later scalar values override.
Use a new profile name for a completely different schema. For example:

```toml
# .henia/harnesses/my-agent/transform.toml
fields = ["name", "description", "automatic"]
controls = ["auto_invoke"]

[computed]
automatic = 'settings.auto_invoke'

[files.manifest]
path = "manifests/{name}.json"
format = "json"
value = '{name: input.name, description: input.description}'
```

```toml
# henia.toml
[harness.my-agent]
profile = "my-agent"
path = ".my-agent"
format = "directives"
```

No runtime script or Go change is needed. Profile syntax is TOML; computed values
use [Expr](https://expr-lang.org/docs/language-definition).

## Profile contract

| Field | Purpose |
|---|---|
| `fields` | Output frontmatter allowlist; must include required skill metadata |
| `aliases` | Simple input key renames; conflicting aliases fail |
| `computed` | Output key → Expr expression |
| `consume` | Input keys consumed by computations and omitted from output |
| `controls` | Invocation preferences represented by this profile |
| `files` | Named file declarations with `path`, `format`, and `value` |

Computed expressions receive `input` (templated canonical metadata), `settings`
(merged Henia preferences), `native` (selected target overrides), and `ctx`:
`name`, `profile`, `path`, `variables`, `tools`, and `keys`.

Helpers:

- `toolNames(value, mapping)`: map a list of tool names, then join with spaces.
  A scalar string is preserved as one vendor expression, or mapped as one exact
  name. Use YAML lists to map several canonical tool names.
- `merge(base, override)`: recursively merge maps using config precedence.
- `require(value, message)`: fail when a needed value is nil or an empty string.

Computations all read the same immutable canonical input, not each other's
results. A `nil` computed result adds no field. Existing field values remain unless
explicitly removed by a native override. Legacy TOML key/value mappings run before
the profile; native output overrides run after it. Output types are preserved.

File `value` expressions produce data for `yaml`/`json`, or a string for `text`.
`nil` and empty maps suppress a file. Paths are relative to the harness root;
`{name}` expands to the skill directory name. Traversal and collisions with main
files, resources or other emitted files fail preflight. Shared harness-level files
are not implicitly merged across skills: competing writers cause an error.

These profiles apply to skills. Harness-wide permissions, command conversion,
plugin publication and upload orchestration are separate concerns. In particular,
OpenCode skill frontmatter must not be mistaken for OpenCode agent permissions.
The [research matrix](vendor-research.md) links the vendor contracts.
