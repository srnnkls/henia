# Implementation Guide: Harness Transform Pipeline

## Package Structure

```
internal/
  canonical/
    canonical.go       # Canonical struct + ToMap()
    canonical_test.go
  starlark/
    vm.go              # Starlark VM wrapper
    lib.go             # henia.dict helpers (get_path)
    resolve.go         # Script resolution logic
    starlark_test.go
  harness/
    opencode.go        # OpenCode-specific output (opencode.json)
    output.go          # Templated reference output
  config/
    merge.go           # TOML merge semantics
  transform/
    transform.go       # Main transform logic (updated)
    strict.go          # Strict mode validation
    legacy.go          # Backwards compat shim
harnesses/             # Embedded via go:embed
  claude/
    transform.star
  opencode/
    transform.star
```

## Core Types

### internal/canonical/canonical.go

```go
package canonical

// Canonical represents the normalized artifact frontmatter
type Canonical struct {
    Name        string         `yaml:"name"`
    Description string         `yaml:"description,omitempty"`
    ModelTier   *string        `yaml:"model_tier,omitempty"`
    Tools       []string       `yaml:"tools,omitempty"`
    Enabled     *bool          `yaml:"enabled,omitempty"`
    Extra       map[string]any `yaml:"-"` // Capture unknown fields
}

// ToMap converts canonical struct to Starlark-friendly dict
func (c *Canonical) ToMap() map[string]any {
    m := map[string]any{
        "name": c.Name,
    }
    if c.Description != "" {
        m["description"] = c.Description
    }
    if c.ModelTier != nil {
        m["model_tier"] = *c.ModelTier
    }
    if len(c.Tools) > 0 {
        m["tools"] = c.Tools
    }
    if c.Enabled != nil {
        m["enabled"] = *c.Enabled
    }
    // Merge extra fields
    for k, v := range c.Extra {
        m[k] = v
    }
    return m
}
```

### internal/starlark/vm.go

```go
package starlark

import (
    "go.starlark.net/starlark"
    "go.starlark.net/starlarkstruct"
)

type VM struct {
    thread  *starlark.Thread
    globals starlark.StringDict  // Pre-loaded globals including henia
}

func NewVM() *VM {
    thread := &starlark.Thread{Name: "transform"}
    globals := makeGlobals() // from lib.go - includes henia namespace
    return &VM{thread: thread, globals: globals}
}

func (vm *VM) Execute(script string, input, ctx map[string]any) (*Result, error) {
    // Parse and execute script with henia pre-loaded as global
    globals, err := starlark.ExecFile(vm.thread, "transform.star", script, vm.globals)
    if err != nil {
        return nil, err
    }

    // Call transform(input, ctx) - henia is available as global
    transformFn := globals["transform"]
    inputVal := mapToStarlark(input)
    ctxVal := mapToStarlark(ctx)

    result, err := starlark.Call(vm.thread, transformFn, starlark.Tuple{inputVal, ctxVal}, nil)
    if err != nil {
        return nil, err
    }

    return parseResult(result)
}

type Result struct {
    Frontmatter map[string]any
    Files       map[string]any
    Warnings    []string
}
```

### internal/starlark/lib.go

```go
package starlark

import (
    "go.starlark.net/starlark"
    "go.starlark.net/starlarkstruct"
)

// makeGlobals creates pre-loaded globals including the henia namespace
func makeGlobals() starlark.StringDict {
    // Build henia.dict namespace
    dictNs := starlark.StringDict{
        "get_path": starlark.NewBuiltin("get_path", heniaGetPath),
    }

    // Build henia namespace
    heniaNs := starlarkstruct.FromStringDict(starlark.String("henia"), starlark.StringDict{
        "dict": starlarkstruct.FromStringDict(starlark.String("dict"), dictNs),
    })

    return starlark.StringDict{
        "henia": heniaNs,
    }
}

// henia.dict.get_path(d, "a.b.c", default=None) - safe nested access
// Returns default (or None) for missing paths. Does not raise on missing keys.
func heniaGetPath(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    // Implementation...
}
```

### internal/starlark/resolve.go

