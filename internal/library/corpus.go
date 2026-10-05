package library

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
	"gopkg.in/yaml.v3"
)

const maxCorpusFile = 1 << 20

type Corpus struct {
	Root      *markup.Element
	Problems  []string
	lib       *Library
	skills    map[string]*markup.Element
	rendering *Rendering
	anchors   map[string]map[string]bool
}

type Rendering struct {
	Body      func(Entry) string
	Reference func(Entry) *regexp.Regexp
}

type Document struct {
	Path       string
	Kind       string
	Dependency bool
}

func (l *Library) Corpus(rendering *Rendering) *Corpus {
	c := newCorpus()
	c.lib, c.rendering = l, rendering
	for _, e := range l.Entries {
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": e.Name, "name": e.Name, "package": e.Package, "ref": l.Reference(e), "dir": filepath.Dir(e.Path)}}
		c.adopt(c.Root, skill)
		c.skills[e.ID] = skill
	}
	for _, e := range l.Entries {
		skill := c.skills[e.ID]
		dir := filepath.Dir(e.Path)
		entry := e
		var render func() (string, *regexp.Regexp)
		if rendering != nil {
			render = func() (string, *regexp.Regexp) { return rendering.Body(entry), rendering.Reference(entry) }
		}
		c.file(skill, e.Path, "SKILL.md", "skill", render)
		for _, path := range resourceFiles(dir) {
			rel, _ := filepath.Rel(dir, path)
			c.file(skill, path, filepath.ToSlash(rel), "resource", nil)
		}
	}
	return c
}

func Documents(docs []Document) *Corpus {
	c := newCorpus()
	dirs := map[string]*markup.Element{}
	for _, d := range docs {
		if filepath.Base(d.Path) != "SKILL.md" {
			continue
		}
		id := filepath.Base(filepath.Dir(d.Path))
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": id, "name": id, "ref": id, "dir": filepath.Dir(d.Path)}}
		if data, err := os.ReadFile(d.Path); err == nil {
			if art, err := artifact.Parse(data); err == nil {
				if name, ok := art.Frontmatter["name"].(string); ok && name != "" {
					skill.Attrs["name"] = name
					c.skills[name] = skill
				}
			}
		}
		c.adopt(c.Root, skill)
		c.skills[id] = skill
		dirs[filepath.Dir(d.Path)] = skill
	}
	for _, d := range docs {
		add := c.file
		if d.Dependency {
			add = c.stub
		}
		if skill := dirs[filepath.Dir(d.Path)]; skill != nil && filepath.Base(d.Path) == "SKILL.md" {
			add(skill, d.Path, "SKILL.md", d.Kind, nil)
			continue
		}
		owner := ""
		for dir := filepath.Dir(d.Path); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if dirs[dir] != nil {
				owner = dir
				break
			}
		}
		if owner != "" && d.Kind == "" {
			rel, _ := filepath.Rel(owner, d.Path)
			add(dirs[owner], d.Path, filepath.ToSlash(rel), "resource", nil)
			continue
		}
		add(c.Root, d.Path, filepath.ToSlash(d.Path), cmp.Or(d.Kind, "document"), nil)
	}
	return c
}

func newCorpus() *Corpus {
	return &Corpus{Root: &markup.Element{Type: "corpus", Attrs: map[string]string{}}, skills: map[string]*markup.Element{}, anchors: map[string]map[string]bool{}}
}

func (c *Corpus) Skill(ref string) *markup.Element {
	if c.lib != nil {
		e, err := c.lib.Resolve(ref)
		if err != nil {
			return nil
		}
		return c.skills[e.ID]
	}
	if _, name, qualified := strings.Cut(ref, ":"); qualified {
		ref = name
	}
	return c.skills[ref]
}

