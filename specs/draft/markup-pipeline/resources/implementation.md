# Implementation Patterns

Note: This file contains code sketches and patterns to follow. These are not tested or final implementations.

## CUE Block Extraction

```go
// internal/preprocess/cue.go

func ExtractCUEBlocks(source string) (cueSource string, cleanedSource string, error) {
    var cueBuilder strings.Builder
    var outputBuilder strings.Builder

    scanner := bufio.Scanner(strings.NewReader(source))
    inCUEBlock := false

    for scanner.Scan() {
        line := scanner.Text()

        if strings.HasPrefix(line, "%cue {{") {
            inCUEBlock = true
            continue
        }

        if inCUEBlock && strings.TrimSpace(line) == "}}" {
            inCUEBlock = false
            cueBuilder.WriteString("\n")
            continue
        }

        if inCUEBlock {
            cueBuilder.WriteString(line)
            cueBuilder.WriteString("\n")
        } else {
            outputBuilder.WriteString(line)
            outputBuilder.WriteString("\n")
        }
    }

    return cueBuilder.String(), outputBuilder.String(), scanner.Err()
}
```

Pattern: Line-oriented extraction using `bufio.Scanner` for idiomatic Go.

## CUE Evaluation

```go
// internal/preprocess/cue.go

func EvaluateCUE(cueSource string, frontmatter map[string]any, config map[string]any) (map[string]any, error) {
    ctx := cuecontext.New()

    // Start with empty value
    base := ctx.CompileString("{}")

    // Add frontmatter (top-level)
    for k, v := range frontmatter {
        base = base.FillPath(cue.ParsePath(k), v)
    }

    // Add config (namespaced)
    base = base.FillPath(cue.ParsePath("config"), config)

    // Unify with CUE source
    cueValue := ctx.CompileString(cueSource)
    result := base.Unify(cueValue)

    if err := result.Err(); err != nil {
        return nil, fmt.Errorf("CUE evaluation failed: %w", err)
    }

    // Decode to Go map
    var output map[string]any
    if err := result.Decode(&output); err != nil {
        return nil, fmt.Errorf("CUE decode failed: %w", err)
    }

    return output, nil
}
```

Pattern: Build base from frontmatter + config, unify with CUE source, decode to Go map.

## Transform Integration

```go
// internal/transform/transform.go

func (t *Transformer) Transform(art *artifact.Artifact) (*artifact.Artifact, error) {
    result := &artifact.Artifact{
        Name:        art.Name,
        Namespace:   art.Namespace,
        Type:        art.Type,
        SourcePath:  art.SourcePath,
        IsDirectory: art.IsDirectory,
        Resources:   art.Resources,
        Frontmatter: make(map[string]any),
    }

    // 1. Process frontmatter (existing)
    for k, v := range art.Frontmatter {
        if str, ok := v.(string); ok {
            transformed, err := ExecuteTemplate(str, t.Variables)
            if err != nil {
                return nil, fmt.Errorf("transform frontmatter %q: %w", k, err)
            }
            result.Frontmatter[k] = transformed
        } else {
            result.Frontmatter[k] = v
        }
    }

    result.Frontmatter = ApplyMappings(result.Frontmatter, t.Keys)
    result.Frontmatter = ApplyValueMappings(result.Frontmatter, t.Values)

    // 2. CUE preprocessing (NEW)
    cueSource, cleanedBody, err := preprocess.ExtractCUEBlocks(art.Body)
    if err != nil {
        return nil, err
    }

    var templateVars map[string]any
    if cueSource != "" {
        cueResult, err := preprocess.EvaluateCUE(cueSource, art.Frontmatter, t.GlobalConfig)
        if err != nil {
            return nil, fmt.Errorf("CUE evaluation: %w", err)
        }
        // CUE values override frontmatter keys (merge strategy)
        templateVars = cueResult
    } else {
        // No CUE blocks: use Variables map (backwards compatibility)
        // Convert map[string]string to map[string]any
        templateVars = make(map[string]any, len(t.Variables))
        for k, v := range t.Variables {
            templateVars[k] = v
        }
    }

    // 3. Execute Go templates (existing, enhanced)
    // NOTE: ExecuteTemplate signature changed to map[string]any
    body, err := ExecuteTemplate(cleanedBody, templateVars)
    if err != nil {
        return nil, fmt.Errorf("transform body: %w", err)
    }

    // 4. Parse and render fenced divs (NEW)
    body, err = t.renderFencedDivs(body)
    if err != nil {
        return nil, err
    }

    // 5. Transform references (existing)
    body = t.transformReferences(body)

    result.Body = body
    return result, nil
}
```

