# Henia

Henia syncs AI coding assistant artifacts (skills, commands, agents) from git repositories to local harness directories.

## What is Henia?

Henia is a deployment tool that fetches AI artifacts from git repositories using [phora](https://github.com/srnnkls/phora) and deploys them to various AI coding assistant harnesses (Claude Code, OpenCode, Codex, etc.). It handles:

- **Fetching**: Clone and update git repositories containing artifacts
- **Discovery**: Find skills, commands, and agents in source repositories
- **Transformation**: Map keys/values, substitute variables, and transform references for different harnesses
- **Deployment**: Write artifacts to harness-specific directory structures

## Installation

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
artifacts = ["skills", "commands"]
```

2. Sync artifacts to your harnesses:

```bash
henia sync --config henia.toml --data-dir ~/.henia/sources
```

## Commands

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

Henia applies transformations to adapt artifacts for different harnesses:

### Variable Substitution

Use Go template syntax in frontmatter values and body content:

**Source:**
```yaml
model: {{.model_strong}}
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

- Go 1.23+
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
   - Apply variable substitution
   - Map frontmatter keys and values
   - Transform inline references
   - Generate commands from skills (if enabled)

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
