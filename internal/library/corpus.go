package library

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
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

func (l *Library) Corpus(rendering *Rendering) *Corpus {
	c := &Corpus{Root: &markup.Element{Type: "corpus", Attrs: map[string]string{}}, lib: l, skills: map[string]*markup.Element{}, rendering: rendering}
	for _, e := range l.Entries {
		skill := &markup.Element{Type: "skill", Attrs: map[string]string{"id": e.Name, "source": e.Source, "ref": l.Reference(e)}}
		c.adopt(c.Root, skill)
		c.skills[e.ID] = skill
		dir := filepath.Dir(e.Path)
		c.file(skill, e, e.Path, "SKILL.md")
		for _, path := range resourceFiles(dir) {
			rel, _ := filepath.Rel(dir, path)
			c.file(skill, e, path, filepath.ToSlash(rel))
		}
	}
	return c
}

func (c *Corpus) Skill(ref string) *markup.Element {
	e, err := c.lib.Resolve(ref)
	if err != nil {
		return nil
	}
	return c.skills[e.ID]
}

func (c *Corpus) file(skill *markup.Element, entry Entry, path, rel string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > maxCorpusFile || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return
	}
	var file *markup.Element
	var body string
	offset, shift := 0, 0
	var frontmatter map[string]any
	if strings.EqualFold(filepath.Ext(path), ".md") {
		art, err := artifact.Parse(data)
		if err != nil {
			c.Problems = append(c.Problems, fmt.Sprintf("%s/%s: %v", skill.Attrs["ref"], rel, err))
			return
		}
		offset, frontmatter = len(data)-len(art.Body), art.Frontmatter
		body, shift = art.Body, bytes.Count(data[:offset], []byte{'\n'})
		var syntax *regexp.Regexp
		if rel == "SKILL.md" && c.rendering != nil {
			body, shift, syntax = c.rendering.Body(entry), 0, c.rendering.Reference(entry)
		}
		file, err = markup.Tree([]byte(body), c.references(body, syntax)...)
		if err != nil {
			c.Problems = append(c.Problems, fmt.Sprintf("%s/%s: %v", skill.Attrs["ref"], rel, err))
		}
		c.resolveLinks(file, skill.Attrs["id"], rel)
	} else {
		file = markup.PlainTree(data)
	}
	file.Attrs["path"], file.Attrs["main"] = rel, fmt.Sprint(rel == "SKILL.md")
	if shift > 0 {
		file.Walk(func(e *markup.Element) bool {
			e.Line, e.EndLine = e.Line+shift, e.EndLine+shift
			return true
		})
	}
	if offset > 0 {
		attrs := map[string]string{}
		for key, value := range frontmatter {
			switch value.(type) {
			case string, bool, int, float64:
				attrs[key] = fmt.Sprint(value)
			}
		}
		head := &markup.Element{Type: "frontmatter", Attrs: attrs, Text: strings.TrimSpace(strings.Trim(strings.TrimSpace(string(data[:offset])), "-"))}
		if shift > 0 {
			head.Line, head.EndLine = 1, shift
		}
		file.Children = append([]*markup.Element{head}, file.Children...)
		for i, child := range file.Children {
			child.Parent, child.Index = file, i
		}
	}
	c.adopt(skill, file)
}

func (c *Corpus) references(body string, syntax *regexp.Regexp) []*markup.Element {
	refs := reference.Skills(body)
	if syntax != nil {
		for _, m := range syntax.FindAllStringSubmatchIndex(body, -1) {
			refs = append(refs, reference.Reference{Name: body[m[2]:m[3]], Start: m[0], End: m[1]})
		}
	}
	var links []*markup.Element
	for _, ref := range refs {
		target := ref.Name
		if e, err := c.lib.Resolve(ref.Name); err == nil {
			target = e.Name
		}
		attrs := map[string]string{"target": target, "path": "SKILL.md"}
		if ref.Resource != "" {
			attrs["path"] = ref.Resource
		}
		if ref.Anchor != "" {
			attrs["anchor"] = ref.Anchor
		}
		links = append(links, &markup.Element{Type: "link", Attrs: attrs, Start: ref.Start, End: ref.End})
	}
	return links
}

func (c *Corpus) resolveLinks(file *markup.Element, self, rel string) {
	file.Walk(func(e *markup.Element) bool {
		destination, anchor, _ := strings.Cut(e.Attrs["url"], "#")
		if e.Type != "link" || e.Attrs["url"] == "" || strings.Contains(destination, ":") {
			return true
		}
		target, inside := self, rel
		if destination != "" {
			inside = path.Clean(path.Join(path.Dir(rel), destination))
		}
		if outside, escapes := strings.CutPrefix(inside, "../"); escapes {
			name, resource, _ := strings.Cut(outside, "/")
			entry, err := c.lib.Resolve(name)
			if err != nil {
				return true
			}
			target, inside = entry.Name, cmp.Or(resource, "SKILL.md")
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
