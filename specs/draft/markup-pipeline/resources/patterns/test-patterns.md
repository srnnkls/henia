# Test Patterns

## CUE Extraction Tests

```go
// internal/preprocess/cue_test.go

func TestExtractCUEBlocks(t *testing.T) {
    tests := []struct {
        name          string
        input         string
        wantCUE       string
        wantCleaned   string
        wantErr       bool
    }{
        {
            name: "single block",
            input: `# Header
%cue {{
env: "prod"
}}
Body content`,
            wantCUE: `env: "prod"
`,
            wantCleaned: `# Header
Body content`,
        },
        {
            name: "multiple blocks",
            input: `%cue {{
env: "prod"
}}
Middle content
%cue {{
enabled: true
}}`,
            wantCUE: `env: "prod"
enabled: true
`,
            wantCleaned: `Middle content
`,
        },
        {
            name: "empty block",
            input: `%cue {{
}}`,
            wantCUE: `
`,
            wantCleaned: ``,
        },
        {
            name: "no blocks",
            input: `Just regular markdown`,
            wantCUE: ``,
            wantCleaned: `Just regular markdown`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            gotCUE, gotCleaned, err := ExtractCUEBlocks(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ExtractCUEBlocks() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if gotCUE != tt.wantCUE {
                t.Errorf("ExtractCUEBlocks() gotCUE = %q, want %q", gotCUE, tt.wantCUE)
            }
            if gotCleaned != tt.wantCleaned {
                t.Errorf("ExtractCUEBlocks() gotCleaned = %q, want %q", gotCleaned, tt.wantCleaned)
            }
        })
    }
}
```

## CUE Evaluation Tests

```go
// internal/preprocess/cue_test.go

func TestEvaluateCUE(t *testing.T) {
    tests := []struct {
        name        string
        cueSource   string
        frontmatter map[string]any
        config      map[string]any
        want        map[string]any
        wantErr     bool
    }{
        {
            name: "simple computation",
            cueSource: `
env: "dev" | "prod"
enabled: bool | *true
`,
            frontmatter: map[string]any{
                "env": "prod",
            },
            config: map[string]any{},
            want: map[string]any{
                "env":     "prod",
                "enabled": true,
            },
        },
        {
            name: "access config",
            cueSource: `
format: config.harness.format
`,
            frontmatter: map[string]any{},
            config: map[string]any{
                "harness": map[string]any{
                    "format": "xml",
                },
            },
            want: map[string]any{
                "format": "xml",
            },
        },
        {
            name: "unification conflict",
            cueSource: `
env: "staging"
`,
            frontmatter: map[string]any{
                "env": "prod",
            },
            config:  map[string]any{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := EvaluateCUE(tt.cueSource, tt.frontmatter, tt.config)
            if (err != nil) != tt.wantErr {
                t.Errorf("EvaluateCUE() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
                t.Errorf("EvaluateCUE() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Block Parser Tests

```go
// internal/fenceddiv/parser_test.go

func TestBlockParser(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantName string
        wantAttrs map[string]string
    }{
        {
            name: "simple block",
            input: `:::instruction
Write tests first
:::`,
            wantName: "instruction",
            wantAttrs: map[string]string{},
        },
        {
            name: "block with attributes",
            input: `:::instruction{priority="critical"}
Always use TDD
:::`,
            wantName: "instruction",
            wantAttrs: map[string]string{
                "priority": "critical",
            },
        },
        {
            name: "multiple attributes",
            input: `:::note{level="warning" id="deprecated"}
This will be removed
:::`,
            wantName: "note",
            wantAttrs: map[string]string{
                "level": "warning",
                "id":    "deprecated",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            md := goldmark.New(
                goldmark.WithExtensions(fenceddiv.New()),
            )
            doc := md.Parser().Parse(text.NewReader([]byte(tt.input)))

            // Find FencedDivNode and verify
            var found *fenceddiv.FencedDivNode
            ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
                if entering {
                    if div, ok := n.(*fenceddiv.FencedDivNode); ok {
                        found = div
                        return ast.WalkStop, nil
                    }
                }
                return ast.WalkContinue, nil
            })

            if found == nil {
                t.Fatal("FencedDivNode not found")
            }
            if found.Name != tt.wantName {
                t.Errorf("Name = %q, want %q", found.Name, tt.wantName)
            }
            if !reflect.DeepEqual(found.Attributes, tt.wantAttrs) {
                t.Errorf("Attributes = %v, want %v", found.Attributes, tt.wantAttrs)
            }
        })
    }
}
```

## Inline Parser Tests

```go
// internal/fenceddiv/parser_test.go