```go
package starlark

import (
    "embed"
    "fmt"
    "os"
    "path/filepath"
)

//go:embed harnesses/**/transform.star
var embedded embed.FS

// Resolve finds the transform script for a harness
// Priority (first match wins):
//   1. Project: .henia/harnesses/{name}/transform.star
//   2. User: ~/.config/henia/harnesses/{name}/transform.star
//   3. Embedded: harnesses/{name}/transform.star
func Resolve(harnessName, projectRoot string) (string, error) {
    // 1. Check project-level override
    projectScript := filepath.Join(projectRoot, ".henia", "harnesses", harnessName, "transform.star")
    if data, err := os.ReadFile(projectScript); err == nil {
        return string(data), nil
    }

    // 2. Check user-level override
    home, _ := os.UserHomeDir()
    if home != "" {
        userScript := filepath.Join(home, ".config", "henia", "harnesses", harnessName, "transform.star")
        if data, err := os.ReadFile(userScript); err == nil {
            return string(data), nil
        }
    }

    // 3. Fall back to embedded
    embeddedPath := filepath.Join("harnesses", harnessName, "transform.star")
    data, err := embedded.ReadFile(embeddedPath)
    if err != nil {
        return "", fmt.Errorf("no transform script for harness %q", harnessName)
    }
    return string(data), nil
}
```

## Integration Points

### internal/transform/transform.go

Add Starlark runner to Transformer:

```go
type Transformer struct {
    Variables    map[string]string
    Keys         map[string]string
    Values       map[string]map[string]string
    Tools        map[string]string
    References   map[string]ReferenceConfig

    // NEW: Starlark integration
    HarnessName  string
    HarnessPath  string
    Strict       bool
    starlarkVM   *starlark.VM
}

func (t *Transformer) Transform(art *artifact.Artifact) (*artifact.Artifact, error) {
    // Try Starlark transform first
    script, err := starlark.Resolve(t.HarnessName, t.HarnessPath)
    if err == nil {
        return t.starlarkTransform(art, script)
    }

    // Fall back to legacy transform
    return t.legacyTransform(art)
}

func (t *Transformer) starlarkTransform(art *artifact.Artifact, script string) (*artifact.Artifact, error) {
    // Build input from canonical
    can := canonical.FromArtifact(art)
    input := can.ToMap()

    // Build ctx from harness config
    ctx := t.buildContext()

    // Execute Starlark
    result, err := t.starlarkVM.Execute(script, input, ctx)
    if err != nil {
        return nil, fmt.Errorf("starlark transform: %w", err)
    }

    // Strict mode validation
    if t.Strict {
        if err := t.validateOutput(result.Frontmatter); err != nil {
            return nil, err
        }
    }

    // Build output artifact
    return &artifact.Artifact{
        Name:        art.Name,
        Namespace:   art.Namespace,
        Type:        art.Type,
        SourcePath:  art.SourcePath,
        IsDirectory: art.IsDirectory,
        Resources:   art.Resources,
        Frontmatter: result.Frontmatter,
        Body:        art.Body,
        ExtraFiles:  result.Files,
    }, nil
}
```

## TOML Merge Semantics

### internal/config/merge.go

```go
package config

// MergeHarness merges harness configs with precedence:
// project > user > embedded
func MergeHarness(project, user, embedded *Harness) *Harness {
    result := &Harness{}

    // Start with embedded defaults
    if embedded != nil {
        *result = *embedded
    }

    // Merge user config
    if user != nil {
        mergeInto(result, user)
    }

    // Merge project config (highest priority)
    if project != nil {
        mergeInto(result, project)
    }

    return result
}

func mergeInto(dst, src *Harness) {
    // Scalars override
    if src.Path != "" {
        dst.Path = src.Path
    }
    if src.Structure != "" {
        dst.Structure = src.Structure
    }
    if src.Strict {
        dst.Strict = src.Strict
    }

    // Maps merge recursively
    dst.Variables = mergeMaps(dst.Variables, src.Variables)
    dst.Tools = mergeMaps(dst.Tools, src.Tools)
    dst.Keys = mergeMaps(dst.Keys, src.Keys)
    // etc.
}
```

## Context Dict Construction

```go
func (t *Transformer) buildContext() map[string]any {
    return map[string]any{
        "name":       t.HarnessName,
        "path":       t.HarnessPath,
        "artifacts":  t.Artifacts,
        "strict":     t.Strict,
        "variables":  t.Variables,
        "tools":      t.Tools,
        "keys":       t.Keys,
        "references": t.References,
    }
}
```