Pattern: Five-step pipeline where each layer feeds into the next.

## Goldmark Rendering

```go
// internal/transform/transform.go

func (t *Transformer) renderFencedDivs(body string) (string, error) {
    // Create goldmark parser with fenced div extension
    md := goldmark.New(
        goldmark.WithExtensions(fenceddiv.New()),
        goldmark.WithRenderer(t.getFencedDivRenderer()),
    )

    // Parse and render in one step
    var buf bytes.Buffer
    if err := md.Convert([]byte(body), &buf); err != nil {
        return "", err
    }

    return buf.String(), nil
}

func (t *Transformer) getFencedDivRenderer() renderer.Renderer {
    switch t.OutputFormat {
    case "xml":
        return renderer.NewRenderer(
            renderer.WithNodeRenderers(
                util.Prioritized(fenceddiv.NewXMLRenderer(), 100),
            ),
        )
    case "directives", "":
        return renderer.NewRenderer(
            renderer.WithNodeRenderers(
                util.Prioritized(fenceddiv.NewPassthroughRenderer(), 100),
            ),
        )
    default:
        return renderer.NewRenderer(
            renderer.WithNodeRenderers(
                util.Prioritized(fenceddiv.NewPassthroughRenderer(), 100),
            ),
        )
    }
}
```

Pattern: Create goldmark instance per transformation with format-specific renderer.

## Markdown Renderer (TextRenderer)

Problem: Goldmark renders to HTML by default. We need markdown output with transformed directives.

Solution: Implement custom renderer that outputs markdown text.

```go
// internal/fenceddiv/renderer_markdown.go

type MarkdownRenderer struct {
    DirectiveRenderer NodeRenderer  // XML or Passthrough
}

func NewMarkdownRenderer(format string) renderer.Renderer {
    var directiveRenderer NodeRenderer
    if format == "xml" {
        directiveRenderer = NewXMLRenderer()
    } else {
        directiveRenderer = NewPassthroughRenderer()
    }

    r := &MarkdownRenderer{
        DirectiveRenderer: directiveRenderer,
    }

    return renderer.NewRenderer(
        renderer.WithNodeRenderers(
            util.Prioritized(r, 500),
        ),
    )
}

func (r *MarkdownRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
    // Standard markdown nodes - output as markdown
    reg.Register(ast.KindDocument, r.renderDocument)
    reg.Register(ast.KindParagraph, r.renderParagraph)
    reg.Register(ast.KindHeading, r.renderHeading)
    reg.Register(ast.KindText, r.renderText)
    reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
    reg.Register(ast.KindList, r.renderList)

    // Directive nodes - delegate to format-specific renderer
    reg.Register(KindFencedDiv, r.DirectiveRenderer.renderFencedDiv)
    reg.Register(KindInlineDirective, r.DirectiveRenderer.renderInlineDirective)
}

func (r *MarkdownRenderer) renderParagraph(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    if entering {
        // Just render children, paragraph is implicit
    } else {
        w.WriteString("\n\n")
    }
    return ast.WalkContinue, nil
}

func (r *MarkdownRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    n := node.(*ast.Heading)
    if entering {
        for i := 0; i < n.Level; i++ {
            w.WriteString("#")
        }
        w.WriteString(" ")
    } else {
        w.WriteString("\n\n")
    }
    return ast.WalkContinue, nil
}

func (r *MarkdownRenderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    if entering {
        n := node.(*ast.Text)
        w.Write(n.Segment.Value(source))
    }
    return ast.WalkContinue, nil
}

// ... implement other node renderers for lists, code blocks, emphasis, etc.
```

