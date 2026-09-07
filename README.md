# Henia

Henia is a Go CLI that compiles canonical Markdown skills, commands, and agents for multiple AI harnesses. It uses Goldmark for semantic directives and includes an offline linter for references, duplication, and skill maintenance.

## What is Henia?

Henia is a deployment tool that fetches AI artifacts from git repositories using [phora](https://github.com/srnnkls/phora) and deploys them to various AI coding assistant harnesses (Claude Code, OpenCode, Codex, etc.).

**Core Capabilities:**

- **Fetching**: Clone and update git repositories containing artifacts
- **Discovery**: Find skills, commands, and agents in source repositories
- **Transformation**: Process artifacts through a multi-layer pipeline
- **Deployment**: Write artifacts to harness-specific directory structures

**Transformation Pipeline:**

Henia transforms artifacts through multiple layers, each using specialized engines:

1. **Data Layer** (Planned): CUE-based validation, schemas, and computation with `%cue` blocks
2. **Control Flow Layer**: Go template execution for conditionals, loops, and variable substitution
3. **Semantic Markup Layer**: Goldmark-based directives (`:::block{attrs}`, `:inline[text]{attrs}`) with format-specific rendering
4. **Reference Layer**: Transform artifact references (`$skill`, `/command`, `@agent`, `!tool`)

This architecture enables:
- **Portable artifacts**: Write once, deploy to multiple harnesses with different syntax requirements
- **Data validation**: Ensure artifact frontmatter meets schemas and constraints
- **Conditional content**: Include/exclude sections based on target harness or environment
- **Semantic markup**: Use directives that render as XML tags for Claude, pass-through for others
- **Cross-references**: Link between skills, commands, and agents with harness-specific output

## Installation

Requires Go 1.26 or later to build.

```bash
go install github.com/srnnkls/henia/cmd/henia@latest
```

Or build from source:

```bash
git clone https://github.com/srnnkls/henia
cd henia
go build -o henia ./cmd/henia
```

## Quick Start

1. Create a configuration file `henia.toml`:

```toml
artifacts = ["skills", "commands", "agents"]

[sources.my-skills]
repo = "https://github.com/user/my-ai-skills"
ref = "main"

[harness.claude]
path = "~/.claude"
structure = "nested"
format = "xml"
artifacts = ["skills", "commands"]
```

2. Sync artifacts to your harnesses:

```bash
henia sync --config henia.toml --data-dir ~/.henia/sources
```

## Commands

### build

Compile local artifacts without fetching repositories or deploying to live harness
locations. The source directory contains `skills/`, `commands/`, and/or `agents/`.

```bash
henia build ./examples --config examples/henia.toml --output .henia/build
henia build ./examples --harness claude,codex
```

The output is `<output>/<harness>/<artifact-type>/...`; `--output` defaults to
`.henia/build`. `--harness` selects configured harnesses. When `henia.toml` is absent,
build uses the bundled presets; an explicitly requested missing config is an error.
Resource files are copied alongside the generated main artifact. Resource contents
are not transformed. Rebuilding overwrites generated paths but does not remove files
for artifacts deleted from the source.

### lint

Scan Markdown files or directories recursively, including skill reference documents:

```bash
henia lint ./examples --strict
henia lint ./examples --format json --config examples/henia.toml
henia lint ./skills --disable duplicate-heading,large-skill
```

Output includes the file, one-based line/column, severity, rule, and explanation.
Errors return a nonzero exit status. Warnings also fail with `--strict`; JSON output
is always an array (including `[]` when clean). The linter does not execute templates,
fetch URLs, or claim that a remote page or model is current. Directive checks mask unevaluated template actions; build validates the fully expanded
result for each harness. Reference and duplication rules skip code blocks
and resolve artifact references against the complete set of scanned paths.

| Rule | What it checks |
|---|---|
| `metadata` | Missing name/description, malformed YAML, invalid review date |
| `invalid-template` | Malformed Go template syntax (parsed without execution) |
| `invalid-markup` | Malformed semantic directives |
| `broken-link` | Missing relative links, images, `#file` references and Markdown heading anchors |
| `missing-reference` | Unknown backtick-wrapped `$skill`, `/command`, `@agent` references |
| `duplicate-heading` | Repeated heading text within a document |
| `duplicate-content` | Repeated paragraphs of at least 12 words, across scanned files |
| `duplicate-skill` | Duplicate artifact names within the same artifact type |
| `outdated-reference` | Occurrences of configured obsolete strings |
| `large-skill` | Main artifact exceeds the line budget (default 500) |
| `stale-review` | Optional `last_verified` date is older than the review interval (default 180 days) |

Duplication comparisons ignore case and whitespace. Heuristic warnings need human
judgment; they never rewrite skills automatically. Configure known obsolete references
and thresholds in `henia.toml`:

```toml
[lint]
max_lines = 500
max_age_days = 180
disable = ["duplicate-heading"]

[lint.outdated]
"old-model-id" = "replacement-model-id"
"https://docs.example.test/v1/" = "https://docs.example.test/v2/"
```

### sync

Fetch sources and deploy to harnesses in one command.

```bash
henia sync [flags]
```

**Flags:**
- `--config <path>`: Configuration file path
- `--data-dir <path>`: Data directory for cloned repositories

**Example:**

```bash
henia sync --config henia.toml --data-dir ~/.henia/sources
```

### update

Alias for `sync`. Fetches sources and deploys to harnesses.

```bash
henia update [flags]
```

### deploy

Deploy artifacts from already-fetched sources to harnesses (skip git fetch).

```bash
henia deploy [flags]
```

Use this when sources are already cloned and you want to re-deploy without fetching updates.

### add

Add a repository as a source and immediately sync it.

```bash
henia add <repo> [flags]
```

**Flags:**
- `--ref <branch|tag|commit>`: Branch, tag, or commit to fetch (default: "main")

**Example:**

```bash
henia add https://github.com/user/skills --ref main
```

## Configuration

Henia uses TOML configuration files. The configuration defines:

1. **Sources**: Git repositories containing artifacts
2. **Harnesses**: Target AI coding assistants and their settings
3. **Transformations**: How to adapt artifacts for each harness

### Configuration Schema

```toml
# Global artifact types to sync (optional, defaults to all)
artifacts = ["skills", "commands", "agents"]

# Sources - git repositories to fetch
[sources.<name>]
repo = "<git-url>"
ref = "<branch|tag|commit>"

# Harnesses - deployment targets
[harness.<name>]
path = "<output-directory>"
structure = "nested" | "flat"
artifacts = ["skills", "commands", "agents"]
generate_commands_from_skills = false
include = ["artifact-name"]
exclude = ["artifact-name"]

# Frontmatter key mappings
[harness.<name>.keys]
original_key = "new_key"

# Frontmatter value mappings
[harness.<name>.values]
key_name = { "old_value" = "new_value" }

# Template variables
[harness.<name>.variables]
var_name = "value"

# Tool name mappings for reference transformation
[harness.<name>.tools]
bash = "Bash"
read = "Read"

# Reference output templates
[harness.<name>.references.skill]
output = "/{{.Name}}"

[harness.<name>.references.command]
output = "/{{.Name}}"

[harness.<name>.references.agent]
output = "@{{.Name}}"
```

### Configuration Examples

#### Claude Code

```toml
[harness.claude]
path = "~/.claude"
structure = "nested"
format = "xml"

[harness.claude.variables]
model_strong = "opus"
model_normal = "sonnet"
model_weak = "haiku"

[harness.claude.tools]
bash = "Bash"
read = "Read"
write = "Write"

[harness.claude.references.skill]
output = "/{{.Name}}"
```

#### OpenCode

```toml
[harness.opencode]
path = "~/.opencode"
structure = "flat"
generate_commands_from_skills = true

[harness.opencode.keys]
allowed_tools = "tools"

[harness.opencode.variables]
model_strong = "anthropic/claude-sonnet-4-5"
model_weak = "anthropic/claude-haiku-4-5"

[harness.opencode.references.skill]
output = "@{{.Name}}"
```

## Artifact Structure

Henia recognizes three artifact types:

### Skills

Directory or file containing a skill definition.

**Directory structure:**
```
skills/
  my-skill/
    SKILL.md          # Main definition
    reference/        # Optional resources
      guide.md
```

**File structure:**
```
skills/
  my-skill.md         # Flat file
```

**SKILL.md format:**
```markdown
---
name: my-skill
description: Skill description
model: opus
allowed_tools:
  - read
  - write
user-invocable: true
---

# My Skill

Skill content here.
```

### Commands

### build

Compile local artifacts without fetching repositories or deploying to live harness
locations. The source directory contains `skills/`, `commands/`, and/or `agents/`.

```bash
henia build ./examples --config examples/henia.toml --output .henia/build
henia build ./examples --harness claude,codex
```

The output is `<output>/<harness>/<artifact-type>/...`; `--output` defaults to
`.henia/build`. `--harness` selects configured harnesses. When `henia.toml` is absent,
build uses the bundled presets; an explicitly requested missing config is an error.
Resource files are copied alongside the generated main artifact. Resource contents
are not transformed. Rebuilding overwrites generated paths but does not remove files
for artifacts deleted from the source.

### lint

Scan Markdown files or directories recursively, including skill reference documents:

```bash
henia lint ./examples --strict
henia lint ./examples --format json --config examples/henia.toml
henia lint ./skills --disable duplicate-heading,large-skill
```

Output includes the file, one-based line/column, severity, rule, and explanation.
Errors return a nonzero exit status. Warnings also fail with `--strict`; JSON output
is always an array (including `[]` when clean). The linter does not execute templates,
fetch URLs, or claim that a remote page or model is current. Directive checks mask unevaluated template actions; build validates the fully expanded
result for each harness. Reference and duplication rules skip code blocks
and resolve artifact references against the complete set of scanned paths.

| Rule | What it checks |
|---|---|
| `metadata` | Missing name/description, malformed YAML, invalid review date |
| `invalid-template` | Malformed Go template syntax (parsed without execution) |
| `invalid-markup` | Malformed semantic directives |
| `broken-link` | Missing relative links, images, `#file` references and Markdown heading anchors |
| `missing-reference` | Unknown backtick-wrapped `$skill`, `/command`, `@agent` references |
| `duplicate-heading` | Repeated heading text within a document |
| `duplicate-content` | Repeated paragraphs of at least 12 words, across scanned files |
| `duplicate-skill` | Duplicate artifact names within the same artifact type |
| `outdated-reference` | Occurrences of configured obsolete strings |
| `large-skill` | Main artifact exceeds the line budget (default 500) |
| `stale-review` | Optional `last_verified` date is older than the review interval (default 180 days) |

Duplication comparisons ignore case and whitespace. Heuristic warnings need human
judgment; they never rewrite skills automatically. Configure known obsolete references
and thresholds in `henia.toml`:

```toml
[lint]
max_lines = 500
max_age_days = 180
disable = ["duplicate-heading"]

[lint.outdated]
"old-model-id" = "replacement-model-id"
"https://docs.example.test/v1/" = "https://docs.example.test/v2/"
```

```
commands/
  my-command/
    COMMAND.md
```

**COMMAND.md format:**
```markdown
---
name: my-command
description: Command description
---

# My Command

Command content here.
```

### Agents

```
agents/
  my-agent/
    AGENT.md
```

**AGENT.md format:**
```markdown
---
name: my-agent
description: Agent description
model: haiku
---

# My Agent

Agent content here.
```

## Transformations

Henia processes artifacts through a powerful multi-layer transformation pipeline, enabling portable artifacts that adapt to different AI coding assistants.

### Transformation Architecture

The implementation executes templates, Goldmark directives, and references in that order. CUE remains a separate planned extension:

1. **Data Layer** (Planned) - CUE validation and computation
2. **Control Flow Layer** - Go template execution
3. **Semantic Markup Layer** - Goldmark directive rendering
4. **Reference Layer** - Artifact reference transformation

Each layer uses a specialized engine for its purpose, ensuring clean separation of concerns.

### Variable Substitution

Use Go template syntax in frontmatter string values and body content. Frontmatter values (including lists, booleans, and nested maps) form the template context. Harness variables override same-named frontmatter values, preserving existing variable templates:

**Source:**
```yaml
model: "{{.model_strong}}"
```

**Configuration:**
```toml
[harness.claude.variables]
model_strong = "opus"
```

**Result:**
```yaml
model: opus
```

### Key Mappings

Rename frontmatter keys:

**Configuration:**
```toml
[harness.opencode.keys]
allowed_tools = "tools"
```

**Source:**
```yaml
allowed_tools:
  - read
```

**Result:**
```yaml
tools:
  - read
```

### Value Mappings

Transform specific values:

**Configuration:**
```toml
[harness.opencode.values.model]
opus = "anthropic/claude-sonnet-4-5"
haiku = "anthropic/claude-haiku-4-5"
```

**Source:**
```yaml
model: opus
```

**Result:**
```yaml
model: anthropic/claude-sonnet-4-5
```

### Reference Transformation

Transform inline references in artifact bodies:

**Reference types:**
- `$skill-name` - Skill reference
- `/command-name` - Command reference
- `@agent-name` - Agent reference
- `#file-path` - File reference
- `!tool-name` - Tool reference

**Configuration:**
```toml
[harness.claude.tools]
bash = "Bash"

[harness.claude.references.skill]
output = "/{{.Name}}"
```

**Source:**
```markdown
Use `$code-review` after implementation.
Run `!bash` to execute tests.
```

**Result:**
```markdown
Use `/code-review` after implementation.
Run `Bash` to execute tests.
```

### Data Validation with CUE (Planned)

The CUE layer enables data validation, schemas, and computation using the CUE language:

**`%cue` blocks:**
```markdown
---
name: my-skill
enabled: false
---

%cue {{
// Validate and compute values
enabled: true  // Override frontmatter
models: {
  strong: "opus"
  weak: "haiku"
}
}}

# My Skill

Use model: {{.models.strong}}
```

**Features:**
- **Schema validation**: Ensure frontmatter meets constraints
- **Value computation**: Calculate derived values from inputs
- **Unification**: Merge multiple CUE blocks with conflict detection
- **Context access**: Reference harness configuration via `config` namespace
- **Type safety**: Strong typing with CUE's type system

**CUE evaluation:**
- Multiple `%cue` blocks are unified in document order
- CUE values override frontmatter keys on conflict
- Final unified value becomes template context
- Validation errors report clear constraint violations

### Semantic Markup with Directives

The Goldmark layer enables semantic markup that renders differently per harness:

**Block directives:**
```markdown
:::instruction{priority="critical"}
Always write tests before implementation.
:::

:::example{lang="python"}
def test_feature():
    assert feature() == expected
:::
```

**Inline directives:**
```markdown
Use :term[TDD]{abbr="Test-Driven Development"} for all features.
Configure :config[timeout]{unit="seconds" default="30"} appropriately.
```

**Format-specific rendering:**

For Claude harness with `format = "xml"`:
```markdown
<instruction priority="critical">
Always write tests before implementation.
</instruction>

Use <term abbr="Test-Driven Development">TDD</term> for all features.
```

For default harness with `format = "directives"`:
```markdown
:::instruction{priority="critical"}
Always write tests before implementation.
:::

Use :term[TDD]{abbr="Test-Driven Development"} for all features.
```

**Benefits:**
- **Semantic richness**: Add meaning to content with typed directives
- **Format adaptation**: XML tags for Claude, passthrough for others
- **Standard syntax**: CommonMark-compatible directive syntax
- **Nested content**: Directives can contain markdown and other directives

### Pipeline Integration

```
YAML frontmatter → Go templates → Goldmark directives → references → write
```

Frontmatter key/value mappings apply to the rendered metadata. Directives are semantic
markup; they do not execute code. CUE and Starlark processing are not implemented.

Both container directives (`:::name{attrs}`) and inline directives
(`:name[content]{attrs}`) support nesting. Goldmark parses the attributes, including
`#id`, `.class`, quoted strings with escaped quotes, and unquoted scalar values.
Quote values containing spaces or punctuation. Directive and attribute names use
ASCII letters or `_`, followed by letters, digits, `_`, `-`, or `.`. Duplicate
attribute names and structured attribute values are rejected. Two-colon leaf
directives are not processed in this version.

`format = "xml"` converts only directives to XML tags and escapes attribute values.
The body remains Markdown, so the complete output is not an XML document.
`format = "directives"` (also the default when omitted) normalizes directive spacing
and quotes attributes. Empty attribute blocks may disappear. Markdown outside the
directives, including code fences, retains its original bytes.

Malformed directives report line and column positions in the expanded body.
All selected artifacts are transformed before any output is written; syntax errors
and output collisions fail preflight. Filesystem errors during writing can leave
partial output and cause a nonzero exit status.

See [the runnable example](examples/skills/review/SKILL.md) and its
[harness configuration](examples/henia.toml).

## Directory Structures

Henia supports two output structures:

### Nested (Default)

Each artifact is a directory with a main file and optional resources:

```
~/.claude/
  skills/
    code-test/
      SKILL.md
      reference/
        guide.md
  commands/
    test-run/
      COMMAND.md
```

### Flat

Each artifact is a single markdown file:

```
~/.opencode/
  skills/
    code-test.md
  commands/
    test-run.md
```

Configure with:
```toml
[harness.name]
structure = "flat"
```

## Artifact Filtering

Control which artifacts are deployed:

**By type:**
```toml
[harness.claude]
artifacts = ["skills", "commands"]  # Excludes agents
```

**By name (include list):**
```toml
[harness.claude]
include = ["code-test", "code-review"]  # Only these artifacts
```

**By name (exclude list):**
```toml
[harness.claude]
exclude = ["deprecated-skill"]  # Everything except this
```

## Multiple Sources

Henia can sync from multiple git repositories:

```toml
[sources.company-skills]
repo = "https://github.com/company/ai-skills"
ref = "main"

[sources.personal-skills]
repo = "https://github.com/user/my-skills"
ref = "v1.2.0"

[sources.third-party]
repo = "https://github.com/community/skills"
ref = "develop"
```

All artifacts from all sources are discovered and deployed together.

## Development

### Prerequisites

- Go 1.26+
- [mise](https://mise.jdx.dev/) (optional, for task runner)
- [scrut](https://github.com/facebookincubator/scrut) (for integration tests)

### Build

```bash
go build -o henia ./cmd/henia
```

Or with mise:

```bash
mise run build
```

### Test

Run unit tests:

```bash
go test ./...
```

Or with mise:

```bash
mise run test:unit
```

Run integration tests:

```bash
mise run test:integration
```

Run all tests:

```bash
mise run test
```

## How It Works

1. **Fetch Phase** (phora):
   - Clone or update git repositories to local data directory
   - Track commit hashes for each source

2. **Discovery Phase**:
   - Scan source directories for skills/commands/agents
   - Parse frontmatter and body content
   - Collect resources for directory-based artifacts

3. **Transform Phase**:
   - **Parse frontmatter**: Extract YAML metadata from artifacts
   - **CUE evaluation** (planned): Validate data and compute derived values
   - **Template execution**: Go templates for conditionals and variable substitution
   - **Directive rendering**: Goldmark-based semantic markup transformation
   - **Reference transformation**: Convert artifact references to harness-specific syntax
   - **Key/value mapping**: Normalize frontmatter for target harness
   - **Command generation**: Auto-generate commands from user-invocable skills (if enabled)

4. **Deploy Phase**:
   - Create harness directory structure
   - Write transformed artifacts
   - Copy resources for directory-based artifacts
   - Update lock files to track managed files

## Supported Harnesses

Henia includes default configurations for popular AI coding assistants:

- **Claude Code** - Anthropic's official CLI
- **OpenCode** - Open source coding assistant
- **Codex** - OpenAI Codex CLI
- **Gemini** - Google Gemini CLI
- **GitHub Copilot** - GitHub's AI pair programmer
- **Cursor** - AI-powered code editor

See `internal/defaults/henia.toml` for default harness configurations.

## Advanced Features

### Transformation Pipeline Status

**Currently Implemented:**
- ✓ Local multi-harness builds and offline skill linting
- ✓ Goldmark block/inline directives with XML and normalized directive output
- ✓ Typed frontmatter template context with harness variable overrides
- ✓ Go template execution with variable substitution
- ✓ Frontmatter key/value mapping
- ✓ Reference transformation (`$skill`, `/command`, `@agent`, `!tool`)
- ✓ Multiple source support
- ✓ Harness-specific configuration

**Planned (In Development):**
- ⏳ CUE data layer for validation and computation (`%cue` blocks)


See the [Transformations](#transformations) section for detailed examples of both current and planned features.

### Generate Commands from Skills

Automatically create commands that invoke skills:

```toml
[harness.opencode]
generate_commands_from_skills = true
```

This creates a command for each skill marked with `user-invocable: true`.

### Namespacing

Artifacts from non-global sources are automatically namespaced:

**Source:** `company/skills/code-test`
**Output:** `company.code-test`

This prevents name conflicts when syncing from multiple sources.

### Lock Files

Henia tracks managed files in `.phora.lock` files in each harness directory. This enables:

- Re-deployment without duplicates
- Cleanup of removed artifacts
- Tracking which source manages each artifact

## FAQ

**Q: What's the difference between henia and phora?**

A: Phora handles git repository management and local deployment with transformation. Henia extends phora to support multiple sources and centralized configuration for syncing to multiple harnesses.

**Q: Can I use henia without git repositories?**

A: Henia requires sources to be git repositories. Use phora directly if you want to deploy from local directories.

**Q: How do I update artifacts?**

A: Run `henia sync` or `henia update`. This fetches the latest commits and re-deploys.

**Q: Can I customize transformations per artifact?**

A: Transformations are defined per harness, not per artifact. All artifacts for a harness use the same transformation rules.

**Q: What happens if artifacts have the same name?**

A: Later sources override earlier ones. Use include/exclude filters to control this.

## License

MIT

## Related Projects

- [phora](https://github.com/srnnkls/phora) - Git-based package manager for AI artifacts
- [tropos](https://github.com/srnnkls/tropos) - AI artifact registry and discovery

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Submit a pull request

## Support

- Issues: https://github.com/srnnkls/henia/issues
- Discussions: https://github.com/srnnkls/henia/discussions
