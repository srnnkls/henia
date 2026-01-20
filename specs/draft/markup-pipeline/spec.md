---
issue_type: Feature
created: 2026-01-15
status: Draft
stage: draft
claude_plan: ~/.claude/plans/markup-pipeline.md
---

# Two-Layer Processing: Templates and Semantic Markup

## Goal

Add semantic markup processing to Henia using Goldmark, complementing the existing Go template layer:
- **Control-flow layer** (Go templates) - if/range/interpolation (existing)
- **Semantic markup layer** (Goldmark extensions) - block/inline directives with format-aware rendering

**Key principle:** Henia normalizes author-friendly syntax and delegates semantics to existing engines. No custom parsers.

## Scope

### In Scope
- Block fenced divs (`:::name{attrs}`) via goldmark parser
- Inline directives (`:name[content]{attrs}`) via goldmark parser
- XML renderer for Claude harness (both block and inline)
- Pass-through renderer for default/directives format
- Format configuration per harness
- Pipeline integration (templates → directives → references)

### Out of Scope
- Inline computation blocks (`%cue`, `%starlark`) - handled by harness transforms
- Directive execution (they're purely semantic markup)
- JSON/YAML output formats (future extension)
- Custom attribute transformations per harness
- HTML output format

## Requirements

### Control Flow Layer (Go Templates)

**Given** markdown with frontmatter
**When** template execution runs
**Then** it should:
- Use frontmatter values as template context
- Execute existing Go template syntax (if/range/template)
- Maintain backwards compatibility with existing Variable-based templates

### Semantic Markup Layer (Goldmark)

**Attribute Syntax Grammar:**

Attributes follow CommonMark directive syntax (https://talk.commonmark.org/t/generic-directives-plugins-syntax/444):
```
{key=value key2="quoted value" #id .class}
```

**Rules (delegated to goldmark):**
- Attribute values may be unquoted (if no spaces) or quoted (if spaces/special chars)
- Escape quotes inside values: `{msg="Use \"quotes\" here"}`
- Special syntax: `#id` for IDs, `.class` for classes
- Multiple attributes separated by whitespace
- Empty attributes block `{}` is valid (no attributes)
- Spacing between directive name and attributes is allowed

**Examples:**
- Valid: `{priority=critical}`, `{level="warning" id=deprecated}`, `{#main .highlight}`, `{}`
- Invalid: `{attr=}` (empty value), `{key=value with spaces}` (unquoted with spaces)

**Note:** Attribute parsing and special syntax (`#id`, `.class`) are handled by goldmark's attribute parser, not custom code.

**Block Directives:**

CommonMark directive syntax supports two types:
- **Container blocks** (3+ colons): `::: name [content] {attrs}` - contains nested content
- **Leaf blocks** (2 colons): `:: name [content] {attrs}` - standalone block

**Given** markdown with container block directives `:::directive{attr=value}`
**When** goldmark parser runs
**Then** it should:
- Parse directive name and attributes correctly
- Handle multi-line content within blocks
- Support nested fenced divs
- Build proper AST nodes

**Note:** Leaf block directives (`::`) are part of the CommonMark standard but implementation priority can be deferred. Goldmark may handle them automatically.

**Inline Directives:**

**Given** markdown with `:name[content]{attr="value"}` syntax
**When** goldmark parser runs
**Then** it should:
- Parse directive name, content, and attributes
- Handle inline placement within paragraphs
- Build proper AST nodes

**XML Rendering:**

**Given** AST with directive nodes and format="xml"
**When** XML renderer runs
**Then** it should:
- Transform block fenced divs to `<name attr="value">content</name>`
- Transform inline directives to `<name attr="value">content</name>`
- Escape attribute values properly
- Preserve nested structure

**XML Output Contract:**
- **Directives only:** Only directive nodes (block/inline) convert to XML
- **Standard markdown:** Paragraphs, headers, lists, code blocks pass through as markdown
- **Mixed output:** Result contains XML tags for directives embedded in markdown text

**Example:**
```markdown
# Header

:::instruction{priority="critical"}
Always use TDD
:::

Use :term[TTL]{abbr="time to live"} for caching.
```

**XML output:**
```markdown
# Header

<instruction priority="critical">
Always use TDD
</instruction>

Use <term abbr="time to live">TTL</term> for caching.
```

Standard markdown structure preserved; only directives transformed.

**Pass-through Rendering:**

**Given** AST with directive nodes and format="directives"
**When** pass-through renderer runs
**Then** it should:
- Output block fenced divs as `:::name{attrs}`
- Output inline directives as `:name[content]{attrs}`
- Re-serialize from AST (normalized formatting)

**Note:** Pass-through re-serializes from the parsed AST, not from original source bytes. This means:
- Attribute whitespace normalized: `{key = "val"}` → `{key="val"}`
- Directive formatting normalized
- This is an acceptable trade-off for simpler implementation

### Configuration

**Given** harness configuration in henia.toml
**When** transformer is initialized
**Then** it should:
- Default to "directives" format if not specified
- Support explicit "xml" format per harness

**Note:** Harness-specific frontmatter transforms (model_tier → model, tool mappings, etc.) are handled by the [Harness Transform Pipeline](../schema-transform/spec.md), not this spec.

## Acceptance Criteria

1. **Goldmark parsers work correctly**
   - Block parser handles `:::name{attr="val"}` syntax
   - Inline parser handles `:name[content]{attr="val"}` syntax
   - Both handle attributes with quotes
   - Parsing errors include line/column information

2. **XML rendering works correctly**
   - Block divs render as XML tags with attributes
   - Inline directives render as XML tags with content
   - Special characters in attributes are escaped
   - Nested content is preserved

3. **Pass-through rendering works correctly**
   - Block divs pass through unchanged
   - Inline directives pass through unchanged
   - Original formatting preserved

4. **Pipeline integration works correctly**
   - Templates run before goldmark
   - Reference transformation runs after goldmark
   - Each layer's output feeds the next

5. **Backwards compatibility maintained**
   - Existing configs without `format` field work unchanged
   - Existing Variable-based templates continue working
   - No breaking changes to artifact structure

6. **Performance is acceptable**
   - **Target:** Process typical skill file (~10KB) in <100ms
   - **Measurement:** Warm cache (after first invocation), excludes disk I/O
   - **Includes:** Template execution and goldmark rendering
   - Goldmark parsing scales linearly with content size

## Non-Goals

- We are **not** creating a custom control flow language
- We are **not** executing directives (they're semantic markup only)
- We are **not** supporting unquoted attribute values (always use quotes)
- We are **not** adding JSON/YAML output in this iteration

## Dependencies

- github.com/yuin/goldmark - Markdown parser with extension API

## Open Questions

None at this time. The plan is comprehensive and architecture is clear.
