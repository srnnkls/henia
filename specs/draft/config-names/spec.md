---
issue_type: Feature
created: 2026-10-04
status: Accepted
stage: draft
---

# Clear names for henia.toml

## Goal

A source's `henia.toml` sits next to its skills and decides, per harness, how
they compile and which are static or dynamic. Its keys should say what they
control without reading the code. Today several do not: `include`/`exclude` read
as file globs, `artifact_mappings` hides that it overrides per artifact type,
and `keys`/`values`, `format` and `structure` say nothing about what they
change.

## Findings

Harness configuration (Claude Code, Codex, Agent Skills, OpenCode, Gemini CLI,
Cursor, Copilot, pi; fetched 2026-10-04):

- Being installed and being listed to the model are separate axes in every
  harness; neither is called exposure, projection or visibility. Henia's
  distinction is a third one, how a skill is delivered: built ahead of time
  or rendered on demand.
  Claude Code's `skillOverrides` grades it (`on`, `name-only`, `off`).
- Invocation policy is two booleans with shared names across Claude Code,
  Copilot, Cursor and pi: `disable-model-invocation` and `user-invocable`.
  Codex states the first positively (`policy.allow_implicit_invocation`).
- Selection lists default to "everything" and come as allow/deny pairs whose
  docs always state that deny wins.

Packaging systems (Cargo, npm, Hatch, Bazel, Nix, VS Code; from memory, not
re-fetched):

- `include`/`exclude` always mean file globs, never member exposure.
- Systems with an on-demand tier name the discovery channel, not the item:
  Nix `packages` versus `legacyPackages`, Python `scripts` versus
  `entry-points`.
- Per-target tables are easiest to predict when an override table reuses the
  base table's key names (Hatch `build` and `build.targets.<name>`).
- "target" is overloaded (artifact kind and platform); "harness" is not.

## Proposal

Keep `[harness.<name>]`, `artifacts`, `tools`, `strict` and `files`. Group each
harness's keys by what they change, and give every artifact type in `artifacts`
its own table that reuses the harness keys.

```toml
[build]
output = ".henia/build"
clean = true

[harness.claude]
profile = "claude"
artifacts = ["skills", "agents"]      # unchanged: artifact types built for this harness
directives = "xml"                    # format: "xml", "markdown" (or "md"), or "keep" for :::directive syntax
layout = "nested"                     # structure
strict = true                         # unchanged: transform warnings fail the build

[harness.claude.skills]               # per-type table; takes the harness keys too
dynamic = ["debate", "dispatch", "limen", "loqui", "validate"]    # exclude
catalog = true                        # default: build the `henia` catalog skill when any skill is dynamic

[harness.claude.agents]               # artifact_mappings.agents
profile = "claude-agent"
layout = "flat"

[harness.claude.variables]            # unchanged
preload_context = "true"

[harness.claude.tools]                # unchanged
bash = "Bash"

[harness.claude.references]           # references.<type>.output
skill = "/{{.Name}}"

[harness.claude.frontmatter]
rename = { allowed_tools = "tools" }  # keys
values = { model = { strong = "opus" } }   # values

[harness.claude.files]                # unchanged: output path = { source = … }
"AGENTS.md" = { source = "instructions/AGENTS.md" }
```

### Static and dynamic skills

A skill is static or dynamic per harness, never by itself:

- Static: `henia build` compiles it into the harness's skill tree; the harness
  lists it and loads it directly.
- Dynamic: not built for that harness; the agent reads it with `henia show`,
  rendered on demand.

`henia show` serves both. Per artifact-type table (`[harness.<name>.skills]`, `.agents`,
`.commands`):

- `static = [...]`: exactly these are built; all others are dynamic.
- `dynamic = [...]`: these are dynamic; all others are built.
- Neither: every skill is static. Both: a configuration error, so no
  precedence rule is needed.

#### The catalog skill

When a harness has dynamic skills, `henia build` adds a generated skill named
`henia` to that harness's tree, so the agent learns the dynamic skills exist
without the source writing instructions for it:

- Its description names the job ("skills beyond those installed: list them
  and read one with henia") and stays within every profile's description
  limit.
- Its body lists that harness's dynamic skills from this source, each with its
  description and the `henia show <skill>` command that reads it, and points to
  `henia ls` for skills from other sources.
- It renders through the harness profile like any skill; on Claude it allows
  `Bash(henia ls *)` and `Bash(henia show *)`.
- Its text is Henia's, embedded in the binary; sources do not copy it.
- A source skill named `henia` in the same harness is a build error while
  `catalog` is on.

`catalog = false` in `[harness.<name>.skills]` turns it off, for sources
that describe the runtime in their own instructions. A harness without dynamic
skills never gets it. It is the static entry to the catalog `henia ls` prints:
it names this harness's dynamic skills and defers to `henia ls` for the rest.

`henia context` currently calls slot providers and referenced sections
"dynamic context"; that wording becomes "runtime context" so "dynamic" keeps
one meaning.

### Invocation

Invocation policy stays in skill frontmatter under its harness-neutral names,
`henia.auto_invoke` and `henia.user_invocable`. Henia does not enforce it; each
profile translates it (Claude `disable-model-invocation` and `user-invocable`,
Codex `policy.allow_implicit_invocation`), and references to a skill the model
may not invoke render as `henia show <skill>`.

### Migration

Old keys load for one release with a deprecation warning naming the new key;
`henia lint` reports them. Tropos migrates in the same release.

## Non-goals

- Moving listing out of the source's `henia.toml`.
- Renaming `[build]`, `[lint]`, `[preload]` or `variables`.
