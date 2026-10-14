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
  `mode` (octal, `0644` default).
- `value` sees `shared`, `harness` (this harness's typed inputs), `ctx`
  (`name`, `profile`, `path`, `variables`, `tools`, `keys`), `merge` and
  `require`. Native data merges over the result once.
- Nil table entries and array items drop out; an empty result writes no file.
- Keys are sorted in every format. A `schema` adds `$schema` (json),
  `#:schema` (toml) or a `yaml-language-server` comment (yaml).
- Output is a per-harness artifact of type `config` under `<output>/<harness>/`,
  subject to the build's collision, escape and clean-build handling.

### Schemas

- Validation uses `santhosh-tekuri/jsonschema/v6`. The Claude settings and
  Codex config schemas are pinned in `internal/profile/schemas/`.
- Other URLs are fetched once into `os.UserCacheDir()/henia/schemas`, one file
  per URL digest.
- Violations and unavailable schemas are warnings, errors under `strict`.

### Bundled config

| Harness | Files | Inputs |
|---|---|---|
| claude | `settings.json` (`main`, schema), `.mcp.json` (`mcp`), `statusline.sh` (`statusline`, `0755`) | `shared.mcp.<server>`: `command`, `args`, `env` (`$NAME` → `${NAME}`, literals as is), `url` → `type = "http"`; `harness.statusline` → `statusLine` |
| codex | `config.toml` (`main`, schema) | `shared.mcp.<server>`: `command`, `args`, `url`, `env` (`$NAME` → `env_vars`, literals → `env`); a reference must name its own variable |

### Status line

`[harness.claude.config.statusline]` takes exactly one of `command` (used as
is), `script` (inline, run by the shell) or `file`, plus `padding`,
`refresh_interval` and `hide_vim_mode`.

- A `script` or `file` deploys as `statusline.sh` in the harness tree, with a
  `#!/bin/sh` line when it has none; `statusLine.command` runs
  `"${CLAUDE_CONFIG_DIR:-$HOME/.claude}/statusline.sh"`.
- A `file` resolves against the root of the layer that sets it. Building config
  fails when it is missing.
- A later layer's `statusline` replaces the earlier one whole.
- A harness whose profile reads no harness inputs warns that its `statusline`
  is omitted.

### Install

`henia install` drops `config` from each package's artifacts. A package that
selects only config is neither installed nor built as another package's
dependency.

## Open

1. Install for config: managed key paths with a sidecar manifest, behind a flag.
2. pi and omp profiles (skills and config).
3. Lint rules over config.
4. Header values in the shared MCP vocabulary.
