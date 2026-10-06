package transform

import (
	"bytes"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"text/template"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/canonical"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/srnnkls/henia/internal/slots"
	"github.com/srnnkls/henia/internal/vendor"
)

const StaticBlock = "static"

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
	Keys         map[string]string
	Values       map[string]map[string]string
	References   map[string]ReferenceConfig
	Tools        map[string]string
	Served       map[string]bool
	Head         bool
	LibraryLinks bool
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
	result, _, err := t.TransformReferences(art)
	return result, err
}

func (t *Transformer) TransformReferences(art *artifact.Artifact) (*artifact.Artifact, []reference.Rendered, error) {
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
			return nil, nil, fmt.Errorf("transform frontmatter %q: %w", k, err)
		}
		result.Frontmatter[k] = transformed
	}

	result.Frontmatter = ApplyMappings(result.Frontmatter, t.Keys)

	result.Frontmatter = ApplyValueMappings(result.Frontmatter, t.Values)
	if (art.Type == artifact.TypeSkill || art.Type == artifact.TypeAgent) && t.Profile != "" {
		can, err := canonical.Parse(result.Frontmatter)
		if err != nil {
			return nil, nil, err
		}
		if t.Compiler == nil {
			return nil, nil, fmt.Errorf("profile %s was not compiled", t.Profile)
		}
		compiled, err := t.Compiler.Compile(art.FullName(), can.ToMap(), t.Context)
		if err != nil {
			return nil, nil, err
		}
		result.Frontmatter, result.Files, result.Warnings = compiled.Frontmatter, compiled.Files, compiled.Warnings
		if t.Strict && len(result.Warnings) > 0 {
			return nil, nil, fmt.Errorf("unsupported skill metadata: %s", strings.Join(result.Warnings, "; "))
		}
	}

	body, err := ExecuteTemplate(art.Body, templateContext)
	if err != nil {
		return nil, nil, fmt.Errorf("transform body: %w", err)
	}
	body, refs, warnings, err := t.renderBody(body, art)
	if err != nil {
		return nil, nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Body = body

	return result, refs, nil
}

func (t *Transformer) renderBody(body string, art *artifact.Artifact) (string, []reference.Rendered, []string, error) {
	tree, err := markup.Tree([]byte(body))
	if err != nil {
		return "", nil, nil, fmt.Errorf("render directives: %w", err)
	}
	var edits []markup.Edit
	for _, a := range slots.ApplicationsIn(tree) {
		edits = append(edits, markup.Edit{Start: a.Start, End: a.End, Text: slots.Preload(a.Slots), Rule: "slot"})
	}
	statics := staticBlocks(tree, body)
	if !t.Head {
		for _, s := range statics {
			edits = append(edits, markup.Edit{Start: s.start, End: s.inner, Rule: "static"}, markup.Edit{Start: max(s.inner, s.innerEnd-1), End: s.end, Rule: "static"})
		}
	}
	var links []libraryLink
	if art.Type == artifact.TypeSkill {
		var linkEdits []markup.Edit
		linkEdits, links = t.linkEdits(tree, body, art.Name)
		edits = append(edits, linkEdits...)
	}
	rewrites, sources, err := t.rewrites(tree, art.FullName())
	if err != nil {
		return "", nil, nil, fmt.Errorf("render references: %w", err)
	}
	edits = append(edits, rewrites...)
	directives, err := markup.DirectiveEdits(tree, t.OutputFormat)
	if err != nil {
		return "", nil, nil, fmt.Errorf("render directives: %w", err)
	}
	edits = append(edits, directives...)
	applied := markup.Apply(body, edits)
	warnings := reference.Warnings(body, applied.Dropped)
	refs := reference.Locate(applied, sources)
	for _, link := range links {
		if start, _, rule, ok := applied.Span(link.source.Start, link.source.End); ok && rule == "library-link" {
			ref := link.source
			ref.Start = start + link.offset
			ref.End = ref.Start + 1 + len(ref.Raw)
			refs = append(refs, ref)
		}
	}
	slices.SortFunc(refs, func(a, b reference.Rendered) int { return a.Start - b.Start })
	if !t.Head {
		return applied.Text, refs, warnings, nil
	}
	var head strings.Builder
	var located []reference.Rendered
	for i, s := range statics {
		if i > 0 {
			head.WriteString("\n")
		}
		from, _ := applied.Offset(s.inner)
		to, _ := applied.Offset(s.innerEnd)
		for _, ref := range refs {
			if from <= ref.Start && ref.End <= to {
				ref.Start, ref.End = ref.Start-from+head.Len(), ref.End-from+head.Len()
				located = append(located, ref)
			}
		}
		head.WriteString(applied.Text[from:to])
	}
	return head.String(), located, warnings, nil
}

