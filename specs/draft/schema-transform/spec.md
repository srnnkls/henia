---
issue_type: Feature
created: 2026-01-18
status: Draft
stage: draft
claude_plan: ~/.claude/plans/quirky-mapping-sunset.md
---

# Harness Transform Pipeline

## Goal

Replace hardcoded Go transformation logic with a pluggable Starlark-based transform pipeline, enabling user-extensible harness support without recompiling.

## Architecture

```
Canonical Frontmatter (Go struct)
         │
         ▼
    ToMap() → input dict
         │
    ┌────┴────┐
    │         │
    ▼         ▼
  ctx       transform.star
(from TOML)  (Starlark)
    │         │
    └────┬────┘
         │
         ▼
  transform(input, ctx)  # henia is global
         │
         ▼
  {frontmatter, files, warnings}
         │
         ▼
    Go renders output
```

**Key principle:** TOML handles declarative config (variables, tool mappings, references). Starlark handles non-trivial logic (conditionals, coupling rules, reshaping).

## Requirements

### Functional Requirements

- Transform artifact frontmatter using Starlark scripts per harness
- Load harness config from TOML with merge semantics (project > user > embedded)
- Resolve transform scripts: `<path>/transform.star` > embedded default
- Emit harness-specific files (e.g., `opencode.json` for OpenCode)
- Support user-defined harnesses without Go changes

### Technical Requirements

- Add Starlark interpreter dependency (`go.starlark.net`)
- Define canonical Go struct for artifact frontmatter with `ToMap()` method
- Build `ctx` dict from merged TOML config
- Provide `henia` helpers for common operations (get, ensure, map)
- Maintain backwards compatibility: existing transforms continue working

## Config Model

### Precedence (highest wins)

1. Project TOML (repo-local)
2. User/global TOML
3. Embedded harness defaults

### Merge Strategy (Cargo-style)

- **Scalars:** Most specific wins (project overrides user overrides embedded)
- **Arrays:** Concatenated, higher-precedence items placed later
- **Tables:** Deep merge recursively, with scalar override on collision

### Example TOML

```toml
[harness.claude]
path = "~/.claude"
artifacts = ["skills", "commands"]
strict = true

[harness.claude.variables]
model_strong = "claude-opus-4-5-20251101"
model_weak = "claude-3-5-haiku-latest"

[harness.claude.tools]
bash = "Bash"
read = "Read"
edit = "Edit"

[harness.claude.keys]
allowed_tools = "tools"

[harness.claude.references.skill]
output = "/{{.Name}}"
```

## Error Handling

**Policy:** Fail-fast. Script errors abort transformation immediately.

- Starlark syntax errors → immediate abort with line number
- `fail()` calls → immediate abort with custom message
- Missing required fields → immediate abort
- Warnings collected and logged to stderr with artifact path prefix, also returned in result for caller aggregation

## Sandbox Policy

**Policy:** Strict hermetic execution.

| Constraint | Value | Configurable |
|------------|-------|--------------|
| Filesystem | No access | No |
| Network | No access | No |
| Max steps | 10,000 (default) | Yes, globally via `[starlark].max_steps` |
| Recursion | 1,000 depth | No |

Scripts have access only to: `input`, `ctx`, and `henia` builtins.

### Global Starlark Config

```toml
[starlark]
max_steps = 50000  # Override default 10k limit
```

## File Emission Security

Emitted file paths are validated before writing:
- No `..` path components (traversal)
- Must be relative paths (no leading `/`)
- No symlink targets outside harness.path

Violations fail the transform with a clear error message.

## Harness Discovery

**Resolution order (first match wins):**

1. Project: `.henia/harnesses/{name}/transform.star`
2. User: `~/.config/henia/harnesses/{name}/transform.star`
3. Embedded: `harnesses/{name}/transform.star`

## Starlark Contract

### Input Schema

```python
input = {
    "name": str,           # Required
    "description": str,    # Optional
    "model_tier": str,     # Optional: "strong" | "weak"
    "tools": [str],        # Optional
    "tools_policy": dict,  # Optional: per-tool permission overrides
    "enabled": bool,       # Optional
    "user_invocable": bool,# Optional
    # ... extra fields passed through
}
```

### Context Schema

