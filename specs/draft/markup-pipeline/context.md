# Implementation Context

## Native Plan

Source: `~/.claude/plans/markup-pipeline.md`

The plan establishes a two-layer architecture:
- Go templates for control flow (existing)
- Goldmark for semantic markup parsing and rendering

Note: Harness-specific frontmatter transforms are handled separately by the [Harness Transform Pipeline](../schema-transform/spec.md) using TOML + Expr (updated 2026-09-07).

The key architectural insight: Henia should not invent new languages. Instead, it normalizes author-friendly syntax and delegates semantics to proven engines.

## Current Architecture

Henia processes markdown artifacts through a pipeline:
```
DISCOVER → PARSE → FILTER → TRANSFORM → RENDER → WRITE
```

The TRANSFORM phase currently handles:
1. YAML frontmatter parsing
2. Go template execution with Variables
3. Frontmatter key/value mapping
4. Backtick reference transformation

## Key Files

### Core Transform Pipeline
- `internal/transform/transform.go:90-131` - Main transformation logic
  - Processes frontmatter (lines 101-111)
  - Executes Go templates (lines 121-124)
  - Transforms references (line 126)
  - Returns transformed artifact

### Configuration
- `config.go:18-31` - Harness configuration struct
  - Variables, Keys, Values for template/frontmatter processing
  - References and Tools for backtick transformation
  - Missing: Format field for output format selection

### Artifact Structure
- `internal/artifact/artifact.go:22-31` - Artifact representation
  - Frontmatter (map[string]any)
  - Body (string)
  - Resources ([]string) for directory-based artifacts
  - Type (skill/command/agent)

### Reference Transformation
- `internal/reference/reference.go:31-73` - Backtick reference parsing
  - Pattern: `backtickPattern = regexp.MustCompile("`([^`]+)`")`
  - Supports sigils: $ (skill), / (command), @ (agent), # (file), ! (tool)
  - Applied after template execution

What are "References"?

References are an existing Henia feature for transforming backtick-wrapped sigils into target-specific output. Examples:
- `` `$skill-name` `` → `[skill-name](#skill-name)` (skill reference)
- `` `/command-name` `` → `/command-name` (command reference)
- `` `@agent-name` `` → `@agent-name` (agent reference)
- `` `!tool-name` `` → `kubectl` (tool mapping from config)

In the new pipeline, reference transformation runs after goldmark rendering, ensuring directives are already transformed before reference replacement occurs.

## Architectural Decisions

### 1. Layer Separation
Each layer has a single responsibility and uses a specialized engine:
- Control flow layer: Go templates handle conditionals, loops, composition
- Semantic markup layer: Goldmark handles parsing, AST, rendering

No overlap. No custom languages.

### 2. Processing Order
```
Go templates (control) → Goldmark (markup) → References (transform)
```

This order ensures:
- Templates can conditionally include/exclude directives
- Directives render after template expansion
- References transform the final output

### 3. Output Format Model
Simple string-based format selection:
- `"directives"` (default) - pass through unchanged
- `"xml"` - transform to XML tags
- Future: `"json"`, `"yaml"`, `"html"`

No complex renderer configuration. Just a format string per harness.

### 4. Goldmark Extension Strategy
Use goldmark's official extension API:
- `parser.WithBlockParsers()` for block fenced divs
- `parser.WithInlineParsers()` for inline directives
- `renderer.WithNodeRenderers()` for format-specific rendering

No regex parsing. No custom markdown implementation.

Markdown Output Strategy:

Goldmark renders to HTML by default. To produce markdown output with transformed directives:

1. Parse with custom extension - Use goldmark parser with fenced div extension to build AST
2. Walk AST manually - Traverse AST nodes and build output string
3. Custom TextRenderer - Implement goldmark renderer that outputs markdown text instead of HTML
   - Standard markdown nodes (paragraphs, headers, lists) → markdown syntax
   - Directive nodes → XML tags (format="xml") or directive syntax (format="directives")
   - Use goldmark's NodeRenderer interface for each node type

Implementation approach:
- Create `MarkdownRenderer` implementing `renderer.Renderer`
- Register node renderers for all standard goldmark nodes (paragraph, heading, list, etc.)
- Each renderer outputs markdown text, not HTML
- Directive renderers output based on format (XML vs passthrough)

Inline parser trigger requirement:
- Inline directive parser uses `:` as trigger character
- Must register trigger: `parser.WithInlineParsers(util.Prioritized(NewInlineParser(), 100))`
- Inline parser checks for `:name[content]{attrs}` pattern when triggered

## Integration Points

### Transform Pipeline Enhancement
Current `transform.go:90-131` needs:
1. Goldmark rendering after template execution
   - Parse with fenced div extension
   - Render based on Format field
2. Keep reference transformation after goldmark

### Configuration Extension
`config.go:18-31` Harness struct needs:
- Add `Format string` field at line 22 (after Structure)
- Default to "directives" if empty

### Transformer Enhancement
`internal/transform/transform.go:17-24` needs:
- Add `OutputFormat string` field

## Data Model

### AST Node Types
```go
// Block fenced div: :::name{attrs}
type FencedDivNode struct {
    ast.BaseBlock
    Name       string
    Attributes map[string]string
}

// Inline directive: :name[content]{attrs}
type InlineDirectiveNode struct {
    ast.BaseInline
    Name       string
    Content    []byte
    Attributes map[string]string
}
```

Simple. Just name, content (inline only), and attributes.

## Edge Cases

### Goldmark Edge Cases
- Nested fenced divs - Goldmark handles correctly
- Escaped syntax `\:::` - Goldmark escaping rules apply
- Malformed attributes `{attr=}` - Parse error with position
- Empty blocks `:::\n:::` - Valid, renders as empty
- Special chars in attrs `{id="foo-bar_123"}` - Properly escaped

### Template + Directive Interaction
- Template conditionally includes directive - Works correctly
- Template generates directive syntax - Rendered by goldmark
- Directive contains template syntax - Already expanded before goldmark

## Testing Strategy

### Unit Tests
- `internal/fenceddiv/parser_test.go` - Block and inline parsing
- `internal/fenceddiv/renderer_test.go` - XML and pass-through rendering

### Integration Tests
- `internal/fenceddiv/integration_test.go` - Full pipeline with all layers
- Test fixtures in `internal/fenceddiv/testdata/`

### End-to-End Tests
- Real skill files with Go templates, CUE blocks, and directives
- Verify XML output for Claude harness
- Verify pass-through for default harness

## Migration Notes

Backwards compatibility guaranteed:
- Configs without `Format` field work unchanged (default to pass-through)
- Existing Variable-based templates continue working
- Artifacts without directives render unchanged

Gradual adoption path:
1. Add `format = "xml"` to specific harnesses
2. Migrate to fenced divs incrementally
3. Keep existing artifacts unchanged until ready

## Dependencies

### New Dependencies
- `github.com/yuin/goldmark` - Markdown parser (used by Hugo, Gitea, etc.)

### Why This Dependency
- Goldmark: Standard Go markdown parser, proper AST, extension API, actively maintained

No NIH syndrome. Use proven tools.