type static struct{ start, inner, innerEnd, end int }

func staticBlocks(tree *markup.Element, body string) []static {
	var blocks []static
	var visit func(*markup.Element)
	visit = func(e *markup.Element) {
		if e.Type != "directive" || e.Attrs["name"] != StaticBlock || e.Attrs["inline"] == "true" {
			for _, child := range e.Children {
				visit(child)
			}
			return
		}
		block := static{start: e.Start, end: e.End, inner: e.End, innerEnd: e.End}
		if newline := strings.IndexByte(body[e.Start:e.End], '\n'); newline >= 0 {
			block.inner = e.Start + newline + 1
			block.innerEnd = block.inner
			if last := strings.LastIndexByte(body[block.inner:e.End], '\n'); last >= 0 {
				block.innerEnd = block.inner + last + 1
			}
		}
		blocks = append(blocks, block)
	}
	visit(tree)
	return blocks
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

func (t *Transformer) RenderReferences(body string) (string, error) {
	if !strings.Contains(body, "`") {
		return body, nil
	}
	body, _, err := reference.Render(body, t.rendering(""))
	return body, err
}

func (t *Transformer) rendering(self string) reference.Rendering {
	served := maps.Clone(t.Served)
	delete(served, self)
	references := make(map[string]string, len(t.References))
	for kind, config := range t.References {
		references[kind] = config.Output
	}
	return reference.Rendering{Tools: t.Tools, Served: served, References: references}
}

func (t *Transformer) rewrites(tree *markup.Element, self string) ([]markup.Edit, []reference.Rendered, error) {
	return reference.Rewrites(tree, t.rendering(self))
}

type libraryLink struct {
	source reference.Rendered
	offset int
}

func (t *Transformer) linkEdits(tree *markup.Element, body, skill string) ([]markup.Edit, []libraryLink) {
	var edits []markup.Edit
	var links []libraryLink
	tree.Walk(func(e *markup.Element) bool {
		if e.Type != "link" {
			return true
		}
		link, ok := markup.LinkSyntax(body, e)
		if !ok {
			return true
		}
		ref, ok := t.libraryTarget(skill, link.Dest)
		if !ok {
			return true
		}
		target := strings.TrimPrefix(ref.Raw, "henia show ")
		command := "`" + ref.Raw + "`"
		text := strings.Trim(link.Text, "`*_~")
		file := strings.SplitN(link.Dest, "#", 2)[0]
		edit, offset := markup.Edit{Start: link.Start, End: link.End, Text: command, Rule: "library-link"}, 0
		if !(text == link.Dest || text == file || text == strings.TrimPrefix(path.Clean(file), "../") || text == strings.SplitN(target, "#", 2)[0]) {
			edits = append(edits, markup.Edit{Start: link.Start, End: link.Start + 1, Rule: "library-link"})
			edit, offset = markup.Edit{Start: link.Start + 1 + len(link.Text), End: link.End, Text: " (" + command + ")", Rule: "library-link"}, 2
		}
		edits = append(edits, edit)
		ref.Start, ref.End = edit.Start, edit.End
		links = append(links, libraryLink{reference.Rendered{Reference: ref, Address: true}, offset})
		return true
	})
	return edits, links
}

func (t *Transformer) libraryTarget(skill, dest string) (reference.Reference, bool) {
	if dest == "" || strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "mailto:") {
		return reference.Reference{}, false
	}
	file, anchor, _ := strings.Cut(dest, "#")
	target := func(owner, module, resource string) reference.Reference {
		address := owner
		if module != "" {
			address += "." + module
		}
		if resource != "" {
			address += "/" + resource
		}
		if anchor != "" {
			address += "#" + anchor
		}
		return reference.Reference{Type: reference.TypeSkill, Name: owner, Module: module, Resource: resource, Anchor: anchor, Raw: "henia show " + address}
	}
	if file == "" {
		return target(skill, "", ""), t.Head
	}
	clean := path.Clean(file)
	owner, rest := skill, clean
	if other, ok := strings.CutPrefix(clean, "../"); ok {
		owner, rest, _ = strings.Cut(other, "/")
		if owner == "" || owner == ".." || !t.LibraryLinks && !t.Served[owner] {
			return reference.Reference{}, false
		}
	} else if !t.LibraryLinks || strings.HasPrefix(clean, "..") {
		return reference.Reference{}, false
	}
	if rest == "" || rest == "SKILL.md" {
		return target(owner, "", ""), true
	}
	if module, ok := library.Module(rest); ok {
		return target(owner, module, ""), true
	}
	return target(owner, "", rest), true
}