```python
ctx = {
    "name": str,           # Harness name
    "path": str,           # Harness output path
    "artifacts": [str],    # Artifact types
    "strict": bool,        # Strict validation mode
    "variables": dict,     # User-defined variables (never None, empty dict if unset)
    "tools": dict,         # Tool name mappings (never None, empty dict if unset)
    "keys": dict,          # Field key mappings (never None, empty dict if unset)
    "references": dict,    # Reference config (never None, empty dict if unset)
}
```

**Contract:** Dict fields (`variables`, `tools`, `keys`, `references`) are guaranteed never None. Scripts can safely call `.get()` without nil checks.

### Starlark Modules

| Module | Source | Functions |
|--------|--------|-----------|
| `json` | `go.starlark.net/lib/json` | `encode`, `decode`, `indent` |
| `re` | `github.com/magnetde/starlark-re` | `match`, `search`, `sub`, `split`, `findall` |

### Henia API

`henia` is a pre-loaded global available in all transform scripts.

```python
# henia.dict - extended dict operations
henia.dict.get_path(d, "a.b.c", default=None)  # Nested dot-path access
```

**Behavior:** `get_path()` returns `default` (or `None` if omitted) for missing paths. Does not raise on missing keys or `None` values in the path.

**Built-in (no module needed):** `dict.get()`, `fail()`, list comprehensions, `hasattr()`, `getattr()`

### Output Schema

```python
def transform(input, ctx):
    """Transform input frontmatter to harness-specific output.

    Args:
        input: Canonical frontmatter dict from ToMap()
        ctx: Harness context dict from merged TOML config

    Returns dict with keys:
        frontmatter: dict - Output frontmatter fields
        files: dict - Additional files (relative to harness.path)
                      Values: dict → JSON, str → raw content
        warnings: [str] - Non-fatal issues (collected, don't abort)

    Note: `henia` is a pre-loaded global, not passed as an argument.
    """
    return {
        "frontmatter": {...},
        "files": {"path/file.json": {...}},
        "warnings": [...],
    }
```

## Acceptance Criteria

- [ ] Given a harness with `[harness.claude]` config and embedded `transform.star`
  When an artifact with `model_tier: "strong"` is transformed
  Then output frontmatter contains `model: "claude-opus-4-5-20251101"`

- [ ] Given `~/.config/henia/harnesses/claude/transform.star` exists (user override)
  When transformer is resolved
  Then user script takes precedence over embedded

- [ ] Given harness config with `tools = { bash = "Bash" }`
  When Starlark calls `ctx["tools"]["bash"]`
  Then it returns `"Bash"`

- [ ] Given OpenCode transform with `write = "allow"` in policy
  When permission coupling logic runs
  Then `edit` permission is also set to `"allow"`

- [ ] Given a new harness defined only in TOML + `transform.star`
  When sync runs
  Then artifacts transform correctly without Go changes

- [ ] Given `strict = true` in harness config
  When output frontmatter contains fields with invalid types vs canonical schema
  Then type validation error is raised (unknown fields are allowed)

- [ ] Given a Starlark script that emits `files: {"../escape.txt": "..."}`
  When transform runs
  Then path traversal is detected and transform fails with error

## Dependency Graph

> Machine-readable: [dependencies.yaml](dependencies.yaml)

```
Phase 1 (Foundation)
├── HTP-001: Define canonical Go struct
├── HTP-002: Implement ToMap() method
└── HTP-003: TOML loading with merge semantics
        │
Phase 2 (Starlark Runtime)
├── HTP-004: Add go.starlark.net dependency
├── HTP-005: Implement Starlark VM wrapper
├── HTP-006: Implement henia helpers (get, ensure, map)
└── HTP-007: Script resolution (path > embedded)
        │
Phase 3 (Built-in Transforms)
├── HTP-008: Claude transform.star
├── HTP-009: OpenCode transform.star
├── HTP-010: opencode.json emission
└── HTP-011: Templated reference output
        │
Phase 4 (Integration)
├── HTP-012: Wire Starlark into Transform()
├── HTP-013: Strict mode validation
└── HTP-014: Deprecate old mapping logic
        │
Phase 5 (Testing)
├── HTP-015: Unit tests for canonical struct
├── HTP-016: Unit tests for Starlark runtime
├── HTP-017: Integration tests
└── HTP-018: Golden tests for Claude/OpenCode
```

## Non-Goals

- CUE-based schema transformation (rejected: overkill for fixed schemas)
- Custom expression languages beyond Starlark
- Breaking existing TOML config structure
- Remote script loading (security concern)