Pattern:
- Create MarkdownRenderer that implements all standard goldmark node types
- Each renderer outputs markdown syntax, not HTML
- Directive nodes delegate to format-specific renderer (XML or Passthrough)
- Result: markdown with transformed directives embedded

Alternative (simpler but less extensible):
```go
// Walk AST manually without using goldmark's renderer
func (t *Transformer) renderFencedDivs(body string) (string, error) {
    md := goldmark.New(goldmark.WithExtensions(fenceddiv.New()))

    doc := md.Parser().Parse(text.NewReader([]byte(body)))

    var buf bytes.Buffer
    walkNode(doc, source, &buf, t.OutputFormat)

    return buf.String(), nil
}

func walkNode(node ast.Node, source []byte, w io.Writer, format string) {
    // Recursively walk and emit markdown or XML based on node type
}
```

## XML Renderer

```go
// internal/fenceddiv/renderer_xml.go

type XMLRenderer struct{}

func (r *XMLRenderer) renderFencedDiv(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    n := node.(*FencedDivNode)

    if entering {
        w.WriteString("<")
        w.WriteString(n.Name)

        for k, v := range n.Attributes {
            w.WriteString(" ")
            w.WriteString(k)
            w.WriteString("=\"")
            w.WriteString(html.EscapeString(v))
            w.WriteString("\"")
        }

        w.WriteString(">")
    } else {
        w.WriteString("</")
        w.WriteString(n.Name)
        w.WriteString(">")
    }

    return ast.WalkContinue, nil
}

func (r *XMLRenderer) renderInlineDirective(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    n := node.(*InlineDirectiveNode)

    if entering {
        w.WriteString("<")
        w.WriteString(n.Name)

        for k, v := range n.Attributes {
            w.WriteString(" ")
            w.WriteString(k)
            w.WriteString("=\"")
            w.WriteString(html.EscapeString(v))
            w.WriteString("\"")
        }

        w.WriteString(">")
        w.Write(n.Content)
        w.WriteString("</")
        w.WriteString(n.Name)
        w.WriteString(">")
    }

    return ast.WalkContinue, nil
}
```

Pattern: Transform AST nodes to XML tags with attribute escaping.

## Pass-through Renderer

```go
// internal/fenceddiv/renderer_passthrough.go

type PassthroughRenderer struct{}

func (r *PassthroughRenderer) renderFencedDiv(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    n := node.(*FencedDivNode)

    if entering {
        w.WriteString(":::")
        w.WriteString(n.Name)
        w.WriteString("{")

        var attrs []string
        for k, v := range n.Attributes {
            attrs = append(attrs, fmt.Sprintf("%s=\"%s\"", k, v))
        }
        w.WriteString(strings.Join(attrs, " "))

        w.WriteString("}\n")
    } else {
        w.WriteString(":::\n")
    }

    return ast.WalkContinue, nil
}

func (r *PassthroughRenderer) renderInlineDirective(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
    n := node.(*InlineDirectiveNode)

    if entering {
        w.WriteString(":")
        w.WriteString(n.Name)
        w.WriteString("[")
        w.Write(n.Content)
        w.WriteString("]{")

        var attrs []string
        for k, v := range n.Attributes {
            attrs = append(attrs, fmt.Sprintf("%s=\"%s\"", k, v))
        }
        w.WriteString(strings.Join(attrs, " "))

        w.WriteString("}")
    }

    return ast.WalkContinue, nil
}
```

Pattern: Reconstruct original directive syntax from AST nodes.

## Goldmark Extension

```go
// internal/fenceddiv/extension.go

type FencedDivExtension struct{}

func New() goldmark.Extender {
    return &FencedDivExtension{}
}

func (e *FencedDivExtension) Extend(md goldmark.Markdown) {
    md.Parser().AddOptions(
        parser.WithBlockParsers(
            util.Prioritized(NewBlockParser(), 100),
        ),
        parser.WithInlineParsers(
            util.Prioritized(NewInlineParser(), 100),
        ),
    )
    // Renderer added separately per output format
}
```

Pattern: Register both block and inline parsers, renderer selection external.
