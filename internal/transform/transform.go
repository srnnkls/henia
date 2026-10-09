package transform

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"text/template/parse"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/canonical"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/profile"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/srnnkls/henia/internal/slots"
	"github.com/srnnkls/henia/internal/templating"
)

const HeadBlock = markup.HeadBlock

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
	Templates    map[string]string
}

func ExecuteTemplate[T any](content string, vars map[string]T) (string, error) {
	return Compose(content, vars, nil, "")
}

// Compose executes content with the shared templates in scope. With a layout,
// content may only define blocks, and the layout template renders them.
func Compose[T any](content string, vars map[string]T, shared map[string]string, layout string) (string, error) {
	tmpl := template.New("content").Funcs(templating.Funcs).Funcs(template.FuncMap{"required": requiredBlock})
	for _, name := range slices.Sorted(maps.Keys(shared)) {
		if _, err := tmpl.New(name).Parse(shared[name]); err != nil {
			return "", fmt.Errorf("parse template %s: %w", name, err)
		}
	}
	if _, err := tmpl.Parse(content); err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	templating.BlankAbsent(tmpl)
	entry := tmpl
	if layout != "" {
		if !blocksOnly(tmpl.Tree.Root) {
			return "", fmt.Errorf("layout %s: text outside define blocks", layout)
		}
		if entry = tmpl.Lookup(layout); entry == nil || entry.Tree == nil {
			return "", fmt.Errorf("unknown layout %q", layout)
		}
	}

	var buf bytes.Buffer
	if err := entry.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

func requiredBlock(name string) (string, error) {
	return "", fmt.Errorf("block %q is required", name)
}

func blocksOnly(root *parse.ListNode) bool {
	if root == nil {
		return true
	}
	for _, node := range root.Nodes {
		text, ok := node.(*parse.TextNode)
		if !ok || len(bytes.TrimSpace(text.Text)) > 0 {
			return false
		}
	}
	return true
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
	if vars, ok := henia(art.Frontmatter)["variables"].(map[string]any); ok {
		maps.Copy(templateContext, vars)
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

	layout, _ := henia(art.Frontmatter)["layout"].(string)
	body, err := Compose(art.Body, templateContext, t.Templates, layout)
	if err != nil {
		return nil, fmt.Errorf("transform body: %w", err)
	}
	var shared func(string) bool
	if len(t.Templates) > 0 {
		shared = func(name string) bool { _, ok := t.Templates[name]; return ok }
	}
	body, err = markup.Expand(body, shared, func(name string, args map[string]string, content string) (string, error) {
		data := maps.Clone(templateContext)
		data["args"], data["content"] = args, content
		return Compose("{{template "+strconv.Quote(name)+" .}}", data, t.Templates, "")
	})
	if err != nil {
		return nil, fmt.Errorf("expand template directives: %w", err)
	}
	body = slots.Expand(body, slots.Preload)
	if art.Type == artifact.TypeSkill {
		body = expandListings(body, art.Name)
	}
	full, head := markup.Unwrap(body, HeadBlock)
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

func (t *Transformer) renderDocument(doc *markup.Document, art *artifact.Artifact) (string, error) {
	if head := markup.UnwrapDocument(doc, HeadBlock); t.Head {
		doc = head
	}
	if art.Type == artifact.TypeSkill {
		markup.RewriteText(doc, func(body string) string { return t.renderLinks(body, art.Name) })
	}
	if err := markup.RenderDocument(doc, t.OutputFormat); err != nil {
		return "", fmt.Errorf("render directives: %w", err)
	}
	markup.RewriteText(doc, func(body string) string { return t.renderReferences(body, art.FullName()) })
	return string(markup.Write(doc)), nil
}

func henia(frontmatter map[string]any) map[string]any {
	h, _ := frontmatter["henia"].(map[string]any)
	return h
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
			replacement = "`" + library.ShowCommand(ref.Name) + "`"
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
		command := "`" + library.ShowCommand(target) + "`"
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

var listings = map[string]func(skill string) string{
	"contents": func(skill string) string { return "!`henia show " + skill + " --toc`" },
	"related":  func(skill string) string { return "!`henia context " + skill + "`" },
}

var listingText = regexp.MustCompile(`^:[a-z]+\[([^\]]*)\]`)

func expandListings(body, self string) string {
	if !strings.Contains(body, ":contents[") && !strings.Contains(body, ":related[") && !strings.Contains(body, ":::related") {
		return body
	}
	doc, _ := markup.Parse("", []byte(body))
	listed := slots.Rewrite(doc, body, func(it *markup.Item, span string) (string, bool) {
		render, ok := listings[it.Attrs["name"]]
		if !ok {
			return "", false
		}
		m := listingText.FindStringSubmatch(span)
		if m == nil {
			return "", false
		}
		return render(cmp.Or(strings.TrimSpace(m[1]), self)), true
	})
	related := relatedSections(&doc.Children, body)
	if !listed && !related {
		return body
	}
	return string(markup.Write(doc))
}

func relatedSections(items *[]*markup.Item, source string) bool {
	changed := false
	for i := 0; i < len(*items); i++ {
		it := (*items)[i]
		if relatedSections(&it.Children, source) {
			changed = true
		}
		if it.Kind != "directive" || it.Attrs["name"] != "related" || it.Attrs["inline"] == "true" || !strings.HasPrefix(source[it.Origin.Start:], ":::") {
			continue
		}
		heading := &markup.Item{Kind: "heading", Attrs: map[string]string{"level": "2"}, Text: "Related"}
		*items = slices.Replace(*items, i, i+1, it.Replace(append([]*markup.Item{heading}, it.Children...)...)...)
		i += len(it.Children)
		changed = true
	}
	return changed
}
