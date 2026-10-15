---
issue_type: Feature
created: 2026-10-14
status: Accepted
stage: draft
---

# Unified harness profiles and the `config` artifact

## Goal

One `transform.toml` per harness holds everything Henia knows about that
harness: harness-wide defaults and one section per artifact type. A new `config`
artifact renders harness configuration files (MCP servers, models, permissions)
from a shared section plus per-harness native data, validated against the
harness's JSON Schema where one exists. Install of config is out of scope.

User documentation: [vendor profiles](../../../docs/vendor-profiles.md#harness-configuration)
and [build](../../../docs/build.md#harnesses).

## Decisions

### Profile layout

- Top level: `directives`, `commands`, `agents`. Sections: `[skill]`,
  `[agent]`, `[command]`, `[config]`.
- The flat form (skill keys at the top level) converts to `[skill]` per layer,
  before the three layers merge. A file mixing both forms fails.
- `[command]` is reserved and must stay empty. A per-type `commands.profile`
  selects native preloads and `allowed-tools` through that profile's `[skill]`.
- Agents use the harness profile's `[agent]`. A per-type
  `[harness.<n>.agents] profile` without `[agent]` maps agents through its
  `[skill]`.
- A harness profile without `[skill]` renders skills without a mapping.
- Every profile a harness names, per-type ones included, loads and compiles at
  config load. Skill profiles require the nested layout.

### Directives

Profiles carry a `directives` default: `xml` for claude and claude-upload,
`keep` elsewhere. Precedence, lowest first: profile, bundled preset,
`[harness.<n>] directives`. Config load and `pipeline.Transformer` both apply
the default.

### henia.toml

- `[shared]`: free-form tables, read by every harness's profile.
- `[harness.<n>.config.native]`: native data. A key naming a config file id
  addresses that file; the remaining keys address file `main` and fail when the
  profile has no `main`.
- `[harness.<n>.config.statusline]`: see [status line](#status-line).
- Arrays under `artifacts` (top level and per harness), `[shared]` and
  `[harness.<n>.config]` replace earlier layers.
- `artifacts` accepts `config`. The empty-list default (all document types)
  excludes it.

### Config rendering

- `[config.files.<id>]`: `path`, `format` (`json`, `yaml`, `toml`, `text`),
  optional `value`, `schema`, `arrays` (`replace` default, or `concat`),
  `mode` (octal, `0644` default), `root`, `merge` and `entries` (see
  [install](#install)).
- `ctx.install` is `project`, `global` or empty for a plain build, and
  `ctx.install_dir` the harness directory relative to the project root.
- `value` sees `shared`, `harness` (this harness's typed inputs), `ctx`
  (`name`, `profile`, `path`, `variables`, `tools`, `keys`), `merge` and
  `require`. Native data merges over the result once.
- Nil table entries and array items drop out; an empty result writes no file.
- Keys are sorted in every format. A `schema` adds `$schema` (json),
  `#:schema` (toml) or a `yaml-language-server` comment (yaml).
- Output is a per-harness artifact of type `config` under `<output>/<harness>/`,
  subject to the build's collision, escape and clean-build handling.

### Schemas

- Validation uses `santhosh-tekuri/jsonschema/v6` against the schemas
  SchemaStore lists: `www.schemastore.org/claude-code-settings.json` and
  `developers.openai.com/codex/config-schema.json`.
- Schemas are fetched into `os.UserCacheDir()/henia/schemas`, one file per URL
  digest, and fetched again after 24 hours. A failed fetch falls back to the
  expired copy, then to the copy pinned in `internal/profile/schemas/`.
- Violations and unavailable schemas are warnings, errors under `strict`.

### Bundled config

| Harness | Files | Inputs |
|---|---|---|
| claude | `settings.json` (`main`, schema), `.mcp.json` (`mcp`), `statusline.sh` (`statusline`, `0755`) | `shared.mcp.<server>`: `command`, `args`, `env` (`$NAME` → `${NAME}`, literals as is), `url` → `type = "http"`, `headers` (`$NAME` → `${NAME}`); `harness.statusline` → `statusLine` |
| codex | `config.toml` (`main`, schema) | `shared.mcp.<server>`: `command`, `args`, `url`, `env` (`$NAME` → `env_vars`, literals → `env`), `headers` (literals → `http_headers`, `$NAME` → `env_http_headers`, `Bearer $NAME` → `bearer_token_env_var`); a reference must name its own variable |
| pi | `settings.json` (`main`, schema), `mcp.json` (`mcp`) | `shared.mcp.<server>`: as claude, without `type` |
| omp | `config.yml` (`main`), `mcp.json` (`mcp`, schema) | `shared.mcp.<server>`: `$NAME` → `NAME`, which omp resolves; `Bearer $NAME` → `!printf 'Bearer %s' "$NAME"`; `type` is `http` or `stdio` |

A `headers` value is a literal, a whole `$NAME`, or `Bearer $NAME` on
`Authorization`: the forms every bundled harness can carry. Headers need a
`url` server.

### Status line

`[harness.claude.config.statusline]` takes exactly one of `command` (used as
is), `script` (inline, run by the shell) or `file`, plus `padding`,
`refresh_interval` and `hide_vim_mode`.

- A `script` or `file` deploys as `statusline.sh` in the harness tree, with a
  `#!/bin/sh` line when it has none; outside project installs
  `statusLine.command` runs `"${CLAUDE_CONFIG_DIR:-$HOME/.claude}/statusline.sh"`.
- A `file` resolves against the root of the layer that sets it. Building config
  fails when it is missing.
- A later layer's `statusline` replaces the earlier one whole.
- A harness whose profile reads no harness inputs warns that its `statusline`
  is omitted.

### Install

Install writes config with the rest of the harness tree, project and global.
Every destination of a harness is checked before any is written; a conflict
anywhere fails the install, and `--force` overrides.

- A config file's `root` places it: `harness` (the install target), `project`
  (the project root, project installs only) or `home` (the home directory,
  global installs only). A build for one scope skips files of the other.
- Whole files (`merge` unset) follow the install manifest: a destination Henia
  did not write, or that changed since, conflicts.
- Merged files (`merge = true`, json, toml or yaml) are owned unit by unit. A unit is
  a top-level value, a member of a table listed in `entries`, or an element of
  an array listed in `entries`. The manifest `<harness>.units.json` records a
  hash per unit and destination.
  - A missing unit is written; one whose hash matches the record is
    overwritten, or removed when no longer rendered; one already holding the
    rendered value is adopted; anything else conflicts.
  - Values Henia does not own are never compared or changed. Numbers keep
    their exact value.
  - A TOML or YAML destination with comments conflicts, since rewriting drops
    them.
  - An unchanged destination is not rewritten. A destination that changes
    between read and write fails the install.
  - The merged result is validated against the file's schema; a violation
    warns, and fails under `strict`.
- Disabling a harness removes its whole files and its units.

| File | Root | Ownership |
|---|---|---|
| claude `settings.json` | harness | units; `permissions.allow`, `deny`, `ask` by element |
| claude `.mcp.json` | project | whole file |
| claude `.claude.json` | home | units; `mcpServers` by server |
| claude `statusline.sh` | harness | whole file |
| codex `config.toml` | harness | units; `mcp_servers` by server |
| pi `settings.json` | harness | units |
| pi, omp `mcp.json` | harness | units; `mcpServers` by server |
| omp `config.yml` | harness | units |

In a project install, `statusLine.command` runs
`"${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}/<dir>/statusline.sh"`,
where `<dir>` is the harness directory relative to the project root.

### Lint

- `henia lint` checks config at its own `config` stage, in Go rather than as
  query rules, for every harness whose `artifacts` include `config`.
- Rules: `mcp-transport` (a shared server without exactly one of `command`
  and `url`), `mcp-header` (headers on a server without `url`, or a value in
  a form not every harness carries), `config-invalid` (a failing profile expression), `config-schema`
  (a schema violation, warning) and `config-unused` (an input the profile
  never reads, warning).
- A finding points at the `henia.toml` key it concerns, in the
  highest-precedence layer that sets it.

## Open

1. pi prompt templates (`prompts/`) and omp built-in command names.
