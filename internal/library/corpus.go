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
	"text/template"
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
}

type Rendering struct {
	Body      func(Entry) string
	Reference func(Entry) *regexp.Regexp
}

type Document struct {
	Path string
	Kind string
}

func (l *Library) Corpus(rendering *Rendering) *Corpus {
	c := newCorpus()
	c.lib, c.rendering = l, rendering
	for _, e := range l.Entries {
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": e.Name, "name": e.Name, "source": e.Source, "ref": l.Reference(e)}}
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
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": id, "name": id, "ref": id}}
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
		if skill := dirs[filepath.Dir(d.Path)]; skill != nil && filepath.Base(d.Path) == "SKILL.md" {
			c.file(skill, d.Path, "SKILL.md", d.Kind, nil)
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
			c.file(dirs[owner], d.Path, filepath.ToSlash(rel), "resource", nil)
			continue
		}
		c.file(c.Root, d.Path, filepath.ToSlash(d.Path), cmp.Or(d.Kind, "document"), nil)
	}
	return c
}

func newCorpus() *Corpus {
	return &Corpus{Root: &markup.Element{Type: "corpus", Attrs: map[string]string{}}, skills: map[string]*markup.Element{}}
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
	file, err := markup.Tree([]byte(body), c.references(body, syntax)...)
	var problems []*markup.Element
	if location, ok := errors.AsType[*markup.Error](err); ok {
		problems = append(problems, problem("markup", location.Message, location.Line, location.Column))
	} else if err != nil {
		problems = append(problems, problem("markup", err.Error(), 1, 1))
	}
	if typed && strings.Contains(body, "{{") {
		if _, err := template.New("skill").Parse(body); err != nil {
			problems = append(problems, problem("template", err.Error(), 1, 1))
		}
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
				entry := &markup.Element{Type: "entry", Attrs: map[string]string{"key": key, "value": node.Value}, Text: node.Value, Line: line, EndLine: line, Column: column}
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
			add(ref.Start, ref.End, c.skillLink(ref))
		case reference.TypeCommand, reference.TypeAgent:
			add(ref.Start, ref.End, map[string]string{"ref": string(ref.Type), "artifact": string(ref.Type) + ":" + ref.Name})
		case reference.TypeFile:
			add(ref.Start, ref.End, map[string]string{"ref": "file", "url": ref.Name})
		}
	}
	for _, ref := range reference.Shown(body) {
		add(ref.Start, ref.End+1, c.skillLink(ref))
	}
	if syntax != nil {
		for _, m := range syntax.FindAllStringSubmatchIndex(body, -1) {
			add(m[0], m[1], c.skillLink(reference.Reference{Name: body[m[2]:m[3]]}))
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
	attrs := map[string]string{"ref": "skill", "target": target, "artifact": "skill:" + target}
	if ref.Resource != "" || ref.Anchor != "" {
		attrs["path"] = cmp.Or(ref.Resource, "SKILL.md")
	}
	if ref.Anchor != "" {
		attrs["anchor"] = ref.Anchor
	}
	return attrs
}

func (c *Corpus) resolveLinks(file *markup.Element, self, rel, physical string) {
	file.Walk(func(e *markup.Element) bool {
		destination, anchor, _ := strings.Cut(e.Attrs["url"], "#")
		if e.Type != "link" || e.Attrs["url"] == "" || strings.Contains(e.Attrs["url"], "{{") {
			return true
		}
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
		_, err = os.Stat(local)
		e.Attrs["exists"] = fmt.Sprint(err == nil)
		if self == "" {
			return true
		}
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
