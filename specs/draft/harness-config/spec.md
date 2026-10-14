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
  before the three layers merge. Bundled profiles are sectioned, and a flat
  user or project layer still merges over them. Mixing both forms in one file
  fails.
- `[command]` is reserved and must stay empty. Commands never rendered through a
  profile (`transform.Prepare` maps skills and agents only). A per-type
  `commands.profile` still selects native preloads and `allowed-tools` through
  that profile's `[skill]` keys, as before.
- Agents use the harness profile's `[agent]` when present, as before none of
  the bundled profiles has one. A per-type `[harness.<n>.agents] profile` in
  the flat form serves agents through its `[skill]` keys, which keeps
  `claude-agent`-style profiles working.
- A harness profile without `[skill]` is valid; its skills render without a
  mapping.
- Every profile a harness names, per-type ones included, loads and compiles at
  config load. Only skill profiles require the nested layout.

### Directives

Profiles carry a `directives` default (`xml` for claude and claude-upload,
`keep` elsewhere). Precedence: profile, then the bundled preset, then
`[harness.<n>] directives`. The presets keep their values, so a preset harness
pointed at a custom profile keeps its directives. Config load and
`pipeline.Transformer` both fill the default, the latter for callers that build
harnesses without config load (`henia install`'s catalog skill). A
custom-named harness with `profile = "claude"` and no `directives` now renders
`xml`; before, it got no default.

### henia.toml

- `[shared]`: free-form tables, carried to every harness.
- `[harness.<n>.config.native]`: native data. A key naming a config file id
  addresses that file; the remaining keys address file `main`, and fail when
  the profile has no `main`.
- Arrays under `artifacts` (top level and per harness), `[shared]` and
  `[harness.<n>.config]` replace earlier layers instead of extending them.
  Concatenation made `artifacts = ["config"]` build skills too and made a
  project unable to shorten a user-level MCP server's `args`.
- `artifacts` accepts `config`. Without it nothing changes; the empty-list
  default (all document types) excludes it.

### Config rendering

- `[config.files.<id>]`: `path`, `format` (`json`, `yaml`, `toml`, `text`),
  optional `value`, `schema`, `arrays` (`replace` default, or `concat`).
- `value` sees `shared`, `harness` (this harness's typed inputs, such as
  `statusline`; no other profile sees them), `ctx` (`name`, `profile`, `path`, `variables`,
  `tools`, `keys`), `merge` and `require`; not `native`. Native data merges over
  the value exactly once, after evaluation, so native arrays never duplicate
  under `concat`.
- Nil table entries drop out (`{mcpServers: shared.mcp}` with no servers
  writes nothing); an empty result writes no file.
- Keys are sorted in every format. A `schema` adds `$schema` (json),
  `#:schema` (toml) or a `yaml-language-server` comment (yaml).
- Output is a per-harness artifact of type `config`, written under
  `<output>/<harness>/` with the existing collision, escape and clean-build
  handling.

### Schemas

- `santhosh-tekuri/jsonschema/v6`. The Claude settings and Codex config schemas
  are pinned in `internal/profile/schemas/`.
- Other URLs are fetched once into `os.UserCacheDir()/henia/schemas`, named
  after the URL plus a digest of it, so similar URLs never share a file.
- Violations and unavailable schemas are warnings; `strict = true` turns them
  into errors.

### Bundled config

| Harness | Files | Shared vocabulary |
|---|---|---|
| claude | `settings.json` (`main`, schema), `.mcp.json` (`mcp`) | `mcp.<server>`: `command`, `args`, `env` (`$NAME` → `${NAME}`, literals as is), `url` → `type = "http"`; `config.statusline` → `statusLine` |
| codex | `config.toml` (`main`, schema) | `mcp.<server>`: `command`, `args`, `url`, `env` (`$NAME` → `env_vars`, literals → `env`); renamed references fail |

The status line is Claude-specific: Codex's `tui.status_line` lists built-in
item names and runs no command. It lives in `[harness.claude.config.statusline]`,
read by expressions as `harness.statusline`, and takes exactly one of
`command` (used as is), `script` (inline, run by the shell) or `file`. A
`file` resolves against the root of the layer that sets it; building config
fails when it is missing or not executable, and commands that do not build
config never look at it. A later layer's `statusline` replaces the earlier one
whole. A harness whose profile reads no harness inputs warns that its
`statusline` is omitted (an error under `strict`). Claude Code gets the file's quoted absolute path; Henia does not copy
it, so the pointer stays valid once config install lands.

### Install

`henia install` drops `config` from each package's artifacts; a package that
selects only config installs nothing and is not built as another package's
dependency. Without that, an existing
`~/.claude/settings.json` blocks the whole install, and `--force` overwrites it.

## Open

1. Install for config: managed key paths with a sidecar manifest, behind a flag.
2. pi and omp profiles (skills and config).
3. Lint rules over config.
4. Header values in the shared MCP vocabulary (credential references inside
   strings).
