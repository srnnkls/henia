# Skill portability

Primary-source research checked 2026-09-07. This document distinguishes the
vendor contract from Henia's compiler choices; it is not an installation guide.

## Shared format

The [Agent Skills specification](https://agentskills.io/specification) defines a
directory containing `SKILL.md`: YAML frontmatter followed by Markdown, with
optional scripts and resources. Required fields are `name` and `description`.
Optional portable fields are `license`, `compatibility`, string-valued `metadata`,
and experimental `allowed-tools`.

Directive syntax is an extension to Markdown. Selecting XML output for Claude is
an author/compiler preference, not a requirement of the skill package format.
Retained directives are prompt text unless a consumer implements their semantics.
Go templates execute before directive rendering; references transform afterward.

## Vendor differences

| Profile | Project skill location or distribution | Metadata differences |
|---|---|---|
| Claude Code | `.claude/skills/<name>/SKILL.md` | Adds invocation controls, tools, model, context, hooks and other Code fields |
| Claude upload/API | Uploadable skill directory | Restricts frontmatter to the six standard fields; extra Code fields can cause upload failure |
| Codex | `.agents/skills/<name>/SKILL.md` | Optional `agents/openai.yaml` carries UI, invocation policy and dependencies |
| ChatGPT | Skill/plugin distribution | Shares OpenAI skill metadata and optional `agents/openai.yaml`; do not invent a local discovery directory |
| OpenCode | `.opencode/skills/<name>/SKILL.md` | Recognizes name, description, license, compatibility and metadata; ignores other frontmatter |
| Gemini CLI | `.gemini/skills/<name>/SKILL.md` | Documents name/description and Agent Skills compatibility; `.agents/skills` is also recognized |
| GitHub Copilot | `.github/skills/<name>/SKILL.md` | Documents name, description, license and `allowed-tools`; also discovers compatible directories |
| Cursor | `.cursor/skills/<name>/SKILL.md` | Adds paths, disable-model-invocation, icon, color and metadata |
| Agent Skills | Portable `<name>/SKILL.md` directory | Standard fields only; installation and optional field support depend on the consumer |

Sources: [Claude Code](https://code.claude.com/docs/en/skills#frontmatter-reference),
[OpenAI skills](https://learn.chatgpt.com/docs/build-skills),
[OpenCode](https://opencode.ai/docs/skills/),
[Gemini](https://geminicli.com/docs/cli/creating-skills/),
[Copilot](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills),
[Cursor](https://cursor.com/docs/skills).

## Mapping implications

- Key renames alone are insufficient. Automatic invocation uses a negative
  `disable-model-invocation` boolean in Claude Code and a positive
  `policy.allow_implicit_invocation` boolean in OpenAI's sidecar.
- Tool lists need value mappings and sometimes shape changes. Tool names and
  permissions are not interchangeable. OpenCode's harness permissions belong in
  its configuration, not in skill frontmatter.
- Native overrides must be scoped to a target so another vendor does not receive
  unsupported fields. Unsupported behavior needs a diagnostic.
- Sidecar emission needs collision and path validation. Packaging a skill is
  separate from publishing a plugin, uploading it or enabling it in an account.
- Compiler variables and lint-only metadata should remain available to templates
  without leaking into strict vendor frontmatter.
- Vendor schemas can evolve independently from body renderers. Keep the two
  configuration choices independent and allow custom harness definitions.

## Scope alignment

The [markup scope](../specs/draft/markup-pipeline/spec.md) requires frontmatter
template context, backward-compatible variables, `if`/`range`/`template`, and
`templates → directives → references`. The linked
[harness scope](../specs/draft/schema-transform/spec.md) additionally specifies
canonical metadata, configuration merging, extensible transforms and emitted
files. The user selected TOML + Expr for metadata computations and the lint
rule DSL; this supersedes the original Starlark requirement.