func TestInlineParser(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantName string
        wantContent string
        wantAttrs map[string]string
    }{
        {
            name: "simple inline",
            input: `Use :term[time to live] for cache.`,
            wantName: "term",
            wantContent: "time to live",
            wantAttrs: map[string]string{},
        },
        {
            name: "inline with attributes",
            input: `Apply :instruction[this pattern]{priority="high"} consistently.`,
            wantName: "instruction",
            wantContent: "this pattern",
            wantAttrs: map[string]string{
                "priority": "high",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            md := goldmark.New(
                goldmark.WithExtensions(fenceddiv.New()),
            )
            doc := md.Parser().Parse(text.NewReader([]byte(tt.input)))

            // Find InlineDirectiveNode and verify
            var found *fenceddiv.InlineDirectiveNode
            ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
                if entering {
                    if dir, ok := n.(*fenceddiv.InlineDirectiveNode); ok {
                        found = dir
                        return ast.WalkStop, nil
                    }
                }
                return ast.WalkContinue, nil
            })

            if found == nil {
                t.Fatal("InlineDirectiveNode not found")
            }
            if found.Name != tt.wantName {
                t.Errorf("Name = %q, want %q", found.Name, tt.wantName)
            }
            if string(found.Content) != tt.wantContent {
                t.Errorf("Content = %q, want %q", string(found.Content), tt.wantContent)
            }
            if !reflect.DeepEqual(found.Attributes, tt.wantAttrs) {
                t.Errorf("Attributes = %v, want %v", found.Attributes, tt.wantAttrs)
            }
        })
    }
}
```

## Renderer Tests

```go
// internal/fenceddiv/renderer_test.go

func TestXMLRenderer(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {
            name: "block to XML",
            input: `:::instruction{priority="critical"}
Write tests first
:::`,
            want: `<instruction priority="critical">
Write tests first
</instruction>`,
        },
        {
            name: "inline to XML",
            input: `Use :term[TTL]{abbr="time to live"} here.`,
            want: `Use <term abbr="time to live">TTL</term> here.`,
        },
        {
            name: "attribute escaping",
            input: `:::note{msg="Use \"quotes\" carefully"}
Content
:::`,
            want: `<note msg="Use &quot;quotes&quot; carefully">
Content
</note>`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            md := goldmark.New(
                goldmark.WithExtensions(fenceddiv.New()),
                goldmark.WithRenderer(renderer.NewRenderer(
                    renderer.WithNodeRenderers(
                        util.Prioritized(fenceddiv.NewXMLRenderer(), 100),
                    ),
                )),
            )

            var buf bytes.Buffer
            if err := md.Convert([]byte(tt.input), &buf); err != nil {
                t.Fatal(err)
            }

            got := strings.TrimSpace(buf.String())
            want := strings.TrimSpace(tt.want)
            if got != want {
                t.Errorf("XML rendering:\ngot:  %q\nwant: %q", got, want)
            }
        })
    }
}

