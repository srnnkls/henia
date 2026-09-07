---
issue_type: Feature
created: 2026-01-15
updated: 2026-09-07
status: Implemented
stage: draft
---

# Templates and semantic markup

## Goal and boundary

Compile canonical Markdown artifacts through Go templates, Goldmark directives
and reference rewriting. Henia consumes local sources and emits artifacts for
Phora to distribute. Fetching, installation, deployment state and pruning belong
to Phora and are outside this scope.

The [harness transform scope](../schema-transform/spec.md) defines canonical
metadata, TOML + Expr profiles, linting and the build output configuration.
The previous CUE preprocessing sketches and proposed fenceddiv package APIs are
superseded by the implemented contracts referenced below.

## Pipeline

1. Parse YAML frontmatter and retain typed values.
2. Execute Go templates using frontmatter, `henia.variables`, then harness
   variables. Support conditionals, ranges, interpolation and named templates.
   Recursively template nested metadata strings.
3. Apply metadata mappings and the selected vendor profile.
4. Parse the body with the Goldmark directive extension and render its nodes.
5. Rewrite artifact and tool references in the rendered body.
6. Preflight outputs and write under `<build-output>/<harness>/...`.

No template, directive or Expr rule executes a deployment action. Templates use
Go's text/template engine; metadata computations and custom lint rules use Expr.
Neither CUE nor Starlark is required or executed.

## Directive syntax

Container blocks and inline directives support nesting:

```markdown
:::instruction{priority="critical"}
Write tests before changing the implementation.
:::

Use :term[TTL]{abbr="time to live"} for caching.
```

Goldmark's attribute parser handles quoted and unquoted values, IDs, classes and
spacing. XML rendering escapes attribute values. The extension retains source
spans so ordinary Markdown remains unchanged; literal code and escaped directive
syntax are not interpreted as directives. Malformed directives return source
line and column information.

The syntax follows the generic-directive proposal, an extension to CommonMark.
Two-colon leaf directives remain deferred.

## Rendering

Rendering is independent of vendor metadata profiles:

- `xml`: replace directive nodes with XML tags and retain ordinary Markdown.
- `directives`: preserve directive syntax while normalizing attributes and spacing.
- `markdown`: render directive names and attributes as Markdown labels.

Claude defaults to XML as a Henia convention. Other profiles may also choose XML,
and Claude may retain directives. The directives renderer is not byte-for-byte
identity for directive syntax; ordinary Markdown source spans are preserved.

```toml
[build]
output = ".henia/build"

[harness.claude]
profile = "claude"
format = "xml"

[harness.codex]
profile = "codex"
format = "directives"
```

Relative configured build output resolves against the declaring configuration
file. `henia build --output` overrides it using the invocation directory. Final
installation paths are configured in Phora.

## Acceptance criteria

- Nested container and inline directives parse and render correctly.
- Attributes escape correctly in XML and normalize consistently in directives.
- Code fences, inline code and escaped syntax remain literal.
- Ordinary Markdown formatting is retained outside directive replacements.
- Templates run before body markup; references rewrite afterwards.
- Template composition and nested metadata retain their types and source inputs.
- Build errors surface before any planned writes; output paths cannot overwrite
  canonical inputs or escape the selected output roots through symlinks.
- Multiple harness variants build from one local canonical tree without fetching
  sources or requiring an installed harness.
- Typical 10 KB skill transformation targets less than 100 ms with warm code/data,
  excluding disk I/O; benchmarks live alongside the implementation.

## Implementation and validation

- [Implementation map](resources/implementation.md)
- [Test coverage](resources/patterns/test-patterns.md)
- [Phora integration phases](../../../docs/phora.md)