func (c *Corpus) file(parent *markup.Element, physical, rel, kind string, render func() (string, *regexp.Regexp)) {
	data, err := os.ReadFile(physical)
	if err != nil || len(data) > maxCorpusFile || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return
	}
	attrs := map[string]string{"path": rel, "main": fmt.Sprint(rel == "SKILL.md"), "kind": kind, "file": physical}
	typed := kind == "skill" || kind == "command" || kind == "agent"
	if !strings.EqualFold(filepath.Ext(physical), ".md") {
		file := markup.PlainTree(data)
		for k, v := range attrs {
			file.Attrs[k] = v
		}
		c.adopt(parent, file)
		return
	}
	art, err := artifact.Parse(data)
	if err != nil {
		file := &markup.Element{Type: "file", Attrs: attrs, Line: 1, EndLine: 1}
		c.adopt(file, problem("frontmatter", err.Error(), 1, 1))
		c.adopt(parent, file)
		c.Problems = append(c.Problems, fmt.Sprintf("%s: %v", physical, err))
		return
	}
	offset := len(data) - len(art.Body)
	body, shift := art.Body, bytes.Count(data[:offset], []byte{'\n'})
	var syntax *regexp.Regexp
	if render != nil {
		body, syntax = render()
		shift = 0
	}
	file, _ := markup.Tree([]byte(body), c.references(body, syntax)...)
	var problems []*markup.Element
	directives := []byte(body)
	if typed {
		masked, err := markup.MaskTemplates(directives)
		if err != nil {
			problems = append(problems, problem("template", err.Error(), 1, 1))
		} else {
			directives = masked
		}
	}
	if _, err := markup.Render(string(directives), "directives"); err != nil {
		line, column := 1, 1
		if location, ok := errors.AsType[*markup.Error](err); ok {
			line, column = location.Line, location.Column
		}
		problems = append(problems, problem("markup", err.Error(), line, column))
	}
	for k, v := range attrs {
		file.Attrs[k] = v
	}
	if typed {
		name, _ := art.Frontmatter["name"].(string)
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(physical), ".md")
			if filepath.Base(physical) == artifact.MainFileName(artifact.Type(kind)) {
				name = filepath.Base(filepath.Dir(physical))
			}
		}
		file.Attrs["name"], file.Attrs["artifact"] = name, kind+":"+name
	}
	c.resolveLinks(file, parent.Attrs["id"], rel, physical)
	file.Walk(func(e *markup.Element) bool {
		if strings.Contains(e.Text, "{{") {
			e.Attrs["dynamic"] = "true"
		}
		e.Line, e.EndLine = e.Line+shift, e.EndLine+shift
		return true
	})
	for _, p := range problems {
		p.Line += shift
		p.EndLine = p.Line
	}
	var head []*markup.Element
	if offset > 0 {
		head = append(head, frontmatter(art.Frontmatter, data[:offset], shift))
	}
	file.Children = append(append(head, problems...), file.Children...)
	for i, child := range file.Children {
		child.Parent, child.Index = file, i
	}
	c.adopt(parent, file)
}

func (c *Corpus) stub(parent *markup.Element, physical, rel, kind string, _ func() (string, *regexp.Regexp)) {
	data, err := os.ReadFile(physical)
	if err != nil {
		return
	}
	art, err := artifact.Parse(data)
	if err != nil {
		return
	}
	file := &markup.Element{Type: "file", Attrs: map[string]string{"path": rel, "main": fmt.Sprint(rel == "SKILL.md"), "kind": kind, "file": physical, "dependency": "true"}}
	if kind == "skill" || kind == "command" || kind == "agent" {
		name, _ := art.Frontmatter["name"].(string)
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(physical), ".md")
			if filepath.Base(physical) == artifact.MainFileName(artifact.Type(kind)) {
				name = filepath.Base(filepath.Dir(physical))
			}
		}
		file.Attrs["name"], file.Attrs["artifact"] = name, kind+":"+name
	}
	sections, _ := markup.Sections([]byte(art.Body))
	for _, section := range sections {
		c.adopt(file, &markup.Element{Type: "section", Attrs: map[string]string{"id": section.Anchor, "title": section.Title, "level": strconv.Itoa(section.Level)}})
	}
	c.adopt(parent, file)
}

