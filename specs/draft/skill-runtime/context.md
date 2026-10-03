# Implementation context

The normative contract is [spec.md](spec.md). Prior art and harness limits that
shaped it are summarised at the end.

## Carry over from the local store commits

`c62250f` and `aecc394` are unpushed and get reworked, not amended.

| Keep | Where |
|---|---|
| Heading sections and anchors | `internal/markup/sections.go`, lint uses it |
| `ls`, `show`, `context` and output budget | `internal/cli/store.go` |
| Caller detection and render cache | `internal/cli/render.go` |
| Serve-time render | `build.Render` |
| Slots over additional directories | `slots.Discover`, `store.Scan` |

| Remove | Why |
|---|---|
| `henia.store` frontmatter and its vendor-profile exception | Exposure is consumer configuration |
| The `store` build pseudo-target and `store/henia.toml` | Sources carry their own configuration |
| `Artifact.Stored`, `Transformer.Stored` keyed on frontmatter | Replaced by the projection set |

| Change | To |
|---|---|
| `internal/store` | `internal/library`: sources, catalog, identity resolution |
| `--store DIR` | `--source DIR` |
| `Transformer.Stored` | the set of skills outside the harness projection |
| `docs/store.md` | `docs/runtime.md` |

## Prior art

- Nix store and profiles: content stays in the store; the consumer's profile is
  the exposed view. Henia's library and projection follow this split.
- Emacs autoloads: entry points are listed eagerly, bodies load lazily and stay
  loaded. Henia's runtime renders on request and caches.
- Go module cache and vendor, Node `node_modules`: project before global.
- Claude `skillOverrides`, Codex `skills.config`, Microsoft Agent Framework
  filters: exposure is host configuration, not a skill property.
- Claude Code lists skills within 1% of the context window, Codex within 2%;
  reports show 103 of 119 Codex descriptions truncated and 67 of 130 Claude
  skills reduced to names. Stacklok measured about ten full entries as the
  practical ceiling and found an imperative instruction naming the tool to be
  the main discovery path beyond that.
- Claude `plugin:skill` namespacing and the agentskills guidance to warn on
  collisions shape `source:name` and the ambiguity rule.
- pixi and devbox write their own `.gitignore` inside a mixed tool folder.
