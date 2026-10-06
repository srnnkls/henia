# Vendor profiles

A *profile* maps canonical skill metadata onto one harness's frontmatter, along
with any extra files that harness reads. Write the skill once, and each profile
emits the shape its harness expects.

```bash
mkdir -p skills/review
cat > skills/review/SKILL.md <<'EOF'
---
name: review
description: Review code changes when the user asks for a review.
tools: [Read, Grep]
henia:
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
          default_prompt: "Use $review to review these changes."
---

# Review

Check correctness first, then coverage.
EOF
cat > henia.toml <<'EOF'
[harness.claude]
profile = "claude"

[harness.codex]
profile = "codex"
EOF
```

```console
$ henia build .
Warning: skills/review (codex): unsupported frontmatter tools; omitted
Built 2 artifact(s) in .henia/build

$ cat .henia/build/claude/skills/review/SKILL.md
---
allowed-tools: Read Grep
argument-hint: '[file or PR]'
description: Review code changes when the user asks for a review.
disable-model-invocation: true
name: review
---

# Review

Check correctness first, then coverage.

$ cat .henia/build/codex/skills/review/agents/openai.yaml
interface:
    default_prompt: Use $review to review these changes.
    display_name: Code review
policy:
    allow_implicit_invocation: false
```

`auto_invoke: false` becomes `disable-model-invocation: true` for Claude Code and
`policy.allow_implicit_invocation: false` in Codex's `agents/openai.yaml`.
Codex skill frontmatter has no tool list, so the Codex profile drops `tools` and
says so. Add `strict = true` to the harness and the warning fails the build:

```console
$ henia build .
Error: skills/review: transform review for codex: unsupported skill metadata: unsupported frontmatter tools; omitted
```

## Canonical metadata

Canonical metadata follows [Agent Skills](https://agentskills.io/specification):
`name`, `description`, `license`, `compatibility` and string-valued `metadata`.
Henia adds `model_tier`, `tools`, `tools_policy` and `enabled` for profiles to
map. Templates can still read any other author field. A profile omits each field
it cannot represent and prints a diagnostic. Type errors in canonical metadata
and malformed target metadata always fail the build.

Template-only data goes in `henia.variables`. Two keys state portable invocation
preferences:

- `henia.auto_invoke`: whether the model may invoke the skill on its own;
- `henia.user_invocable`: whether the user may invoke it.

A profile without the matching behavior prints a diagnostic. A value under
`henia.targets.<harness>` wins over the shared one. Setting it to YAML `null`
drops an inherited preference for that target.

A target's `frontmatter` and `openai` tables are native overrides:

- Native OpenAI policy values override the computed defaults.
- Native frontmatter applies last, and YAML `null` removes a field.

Neither leaks into another target's output. Sidecar files land next to the
skill's resources.

## Customizing profiles

Profiles merge from three places, later ones over earlier:

1. the bundled `internal/profile/profiles/<profile>.toml`;
2. `~/.config/henia/harnesses/<profile>/transform.toml`;
3. `<project>/.henia/harnesses/<profile>/transform.toml`, where the project is
   the directory that holds the selected `henia.toml`.

Tables merge recursively. Arrays concatenate, and later scalars override. A
completely different schema takes a new profile name:

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
```

With `henia.targets.my-agent.auto_invoke: false` in the skill:

```console
$ henia build . --clean
Warning: skills/review (my-agent): unsupported frontmatter tools; omitted
Built 1 artifact(s) in .henia/build

$ cat .henia/build/my-agent/manifests/review.json
{
  "description": "Review code changes when the user asks for a review.",
  "name": "review"
}

$ head -5 .henia/build/my-agent/skills/review/SKILL.md
---
automatic: false
description: Review code changes when the user asks for a review.
name: review
---
```

This needs no script and no Go code. Profiles are TOML; computed values are
[Expr](https://expr-lang.org/docs/language-definition) expressions.

## Profile contract

| Field | Purpose |
|---|---|
| `fields` | output frontmatter allowlist; must include the required skill metadata |
| `aliases` | input key renames; conflicting aliases fail |
| `computed` | output key → Expr expression |
| `consume` | input keys that computations read; omitted from the output |
| `controls` | invocation preferences the profile represents |
| `files` | named files, each with `path`, `format` and `value` |
| `preloads` | the harness runs `` !`cmd` `` preloads natively; others get a run-first block |
| `commands`, `agents` | the harness's built-in slash commands and subagents |

Computed expressions see four inputs:

- `input`: the templated canonical metadata;
- `settings`: the merged Henia preferences;
- `native`: the selected target's overrides;
- `ctx`: `name`, `profile`, `path`, `variables`, `tools` and `keys`.

`ctx.path` is the harness's build output directory and never an installation
destination.

Helpers:

- `toolNames(value, mapping)` maps a list of tool names and joins them with
  spaces. A scalar string passes through as one vendor expression, or maps as a
  single exact name, so list several canonical tools as a YAML list.
- `merge(base, override)` merges maps recursively, with config precedence.
- `require(value, message)` fails when a needed value is nil or an empty string.

Every computation reads the same immutable canonical input; none sees another's
result. A `nil` result adds no field. Existing field values stay unless a native
override removes them. Output types are preserved. The harness's
`[harness.<name>.frontmatter]` renames and value maps run before the profile, and
native overrides run after it.

A file's `value` produces data for `yaml` and `json`, or a string for `text`.
`nil` or an empty map produces no file. Paths are relative to the harness's build
output root, and `{name}` expands to the skill's directory name. A path that
escapes the root or collides with a main file, a resource or another emitted file
fails before anything is written. Henia does not merge harness-level files across
skills; two skills writing one path is an error.

Profiles cover skills. Harness-wide permissions, command conversion, plugin
publication and uploads are separate concerns. OpenCode skill frontmatter, for
one, is not where OpenCode agent permissions go. The
[research matrix](vendor-research.md) links each vendor's contract.

## Built-in commands and agents

A profile lists the slash commands and subagents its harness ships with:

```toml
commands = ["clear", "compact", "plan"]
agents = ["Explore", "Plan", "general-purpose"]
```

`henia lint` resolves `/command` and `@agent` references against these lists for
every configured harness, so `/compact` in a skill is not a broken reference.