func problem(kind, message string, line, column int) *markup.Element {
	return &markup.Element{Type: "problem", Attrs: map[string]string{"kind": kind, "message": message}, Text: message, Line: line, EndLine: line, Column: column}
}

func frontmatter(values map[string]any, header []byte, lines int) *markup.Element {
	attrs := map[string]string{}
	for key, value := range values {
		switch value.(type) {
		case string, bool, int, float64:
			attrs[key] = fmt.Sprint(value)
		}
	}
	head := &markup.Element{Type: "frontmatter", Attrs: attrs, Text: strings.TrimSpace(strings.Trim(strings.TrimSpace(string(header)), "-")), Line: 1, EndLine: lines, Column: 1}
	var document yaml.Node
	if yaml.Unmarshal([]byte(head.Text), &document) == nil && len(document.Content) > 0 {
		var walk func(key string, index int, node *yaml.Node, line, column int)
		walk = func(key string, index int, node *yaml.Node, line, column int) {
			switch node.Kind {
			case yaml.MappingNode:
				for i := 0; i+1 < len(node.Content); i += 2 {
					k := node.Content[i]
					walk(strings.TrimPrefix(key+"."+k.Value, "."), index, node.Content[i+1], k.Line+1, k.Column)
				}
			case yaml.SequenceNode:
				for i, item := range node.Content {
					walk(key, i, item, item.Line+1, item.Column)
				}
			default:
				entry := &markup.Element{Type: "entry", Attrs: map[string]string{"key": key, "value": node.Value, "tag": node.ShortTag()}, Text: node.Value, Line: line, EndLine: line, Column: column}
				if index >= 0 {
					entry.Attrs["index"] = strconv.Itoa(index)
				}
				head.Children = append(head.Children, entry)
				entry.Parent, entry.Index = head, len(head.Children)-1
			}
		}
		walk("", -1, document.Content[0], 1, 1)
	}
	return head
}

func (c *Corpus) references(body string, syntax *regexp.Regexp) []*markup.Element {
	var links []*markup.Element
	add := func(start, end int, attrs map[string]string) {
		links = append(links, &markup.Element{Type: "link", Attrs: attrs, Start: start + 1, End: end - 1})
	}
	for _, ref := range reference.Parse(body) {
		switch ref.Type {
		case reference.TypeSkill:
			attrs := c.skillLink(ref)
			attrs["ref"], attrs["name"], attrs["artifact"] = "skill", ref.Name, "skill:"+ref.Name
			add(ref.Start, ref.End, attrs)
		case reference.TypeCommand, reference.TypeAgent:
			add(ref.Start, ref.End, map[string]string{"ref": string(ref.Type), "name": ref.Name, "artifact": string(ref.Type) + ":" + ref.Name, "dest": ref.Raw})
		case reference.TypeFile:
			add(ref.Start, ref.End, map[string]string{"ref": "file", "url": ref.Name, "dest": ref.Name})
		}
	}
	for _, ref := range reference.Shown(body) {
		attrs := c.skillLink(ref)
		attrs["ref"] = "show"
		if skill := c.Skill(ref.Name); skill != nil && skill.Attrs["dir"] != "" && attrs["path"] != "" {
			c.inspect(attrs, filepath.Join(skill.Attrs["dir"], filepath.FromSlash(attrs["path"])), ref.Anchor)
		}
		add(ref.Start, ref.End+1, attrs)
	}
	if syntax != nil {
		for _, m := range syntax.FindAllStringSubmatchIndex(body, -1) {
			attrs := c.skillLink(reference.Reference{Name: body[m[2]:m[3]], Raw: body[m[0]+1 : m[1]-1]})
			attrs["ref"] = "skill"
			add(m[0], m[1], attrs)
		}
	}
	return links
}

