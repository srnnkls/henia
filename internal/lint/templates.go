package lint

import (
	"bytes"
	"maps"
	"os"
	"strconv"
	"text/template/parse"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/slots"
	"github.com/srnnkls/henia/internal/templates"
	"github.com/srnnkls/henia/internal/transform"
)

type templateName struct {
	name     string
	offset   int
	implicit bool
}

// sectionAnchors over-approximates the anchors d renders: its own headings and
// those of every shared template it reaches, whichever branch emits them.
func sectionAnchors(d document, shared []templates.File) map[string]bool {
	anchors := map[string]bool{}
	collect := func(body []byte) {
		sections, _ := markup.Sections(body)
		for _, section := range sections {
			anchors[section.Anchor] = true
		}
	}
	collect(d.body)
	maps.Copy(anchors, d.expandedAnchors)
	owner := map[string]int{}
	sources := make([][]byte, len(shared))
	for i, f := range shared {
		if data, err := os.ReadFile(f.Path); err == nil {
			sources[i] = data
		}
		owner[f.Name] = i
		defines, _ := templateNames(string(sources[i]))
		for _, define := range defines {
			if _, ok := owner[define.name]; !ok {
				owner[define.name] = i
			}
		}
	}
	_, pending := templateNames(string(d.body))
	if layout := layoutName(d); layout != "" {
		pending = append(pending, templateName{name: layout})
	}
	visited := map[int]bool{}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		i, ok := owner[next.name]
		if !ok || visited[i] {
			continue
		}
		visited[i] = true
		collect(sources[i])
		_, uses := templateNames(string(sources[i]))
		pending = append(pending, uses...)
	}
	return anchors
}

func layoutName(d document) string {
	h, _ := d.art.Frontmatter["henia"].(map[string]any)
	layout, _ := h["layout"].(string)
	return layout
}

func templateNames(body string) (defines, uses []templateName) {
	if !bytes.Contains([]byte(body), []byte("{{")) {
		return nil, directiveTemplates(body)
	}
	tree := parse.New("body")
	tree.Mode = parse.SkipFuncCheck
	trees := map[string]*parse.Tree{}
	if _, err := tree.Parse(body, "", "", trees); err != nil {
		return nil, directiveTemplates(body)
	}
	var visit func(parse.Node)
	visit = func(node parse.Node) {
		switch n := node.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					visit(child)
				}
			}
		case *parse.TemplateNode:
			uses = append(uses, templateName{name: n.Name, offset: int(n.Pos)})
		case *parse.IfNode:
			visit(n.List)
			visit(n.ElseList)
		case *parse.RangeNode:
			visit(n.List)
			visit(n.ElseList)
		case *parse.WithNode:
			visit(n.List)
			visit(n.ElseList)
		}
	}
	for name, t := range trees {
		if name != "body" {
			defines = append(defines, templateName{name: name, offset: int(t.Root.Pos)})
		}
		visit(t.Root)
	}
	return defines, append(uses, directiveTemplates(body)...)
}

func directiveTemplates(body string) []templateName {
	source := []byte(body)
	if masked, err := markup.MaskTemplates(source); err == nil {
		source = masked
	}
	var uses []templateName
	for _, use := range markup.TemplateDirectives(source) {
		uses = append(uses, templateName{use.Name, use.Offset, !use.Explicit})
	}
	return uses
}

func templateSlots(d document, body string, shared, variables map[string]string) []string {
	if len(shared) == 0 {
		return nil
	}
	data := maps.Clone(d.art.Frontmatter)
	if h, ok := d.art.Frontmatter["henia"].(map[string]any); ok {
		if vars, ok := h["variables"].(map[string]any); ok {
			maps.Copy(data, vars)
		}
	}
	for k, v := range variables {
		data[k] = v
	}
	var found []string
	has := func(name string) bool { _, ok := shared[name]; return ok }
	_, _ = markup.Expand(body, has, func(name string, args map[string]string, content string) (string, error) {
		scope := maps.Clone(data)
		scope["args"], scope["content"] = args, ""
		rendered, err := transform.Compose("{{template "+strconv.Quote(name)+" .}}", scope, shared, "")
		if err == nil {
			for _, application := range slots.Applications(rendered) {
				found = append(found, application.Slots...)
			}
		}
		return "", nil
	})
	return found
}
