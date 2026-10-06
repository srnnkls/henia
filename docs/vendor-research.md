# Skill portability

What each vendor's skill contract requires, from primary sources checked on
2026-09-07. Henia writes each variant under its build output directory, and
Phora picks the installation paths. For how Henia maps metadata, see
[vendor profiles](vendor-profiles.md).

## Shared format

The [Agent Skills specification](https://agentskills.io/specification) defines a
skill as a directory holding `SKILL.md`. The file is YAML frontmatter followed by
Markdown, and the directory may also carry scripts and resources. `name` and
`description` are required. The optional portable fields are `license`,
`compatibility`, string-valued `metadata` and the experimental `allowed-tools`.

Directives extend Markdown. Rendering them as XML for Claude is a compiler
choice; the package format does not require it. A kept directive is prompt text
unless its consumer gives it meaning. Go templates run before directives render,
and references transform after.

## Vendor differences

| Profile | Project skill location or distribution | Metadata differences |
|---|---|---|
| Claude Code | `.claude/skills/<name>/SKILL.md` | adds invocation controls, tools, model, context, hooks and other Code fields |
| Claude upload/API | uploadable skill directory | limits frontmatter to the six standard fields; extra Code fields can fail the upload |
| Codex | `.agents/skills/<name>/SKILL.md` | optional `agents/openai.yaml` carries UI, invocation policy and dependencies |
| ChatGPT | skill or plugin distribution | shares OpenAI skill metadata and the optional `agents/openai.yaml`; has no documented local discovery directory |
| OpenCode | `.opencode/skills/<name>/SKILL.md` | reads name, description, license, compatibility and metadata; ignores other frontmatter |
| Gemini CLI | `.gemini/skills/<name>/SKILL.md` | documents name, description and Agent Skills compatibility; also reads `.agents/skills` |
| GitHub Copilot | `.github/skills/<name>/SKILL.md` | documents name, description, license and `allowed-tools`; also discovers compatible directories |
| Cursor | `.cursor/skills/<name>/SKILL.md` | adds paths, disable-model-invocation, icon, color and metadata |
| Agent Skills | portable `<name>/SKILL.md` directory | standard fields only; installation and optional fields depend on the consumer |

Sources: [Claude Code](https://code.claude.com/docs/en/skills#frontmatter-reference),
[OpenAI skills](https://learn.chatgpt.com/docs/build-skills),
[OpenCode](https://opencode.ai/docs/skills/),
[Gemini](https://geminicli.com/docs/cli/creating-skills/),
[Copilot](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills),
[Cursor](https://cursor.com/docs/skills).

## Mapping implications

- Renaming keys is not enough. Claude Code turns automatic invocation off with a
  negative boolean, `disable-model-invocation`. OpenAI's sidecar turns it on with
  a positive one, `policy.allow_implicit_invocation`.
- Tool lists need value mappings, and sometimes a different shape. Tool names
  and permissions differ between vendors. OpenCode's harness permissions belong
  in its configuration, not in skill frontmatter.
- A native override applies to one target, so no other vendor receives fields it
  cannot read. Unsupported behavior gets a diagnostic.
- Sidecar files need path and collision checks. Packaging a skill is separate
  from publishing a plugin, uploading it or enabling it in an account.
- Compiler variables and lint-only metadata stay available to templates and out
  of strict vendor frontmatter.
- Vendor schemas change independently of body rendering. The two stay separate
  configuration choices, and custom harness definitions are allowed.

## Specifications

The [markup spec](../specs/draft/markup-pipeline/spec.md) covers:

- the frontmatter template context and backward-compatible variables;
- `if`, `range` and `template`;
- the order `templates → directives → references`.

The [harness spec](../specs/draft/schema-transform/spec.md) covers canonical
metadata, configuration merging, extensible transforms and emitted files.
Profiles are TOML with Expr computations; lint rules are
[henia query](query.md) patterns.
