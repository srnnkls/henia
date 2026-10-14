# Vendor profiles

A *profile* holds what Henia knows about one harness. It maps canonical skill
metadata onto the harness's frontmatter and extra files, and renders the
harness's own configuration files. Write the skill once, and each profile emits
the shape its harness expects.

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

A harness that a skill package configures resolves its profiles in that
package instead of the project, so a project build and `henia install` find a
package's custom profiles in the package's own `.henia/harnesses/`.

Tables merge recursively. Arrays concatenate, and later scalars override, so a
layer can add to `fields` but never remove from it. A completely different
schema takes a new profile name:

```toml
# .henia/harnesses/my-agent/transform.toml
directives = "keep"

[skill]
fields = ["name", "description", "automatic"]
controls = ["auto_invoke"]

[skill.computed]
automatic = 'settings.auto_invoke'

[skill.files.manifest]
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

A profile has harness-wide keys at the top level and one table per artifact
type.

| Key | Purpose |
|---|---|
| `directives` | default directive rendering (`xml`, `markdown`, `md` or `keep`); `[harness.<name>] directives` overrides it |
| `commands`, `agents` | the harness's built-in slash commands and subagents |
| `[skill]` | how skills render |
| `[agent]` | how agents render; without it, agents keep their canonical frontmatter |
| `[command]` | reserved; commands do not render through profiles, and a nonempty table fails |
| `[config]` | the harness's [configuration files](#harness-configuration) |

`[skill]` and `[agent]` take the same keys:

| Key | Purpose |
|---|---|
| `fields` | output frontmatter allowlist; must include the required skill metadata |
| `aliases` | input key renames; conflicting aliases fail |
| `computed` | output key → Expr expression |
| `consume` | input keys that computations read; omitted from the output |
| `controls` | invocation preferences the profile represents |
| `files` | named files, each with `path`, `format` and `value` |
| `preloads` | the harness runs `` !`cmd` `` preloads natively; others get a run-first block |

The flat form, with the `[skill]` keys at the top level, still reads as `[skill]`.
Each layer converts on its own, so a flat user `transform.toml` merges over a
sectioned bundled profile. One file that mixes top-level skill keys with
`[skill]` fails.

`[harness.<name>.agents] profile = "..."` selects another profile's `[agent]`
table for agents. A per-type profile in the flat form serves agents through its
`[skill]` keys.

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

Command conversion, plugin publication and uploads are separate concerns.
OpenCode skill frontmatter, for one, is not where OpenCode agent permissions go.
The [research matrix](vendor-research.md) links each vendor's contract.

## Harness configuration

`[config.files.<id>]` declares the configuration files a harness reads: MCP
servers, models, permissions and the rest. A build renders them when the
harness's `artifacts` include `config`:

```toml
# henia.toml
artifacts = ["skills", "config"]

[shared.mcp.github]
command = "gh-mcp"
args = ["serve"]
env = { GITHUB_TOKEN = "$GITHUB_TOKEN" }

[harness.claude]
[harness.claude.config.native]
permissions = { allow = ["Read"] }

[harness.codex]
[harness.codex.config.native]
approval_policy = "on-request"
```

```console
$ henia build . --clean
Built 2 artifact(s) in .henia/build

$ cat .henia/build/codex/config.toml
#:schema https://developers.openai.com/codex/config-schema.json

approval_policy = 'on-request'

[mcp_servers]
[mcp_servers.github]
args = ['serve']
command = 'gh-mcp'
env_vars = ['GITHUB_TOKEN']

$ cat .henia/build/claude/.mcp.json
{
  "mcpServers": {
    "github": {
      "args": [
        "serve"
      ],
      "command": "gh-mcp",
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

Config files land in the harness's build tree next to its skills. `henia
install` does not install them.

### Input

`[shared]` holds free-form tables that every harness profile may read.
`[harness.<name>.config.native]` holds data that
one harness takes as is. A native key naming a config file id addresses that
file; the remaining keys address the file with id `main`. In henia.toml layers,
arrays under `artifacts`, `[shared]` and `[harness.<name>.config]` replace
earlier ones.

Secrets stay out of `[shared]`: `"$GITHUB_TOKEN"` names an environment
variable, and each profile maps the reference to its harness's spelling. Henia
never reads the variable.

The bundled profiles read `[shared.mcp.<server>]` with `command`, `args`, `env`
and `url`. An `env` value starting with `$` references a variable; any other
value is literal.

| Harness | Files | Mapping |
|---|---|---|
| claude | `settings.json` (id `main`), `.mcp.json` (id `mcp`) | references become `${NAME}`; a `url` server gets `type = "http"` |
| codex | `config.toml` (id `main`) | references become `env_vars`, literals `env`; a reference must name its own variable |

### Claude Code status line

`[harness.claude.config.statusline]` sets Claude Code's `statusLine` from
exactly one of:

- `command`: a command line or path, used as is;
- `script`: an inline shell script;
- `file`: a script file.

A `script` or `file` deploys as the executable `statusline.sh` in the harness
tree, with a `#!/bin/sh` line when it has none, and `statusLine.command` runs
`"${CLAUDE_CONFIG_DIR:-$HOME/.claude}/statusline.sh"`. A relative `file`
resolves against the directory of the henia.toml that sets it, `~/` against
the home directory; building config fails when it is missing. `padding`,
`refresh_interval` and `hide_vim_mode` map to `padding`, `refreshInterval` and
`hideVimModeIndicator`. A later layer's `statusline` replaces an earlier one
whole.

```toml
[harness.claude.config.statusline]
script = """
input=$(cat)
printf '%s' "$(echo "$input" | jq -r .model.display_name)"
"""
refresh_interval = 5
```

Config expressions read this table as `harness.statusline`. A harness whose
profile has no status line warns and omits it. Codex's status line is set
natively: `[harness.codex.config.native.tui] status_line = ["model-with-reasoning"]`.

### File contract

| Key | Purpose |
|---|---|
| `path` | output path relative to the harness root |
| `format` | `json`, `yaml`, `toml` or `text` |
| `value` | Expr expression for the generated data; optional when native data fills the file |
| `schema` | JSON Schema URL the result is validated against |
| `arrays` | how native arrays combine with generated ones: `replace` (default) or `concat` |
| `mode` | octal file permissions, `0644` by default |

`value` sees `shared`, `harness` (the harness's typed config inputs, such as
`statusline`), `ctx` and the `merge` and `require` helpers, but no skill input.
Native data merges over the generated value last. Nil table entries and array
items drop out, and a file whose result is empty is not written. Output keys
are sorted. With a `schema`, `json` gains a `$schema`
key, `toml` a `#:schema` comment and `yaml` a `yaml-language-server` comment.

Schema violations are warnings, and errors under `strict = true`. Bundled
profiles validate against pinned schemas without network access. Henia fetches
any other schema once into the user cache directory (`henia/schemas`); a schema
it cannot load is a warning.

A custom harness needs no Go code:

```toml
# .henia/harnesses/my-agent/transform.toml
[config.files.main]
path = "agent.yml"
format = "yaml"
value = '{providers: shared.providers, models: shared.models}'
```

## Built-in commands and agents

A profile lists the slash commands and subagents its harness ships with:

```toml
commands = ["clear", "compact", "plan"]
agents = ["Explore", "Plan", "general-purpose"]
```

`henia lint` resolves `/command` and `@agent` references against these lists for
every configured harness, so `/compact` in a skill is not a broken reference.