func (c *Corpus) skillLink(ref reference.Reference) map[string]string {
	target := ref.Name
	if _, name, qualified := strings.Cut(target, ":"); qualified && c.lib == nil {
		target = name
	}
	if skill := c.Skill(ref.Name); skill != nil {
		target = skill.Attrs["id"]
	}
	attrs := map[string]string{"target": target, "dest": ref.Raw}
	if ref.Resource != "" || ref.Anchor != "" {
		attrs["path"] = cmp.Or(ref.Resource, "SKILL.md")
	}
	if ref.Anchor != "" {
		attrs["anchor"] = ref.Anchor
	}
	return attrs
}

func (c *Corpus) inspect(attrs map[string]string, local, anchor string) {
	_, err := os.Stat(local)
	switch {
	case errors.Is(err, os.ErrNotExist):
		attrs["exists"] = "false"
		return
	case err != nil:
		attrs["problem"] = fmt.Sprintf("inspect local reference %s: %v", attrs["dest"], err)
		return
	}
	attrs["exists"] = "true"
	if anchor == "" || !strings.EqualFold(filepath.Ext(local), ".md") {
		return
	}
	anchors, err := c.headingAnchors(local)
	if err != nil {
		attrs["problem"] = err.Error()
		return
	}
	if anchors != nil {
		attrs["anchored"] = fmt.Sprint(anchors[anchor])
	}
}

func (c *Corpus) headingAnchors(path string) (map[string]bool, error) {
	if anchors, ok := c.anchors[path]; ok {
		return anchors, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read reference %s: %w", path, err)
	}
	art, err := artifact.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse reference %s: %w", path, err)
	}
	if strings.Contains(art.Body, "{{") || strings.Contains(art.Body, "id=") || strings.Contains(art.Body, "name=") {
		c.anchors[path] = nil
		return nil, nil
	}
	sections, err := markup.Sections([]byte(art.Body))
	if err != nil {
		return nil, err
	}
	anchors := map[string]bool{}
	for _, section := range sections {
		anchors[section.Anchor] = true
	}
	c.anchors[path] = anchors
	return anchors, nil
}

func (c *Corpus) resolveLinks(file *markup.Element, self, rel, physical string) {
	file.Walk(func(e *markup.Element) bool {
		if e.Type != "link" && e.Type != "image" || e.Attrs["url"] == "" || strings.Contains(e.Attrs["url"], "{{") {
			return true
		}
		e.Attrs["dest"] = e.Attrs["url"]
		u, err := url.Parse(e.Attrs["url"])
		if err != nil {
			e.Attrs["valid"] = "false"
			return true
		}
		if u.IsAbs() || u.Host != "" || strings.HasPrefix(u.Path, "~") || filepath.IsAbs(u.Path) {
			return true
		}
		local := physical
		if u.Path != "" {
			local = filepath.Join(filepath.Dir(physical), filepath.FromSlash(u.Path))
		}
		c.inspect(e.Attrs, local, u.Fragment)
		if self == "" || e.Type == "image" {
			return true
		}
		destination, anchor, _ := strings.Cut(e.Attrs["url"], "#")
		target, inside := self, rel
		if destination != "" {
			inside = path.Clean(path.Join(path.Dir(rel), destination))
		}
		if outside, escapes := strings.CutPrefix(inside, "../"); escapes {
			name, resource, _ := strings.Cut(outside, "/")
			skill := c.Skill(name)
			if skill == nil {
				return true
			}
			target, inside = skill.Attrs["id"], cmp.Or(resource, "SKILL.md")
		}
		e.Attrs["target"], e.Attrs["path"] = target, inside
		if anchor != "" {
			e.Attrs["anchor"] = anchor
		}
		return true
	})
}

func (c *Corpus) adopt(parent, child *markup.Element) {
	child.Parent, child.Index = parent, len(parent.Children)
	parent.Children = append(parent.Children, child)
}
