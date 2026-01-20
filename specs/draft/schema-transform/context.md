# Context: harness-transform

---

## Native Plan

**Source:** `~/.claude/plans/quirky-mapping-sunset.md`

- **Goal:** Replace hardcoded transformation logic with pluggable pipeline
- **Approach:** TOML config + Starlark scripts (pivoted from CUE)
- **Open questions resolved:** CUE rejected as overkill; Starlark chosen for familiarity

---

## Key Files

### Configuration

| File | Lines | Description |
|------|-------|-------------|
| `config.go` | 18-31 | `Harness` struct - add `Strict bool` field (HTP-001) |
| `internal/config/config.go` | all | TOML loading - add merge semantics |

### Transformation

| File | Lines | Description |
|------|-------|-------------|
| `internal/transform/transform.go` | 17-24 | `Transformer` struct - add Starlark runner |
| `internal/transform/transform.go` | 90-131 | `Transform()` method - integrate Starlark |

### New Packages

| Package | Purpose |
|---------|---------|
| `internal/canonical/` | Canonical frontmatter struct + ToMap() |
| `internal/starlark/` | VM wrapper, lib helpers, script loading |
| `internal/harness/` | Harness-specific logic, script resolution |

---

## Architecture Decisions

### AD-1: Starlark over CUE

**Context:** Original proposal used CUE for schema transformation.

**Decision:** Use Starlark instead. CUE is overkill for the actual problem.

**Rationale:**
- OpenCode and Claude skill frontmatter are fixed, tiny schemas (~5 fields)
- CUE's power (arbitrary schema transformation) isn't needed
- Starlark is Python-like syntax, lower learning curve
- Better error messages from Go validation than CUE

**Alternatives rejected:**
- CUE: Too complex for deterministic field mapping
- Pure Go: Not user-extensible without recompilation
- Lua: Less familiar syntax than Python

### AD-2: TOML for Declarative Config

**Context:** Need user-customizable tool mappings, variables, references.

**Decision:** Keep existing TOML structure. Starlark receives config as `ctx` dict.

**Rationale:**
- TOML already handles 90% of customization (variables, tools, keys, references)
- No new config format to learn
- Merge semantics (project > user > embedded) enable overrides

### AD-3: Script Resolution Order

**Context:** Users need to override embedded transforms at project or user level.

**Decision:** 3-level resolution order (first match wins):
1. Project: `.henia/harnesses/{name}/transform.star`
2. User: `~/.config/henia/harnesses/{name}/transform.star`
3. Embedded: `harnesses/{name}/transform.star`

**Rationale:**
- Project-level enables repo-specific customization
- User-level enables personal defaults across projects
- Embedded ensures everything works out-of-box
- Matches config merge precedence (project > user > embedded)

### AD-4: Separation of Concerns

**Context:** Need clear boundaries between Starlark and Go responsibilities.

**Decision:**
- **Starlark decides "what":** which fields, which tools, which files, warnings
- **Go decides "how":** render YAML/JSON, apply templates, validate, write files

**Rationale:**
- Starlark stays simple (data manipulation only)
- Go handles I/O and security-sensitive operations
- Easier to test each layer independently

### AD-5: Cargo-style Merge Semantics

**Context:** Need predictable config merging across project/user/embedded levels.

**Decision:** Use Cargo-style merge semantics:
- Scalars: most specific wins
- Arrays: concatenated, higher-precedence items later
- Tables: deep merge with scalar override on collision

**Rationale:**
- Well-documented precedent (Cargo, uv, mise use similar patterns)
- Predictable behavior for users familiar with Rust tooling
- Array concatenation enables additive customization

### AD-6: Fail-Fast Error Handling

**Context:** Need clear error handling policy for Starlark scripts.

**Decision:** Fail-fast on errors. Script errors abort transformation immediately.

**Rationale:**
- Silent failures lead to subtle bugs
- Explicit errors easier to debug than partial outputs
- Warnings still collected for non-fatal issues

### AD-7: Strict Hermetic Sandbox

**Context:** User scripts must not compromise system security.

