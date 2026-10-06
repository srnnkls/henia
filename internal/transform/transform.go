package transform

import (
	"bytes"
	"fmt"
	"maps"
	"path"
	"strings"
	"sync"
	"text/template"
	"text/template/parse"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/canonical"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/srnnkls/henia/internal/slots"
	"github.com/srnnkls/henia/internal/profile"
)

const StaticBlock = "static"

type ReferenceConfig struct {
	Output string
}

type Transformer struct {
	Profile      string
	Strict       bool
	Compiler     *profile.Compiler
	Context      profile.Context
	OutputFormat string
	Variables    map[string]string
	Keys         map[string]string
	Values       map[string]map[string]string
	References   map[string]ReferenceConfig
	Tools        map[string]string
	Served       map[string]bool
	Head         bool
	LibraryLinks bool
}

const absent = "henia_absent"

func ExecuteTemplate[T any](content string, vars map[string]T) (string, error) {
	tmpl, err := template.New("content").Funcs(template.FuncMap{absent: blankAbsent}).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	for _, t := range tmpl.Templates() {
		if t.Tree != nil {
			printAbsentBlank(t.Tree.Root)
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func blankAbsent(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func printAbsentBlank(node parse.Node) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				printAbsentBlank(child)
			}
		}
	case *parse.ActionNode:
		if len(n.Pipe.Decl) == 0 {
			n.Pipe.Cmds = append(n.Pipe.Cmds, &parse.CommandNode{
				NodeType: parse.NodeCommand,
				Pos:      n.Pos,
				Args:     []parse.Node{parse.NewIdentifier(absent).SetPos(n.Pos)},
			})
		}
	case *parse.IfNode:
		printAbsentBlank(n.List)
		printAbsentBlank(n.ElseList)
	case *parse.RangeNode:
		printAbsentBlank(n.List)
		printAbsentBlank(n.ElseList)
	case *parse.WithNode:
		printAbsentBlank(n.List)
		printAbsentBlank(n.ElseList)
	}
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

	result.Frontmatter = ApplyMappings(result.Frontmatter, t.Keys)

	result.Frontmatter = ApplyValueMappings(result.Frontmatter, t.Values)
	if (art.Type == artifact.TypeSkill || art.Type == artifact.TypeAgent) && t.Profile != "" {
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
	body = slots.Expand(body, slots.Preload)
	full, head := markup.Unwrap(body, StaticBlock)
	if t.Head {
		body = strings.Join(head, "\n")
	} else {
		body = full
	}
	if art.Type == artifact.TypeSkill {
		body = t.renderLinks(body, art.Name)
	}

	body, err = markup.Render(body, t.OutputFormat)
	if err != nil {
		return nil, fmt.Errorf("render directives: %w", err)
	}

	body = t.renderReferences(body, art.FullName())

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

// RenderReferences rewrites canonical references in body to the configured harness syntax.
func (t *Transformer) RenderReferences(body string) string { return t.renderReferences(body, "") }

func (t *Transformer) renderReferences(body, self string) string {
	if !strings.Contains(body, "`") {
		return body
	}
	refs := reference.Parse(body)
	if len(refs) == 0 {
		return body
	}

	var result strings.Builder
	end := 0
	for _, ref := range refs {
		var replacement string

		if ref.Type == reference.TypeTool {
			if mapped, ok := t.Tools[ref.Name]; ok {
				replacement = "`" + mapped + "`"
			}
		} else if ref.Type == reference.TypeSkill && t.Served[ref.Name] && ref.Name != self {
			replacement = "`henia show " + ref.Name + "`"
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
			result.WriteString(body[end:ref.Start])
			result.WriteString(replacement)
			end = ref.End
		}
	}
	result.WriteString(body[end:])
	return result.String()
}

func wrapOutput(output string) string {
	if strings.ContainsAny(output, "*[]") {
		return output
	}
	return "`" + output + "`"
}

var referenceTemplates sync.Map

func referenceTemplate(source string) (*template.Template, error) {
	if cached, ok := referenceTemplates.Load(source); ok {
		return cached.(*template.Template), nil
	}
	parsed, err := template.New("ref").Parse(source)
	if err != nil {
		return nil, err
	}
	referenceTemplates.Store(source, parsed)
	return parsed, nil
}

func (t *Transformer) executeReferenceTemplate(tmpl string, ref reference.Reference) (string, error) {
	data := map[string]string{
		"Name": ref.Name,
		"Type": ref.Type.String(),
		"Raw":  ref.Raw,
	}

	parsed, err := referenceTemplate(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (t *Transformer) renderLinks(body, skill string) string {
	if !strings.Contains(body, "](") {
		return body
	}
	var out strings.Builder
	end := 0
	for _, link := range markup.Links([]byte(body)) {
		target, ok := t.libraryTarget(skill, link.Dest)
		if !ok {
			continue
		}
		command := "`henia show " + target + "`"
		text := strings.Trim(link.Text, "`*_~")
		replacement := link.Text + " (" + command + ")"
		file := strings.SplitN(link.Dest, "#", 2)[0]
		if text == link.Dest || text == file || text == strings.TrimPrefix(path.Clean(file), "../") || text == strings.SplitN(target, "#", 2)[0] {
			replacement = command
		}
		out.WriteString(body[end:link.Start])
		out.WriteString(replacement)
		end = link.End
	}
	if end == 0 {
		return body
	}
	out.WriteString(body[end:])
	return out.String()
}

func (t *Transformer) libraryTarget(skill, dest string) (string, bool) {
	if dest == "" || strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "mailto:") {
		return "", false
	}
	file, anchor, anchored := strings.Cut(dest, "#")
	suffix := ""
	if anchored && anchor != "" {
		suffix = "#" + anchor
	}
	if file == "" {
		return skill + suffix, t.Head
	}
	clean := path.Clean(file)
	owner, rest := skill, clean
	if other, ok := strings.CutPrefix(clean, "../"); ok {
		owner, rest, _ = strings.Cut(other, "/")
		if owner == "" || owner == ".." || !t.LibraryLinks && !t.Served[owner] {
			return "", false
		}
	} else if !t.LibraryLinks || strings.HasPrefix(clean, "..") {
		return "", false
	}
	if rest == "" || rest == "SKILL.md" {
		return owner + suffix, true
	}
	if module, ok := library.Module(rest); ok {
		return owner + "." + module + suffix, true
	}
	return owner + "/" + rest + suffix, true
}