func TestPassthroughRenderer(t *testing.T) {
    tests := []struct {
        name  string
        input string
    }{
        {
            name: "block passthrough",
            input: `:::instruction{priority="critical"}
Write tests first
:::`,
        },
        {
            name: "inline passthrough",
            input: `Use :term[TTL]{abbr="time to live"} here.`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            md := goldmark.New(
                goldmark.WithExtensions(fenceddiv.New()),
                goldmark.WithRenderer(renderer.NewRenderer(
                    renderer.WithNodeRenderers(
                        util.Prioritized(fenceddiv.NewPassthroughRenderer(), 100),
                    ),
                )),
            )

            var buf bytes.Buffer
            if err := md.Convert([]byte(tt.input), &buf); err != nil {
                t.Fatal(err)
            }

            got := strings.TrimSpace(buf.String())
            want := strings.TrimSpace(tt.input)
            if got != want {
                t.Errorf("Passthrough rendering:\ngot:  %q\nwant: %q", got, want)
            }
        })
    }
}
```

## Integration Test Pattern

```go
// internal/fenceddiv/integration_test.go

func TestFullPipeline(t *testing.T) {
    input := `---
name: test-skill
enabled: true
---

# Test Skill

%cue {{
env: "dev" | "prod"
has_unsafe: or([for t in tools { !t.safe }])
}}

{{ if .enabled }}
:::instruction{priority="critical"}
Always write tests first
:::
{{ end }}

Use :term[TDD]{abbr="Test-Driven Development"} consistently.
`

    // Test with XML format
    t.Run("XML output", func(t *testing.T) {
        transformer := &transform.Transformer{
            Variables: map[string]string{},
            OutputFormat: "xml",
            GlobalConfig: map[string]any{
                "harness": map[string]any{
                    "format": "xml",
                },
            },
        }

        artifact := &artifact.Artifact{
            Body: input,
            Frontmatter: map[string]any{
                "enabled": true,
            },
        }

        result, err := transformer.Transform(artifact)
        if err != nil {
            t.Fatal(err)
        }

        // Verify CUE was processed
        // Verify templates were executed
        // Verify directives rendered to XML
        if !strings.Contains(result.Body, `<instruction priority="critical">`) {
            t.Error("Expected XML instruction tag")
        }
        if !strings.Contains(result.Body, `<term abbr="Test-Driven Development">TDD</term>`) {
            t.Error("Expected XML term tag")
        }
    })

    // Test with directives format
    t.Run("Directives passthrough", func(t *testing.T) {
        transformer := &transform.Transformer{
            Variables: map[string]string{},
            OutputFormat: "directives",
        }

        artifact := &artifact.Artifact{
            Body: input,
            Frontmatter: map[string]any{
                "enabled": true,
            },
        }

        result, err := transformer.Transform(artifact)
        if err != nil {
            t.Fatal(err)
        }

        // Verify directives passed through unchanged
        if !strings.Contains(result.Body, `:::instruction{priority="critical"}`) {
            t.Error("Expected passthrough instruction directive")
        }
        if !strings.Contains(result.Body, `:term[TDD]{abbr="Test-Driven Development"}`) {
            t.Error("Expected passthrough term directive")
        }
    })
}
```

## Edge Case Tests

```go
// internal/fenceddiv/edge_cases_test.go

func TestEdgeCases(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {
            name: "nested fenced divs",
            input: `:::outer
:::inner
Content
:::
:::`,
            wantErr: false,
        },
        {
            name: "escaped syntax",
            input: `\:::not-a-directive`,
            wantErr: false,
        },
        {
            name: "malformed attributes",
            input: `:::directive{attr=}`,
            wantErr: true,
        },
        {
            name: "empty block",
            input: `:::
:::`,
            wantErr: false,
        },
        {
            name: "special chars in attrs",
            input: `:::note{id="foo-bar_123" class="my-class"}`,
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            md := goldmark.New(
                goldmark.WithExtensions(fenceddiv.New()),
            )

            _, err := md.Parser().Parse(text.NewReader([]byte(tt.input)))
            if (err != nil) != tt.wantErr {
                t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```
