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

type templateSource struct {
	file    *markup.Element
	source  []byte
	offset  int
	defines []templateName
	uses    []templateName
	shared  *templates.File
}

func addTemplates(root *markup.Element, documents []document, shared []templates.File) {
	files := map[string]*markup.Element{}
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "file" {
			files[e.Attrs["file"]] = e
		}
		return true
	})
	var sources []*templateSource
	for _, d := range documents {
		file := files[d.path]
		if file == nil {
			continue
		}
		s := &templateSource{file: file, source: d.source, offset: d.offset}
		s.defines, s.uses = templateNames(string(d.body))
		if layout := layoutName(d); layout != "" {
			s.uses = append(s.uses, templateName{name: layout, offset: bytes.Index(d.source[:d.offset], []byte(layout)) - d.offset})
		}
		sources = append(sources, s)
	}
	known := map[string]bool{}
	for i := range shared {
		data, err := os.ReadFile(shared[i].Path)
		if err != nil {
			continue
		}
		attrs := map[string]string{"file": shared[i].Path, "path": shared[i].Path, "kind": "template", "name": shared[i].Name}
		if shared[i].User {
			attrs["dependency"] = "true"
		}
		file := &markup.Element{Type: "file", Attrs: attrs, Line: 1, EndLine: 1, Parent: root, Index: len(root.Children)}
		root.Children = append(root.Children, file)
		s := &templateSource{file: file, source: data, shared: &shared[i]}
		s.defines, s.uses = templateNames(string(data))
		known[shared[i].Name] = true
		for _, define := range s.defines {
			known[define.name] = true
		}
		sources = append(sources, s)
	}
	used := map[string]bool{}
	for _, s := range sources {
		own := map[string]bool{}
		for _, define := range s.defines {
			own[define.name] = true
		}
		for _, use := range s.uses {
			if s.shared == nil || !own[use.name] {
				used[use.name] = true
			}
			if !use.implicit {
				s.add("use", use, map[string]string{"resolved": strconv.FormatBool(known[use.name] || own[use.name])})
			}
		}
	}
	for _, s := range sources {
		if s.shared == nil {
			continue
		}
		reached := used[s.shared.Name]
		for _, define := range s.defines {
			reached = reached || used[define.name]
		}
		s.add("template", templateName{name: s.shared.Name}, map[string]string{"used": strconv.FormatBool(reached)})
	}
}

func (s *templateSource) add(role string, n templateName, attrs map[string]string) {
	offset := min(max(s.offset+n.offset, 0), len(s.source))
	line := bytes.Count(s.source[:offset], []byte{'\n'}) + 1
	column := offset - bytes.LastIndexByte(s.source[:offset], '\n')
	attrs["role"], attrs["name"] = role, n.name
	s.file.Children = append(s.file.Children, &markup.Element{Type: "template", Attrs: attrs, Text: n.name, Line: line, EndLine: line, Column: column, Parent: s.file, Index: len(s.file.Children)})
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
