package transform

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/canonical"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/srnnkls/henia/internal/vendor"
)

type ReferenceConfig struct {
	Output string
}

type Transformer struct {
	Profile      string
	Strict       bool
	Compiler     *vendor.Compiler
	Context      vendor.Context
	OutputFormat string
	Variables    map[string]string
	Mappings     map[string]string
	Keys         map[string]string
	Values       map[string]map[string]string
	References   map[string]ReferenceConfig
	Tools        map[string]string
}

func ExecuteTemplate[T any](content string, vars map[string]T) (string, error) {
	tmpl, err := template.New("content").Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func ApplyMappings(fm map[string]any, mappings map[string]string) map[string]any {
	if mappings == nil {
		result := make(map[string]any)
		maps.Copy(result, fm)
		return result
	}

	result := make(map[string]any)
	for k, v := range fm {
		if newKey, ok := mappings[k]; ok {
			result[newKey] = v
		} else {
			result[k] = v
		}
	}

	return result
}

func ApplyValueMappings(fm map[string]any, values map[string]map[string]string) map[string]any {
	if values == nil {
		result := make(map[string]any)
		maps.Copy(result, fm)
		return result
	}

	result := make(map[string]any)
	for k, v := range fm {
		if valueMap, ok := values[k]; ok {
			if str, isStr := v.(string); isStr {
				if mapped, found := valueMap[str]; found {
					result[k] = mapped
				} else {
					result[k] = v
				}
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}

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

	templateContext := make(map[string]any, len(art.Frontmatter)+len(t.Variables))
	maps.Copy(templateContext, art.Frontmatter)
	if h, ok := art.Frontmatter["henia"].(map[string]any); ok {
		if vars, ok := h["variables"].(map[string]any); ok {
			maps.Copy(templateContext, vars)
		}
	}
	for k, v := range t.Variables {
		templateContext[k] = v
	}

	for k, v := range art.Frontmatter {
		transformed, err := templateValue(v, templateContext)
		if err != nil {
			return nil, fmt.Errorf("transform frontmatter %q: %w", k, err)
		}
		result.Frontmatter[k] = transformed
	}

	keyMappings := t.Mappings
	if t.Keys != nil {
		keyMappings = t.Keys
	}
	result.Frontmatter = ApplyMappings(result.Frontmatter, keyMappings)

	result.Frontmatter = ApplyValueMappings(result.Frontmatter, t.Values)
	if art.Type == artifact.TypeSkill && t.Profile != "" {
		can, err := canonical.Parse(result.Frontmatter)
		if err != nil {
			return nil, err
		}
		if t.Compiler == nil {
			return nil, fmt.Errorf("profile %s was not compiled", t.Profile)
		}
		compiled, err := t.Compiler.Compile(art.FullName(), can.ToMap(), t.Context)
		if err != nil {
			return nil, err
		}
		result.Frontmatter, result.Files, result.Warnings = compiled.Frontmatter, compiled.Files, compiled.Warnings
		if t.Strict && len(result.Warnings) > 0 {
			return nil, fmt.Errorf("unsupported skill metadata: %s", strings.Join(result.Warnings, "; "))
		}
	}

	body, err := ExecuteTemplate(art.Body, templateContext)
	if err != nil {
		return nil, fmt.Errorf("transform body: %w", err)
	}

	body, err = markup.Render(body, t.OutputFormat)
	if err != nil {
		return nil, fmt.Errorf("render directives: %w", err)
	}

	body = t.transformReferences(body)

	result.Body = body

	return result, nil
}

func templateValue(value any, context map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		return ExecuteTemplate(v, context)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			var err error
			out[k], err = templateValue(item, context)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			var err error
			out[i], err = templateValue(item, context)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	default:
		return value, nil
	}
}

func (t *Transformer) transformReferences(body string) string {
	refs := reference.Parse(body)
	if len(refs) == 0 {
		return body
	}

	for _, ref := range refs {
		var replacement string

		if ref.Type == reference.TypeTool {
			if mapped, ok := t.Tools[ref.Name]; ok {
				replacement = "`" + mapped + "`"
			}
		} else {
			refConfig, ok := t.References[ref.Type.String()]
			if ok {
				output, err := t.executeReferenceTemplate(refConfig.Output, ref)
				if err == nil {
					replacement = wrapOutput(output)
				}
			}
		}

		if replacement != "" {
			body = strings.Replace(body, "`"+ref.Raw+"`", replacement, 1)
		}
	}

	return body
}

func wrapOutput(output string) string {
	if strings.ContainsAny(output, "*[]") {
		return output
	}
	return "`" + output + "`"
}

func (t *Transformer) executeReferenceTemplate(tmpl string, ref reference.Reference) (string, error) {
	data := map[string]string{
		"Name": ref.Name,
		"Type": ref.Type.String(),
		"Raw":  ref.Raw,
	}

	parsed, err := template.New("ref").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