**Decision:** Strict sandbox with no filesystem or network access, execution limits (10k steps default, 1k recursion depth). Step limit is configurable globally via `[starlark].max_steps`.

**Rationale:**
- User scripts run on potentially untrusted input
- No legitimate use case for file/network access in transforms
- Limits prevent DoS from infinite loops or memory exhaustion
- Global configurability allows power users to increase limit for complex transforms without per-harness complexity

### AD-8: Multi-Level Harness Discovery

**Context:** Users need to override embedded harnesses at project or user level.

**Decision:** Resolution order: project > user > embedded, first match wins.

**Rationale:**
- Project-level enables repo-specific customization
- User-level enables personal defaults across projects
- Embedded ensures everything works out-of-box

---

## Constraints

### Technical

- Starlark scripts must be hermetic (no file I/O, network)
- Transform must complete in <50ms for typical frontmatter
- `henia` helpers must cover common operations (no raw Python imports)
- Sandbox limits: 10k steps (configurable globally), 1k recursion depth
- Minimal lib API: json, re (no template() helper)
- File emission paths validated (no traversal, must be relative)

### Business

- Zero breaking changes to existing TOML configs
- Existing hardcoded transforms must continue working during migration
- New harnesses addable without Go changes

---

## Data Model

### Canonical Frontmatter

```go
type Canonical struct {
    Name          string            `yaml:"name"`
    Description   string            `yaml:"description,omitempty"`
    ModelTier     *string           `yaml:"model_tier,omitempty"`
    Tools         []string          `yaml:"tools,omitempty"`
    ToolsPolicy   map[string]string `yaml:"tools_policy,omitempty"`
    Enabled       *bool             `yaml:"enabled,omitempty"`
    UserInvocable *bool             `yaml:"user_invocable,omitempty"`
    Extra         map[string]any    `yaml:",inline"` // Unknown fields
}

func (c *Canonical) ToMap() map[string]any { ... }
```

### Context Dict (passed to Starlark)

```python
ctx = {
    "name": "claude",
    "path": "/home/user/.claude",
    "artifacts": ["skills", "commands"],
    "strict": True,
    "variables": {
        "model_strong": "claude-opus-4-5-20251101",
        "model_weak": "claude-3-5-haiku-latest",
    },
    "tools": {
        "bash": "Bash",
        "read": "Read",
    },
    "keys": {
        "allowed_tools": "tools",
    },
    "references": {
        "skill": {"output": "/{{.Name}}"},
    },
}
```

### Transform Output

```python
{
    "frontmatter": {
        "name": "my-skill",
        "model": "claude-opus-4-5-20251101",
        "tools": ["Bash", "Read"],
    },
    "files": {
        "opencode.json": {"permission": {"bash": "allow"}},
    },
    "warnings": ["Unknown tool 'custom' ignored"],
}
```

---

## Starlark Modules

| Module | Source | Purpose |
|--------|--------|---------|
| `json` | [go.starlark.net/lib/json](https://pkg.go.dev/go.starlark.net/lib/json) | `encode`, `decode`, `indent` |
| `re` | [github.com/magnetde/starlark-re](https://github.com/magnetde/starlark-re) | Python-compatible regex |

## Henia API

`henia` is a pre-loaded global available in all transform scripts.

| Namespace | Function | Behavior |
|-----------|----------|----------|
| `henia.dict` | `get_path(d, "a.b.c", default=None)` | Returns `default` for missing paths; does not raise |

**Built-in (no module needed):** `dict.get()`, `fail()`, list comprehensions, `hasattr()`, `getattr()`

---

## Gotchas & Learnings

- Starlark dicts are immutable by default; use `dict()` constructor for mutable copies
- `go.starlark.net` provides good error messages with line numbers
- TOML merge needs careful handling of `nil` vs empty values

---

## Resolved Questions

- [x] **template() helper:** No. Keep Starlark pure for transforms. Templates stay in `.tmpl` files.
- [x] **Error handling:** Fail-fast. Script errors abort transformation immediately.

---

## Future Considerations

- `transformer_path` config field for explicit script location
- Starlark module system for shared utility functions
- Schema validation mode using Starlark assertions
