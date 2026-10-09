package lint

import (
	"regexp"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	builtin "github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pipeline"
	"github.com/srnnkls/henia/internal/slots"
)

type view struct {
	expanded, head *markup.Element
	body           string
	rendered       bool
	offset         int
	origins        []origin
	syntax         *regexp.Regexp
	applied        map[string]bool
	anchors        map[string]bool
}

func (u *unit) view(harness string) *view {
	if u.views[harness] == nil {
		u.views[harness] = &view{applied: map[string]bool{}}
	}
	return u.views[harness]
}

func (v *view) observeSlots(expanded []byte) {
	for _, application := range slots.Applications(string(expanded)) {
		for _, slot := range application.Slots {
			v.applied[slot] = true
		}
	}
}

func (v *view) render(u *unit, pu *pipeline.Unit, hybrid bool) {
	if hybrid {
		pu.Transformer.Head = true
		head, err := pu.Render()
		pu.Transformer.Head = false
		if err == nil {
			v.head = headBlock(u, head.Body)
		}
	}
	rendered, err := pu.Render()
	if err != nil {
		return
	}
	v.body, v.rendered, v.offset = rendered.Body, true, pu.Offset()
	v.origins = origins(pu.Document(), []byte(v.body), pu.Path(), v.offset)
	v.anchors = map[string]bool{}
	sections, _ := markup.Sections([]byte(v.body))
	for _, section := range sections {
		v.anchors[section.Anchor] = true
	}
}

func headBlock(u *unit, body string) *markup.Element {
	text := strings.TrimRight(body, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	head := &markup.Element{Type: "directive", Attrs: map[string]string{"name": markup.HeadBlock}, Text: text, Line: u.offset + 1, Column: 1}
	if tree, _ := markup.Tree([]byte(u.art.Body)); tree != nil {
		tree.Walk(func(e *markup.Element) bool {
			if e.Type == "directive" && e.Attrs["name"] == markup.HeadBlock {
				head.Line, head.Column = u.offset+e.Line, e.Column
				return false
			}
			return true
		})
	}
	head.EndLine = head.Line + strings.Count(text, "\n")
	return head
}

func builtHarnesses(configured map[string]henia.Harness) (map[string]henia.Harness, error) {
	if len(configured) > 0 {
		return configured, nil
	}
	cfg, err := builtin.DefaultConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Harness, nil
}

func (r componentResult) views(harness string) map[string]*view {
	views := map[string]*view{}
	for path, u := range r.units {
		if v := u.views[harness]; v != nil {
			views[path] = v
		}
	}
	return views
}

func renderedDocuments(docs []library.Document, views map[string]*view) []library.Document {
	rendered := slices.Clone(docs)
	for i, d := range rendered {
		if v := views[d.Path]; v != nil && v.rendered {
			rendered[i].Render = func() (string, *regexp.Regexp) { return v.body, v.syntax }
		}
	}
	return rendered
}

func stageFiles(root *markup.Element, views map[string]*view, stage string) {
	var files []*markup.Element
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "file" {
			files = append(files, e)
		}
		return true
	})
	for _, file := range files {
		v := views[file.Attrs["file"]]
		if v == nil {
			continue
		}
		var children []*markup.Element
		switch {
		case stage == StageExpanded && v.expanded != nil:
			if len(file.Children) > 0 && file.Children[0].Type == "frontmatter" {
				children = append(children, file.Children[0])
			}
			children = append(children, v.expanded.Children...)
		case stage == StageRendered && v.rendered:
			for _, child := range file.Children {
				if child.Type == "frontmatter" {
					child.EndLine = v.offset
				} else {
					relocate(child, v.origins, v.offset)
				}
				children = append(children, child)
			}
			if v.head != nil {
				children = append(children, v.head)
			}
		default:
			continue
		}
		file.Children = children
		for i, child := range file.Children {
			child.Parent, child.Index = file, i
		}
	}
}

type perHarness struct {
	order []string
	found map[string]Diagnostic
	in    map[string][]string
}

func (h *perHarness) add(harness string, found []Diagnostic) {
	if h.found == nil {
		h.found, h.in = map[string]Diagnostic{}, map[string][]string{}
	}
	for _, d := range found {
		key := d.key()
		if _, ok := h.found[key]; !ok {
			h.order = append(h.order, key)
			h.found[key] = d
		}
		if !slices.Contains(h.in[key], harness) {
			h.in[key] = append(h.in[key], harness)
		}
	}
}

func (h *perHarness) diagnostics(harnesses int) []Diagnostic {
	var out []Diagnostic
	for _, key := range h.order {
		d := h.found[key]
		if len(h.in[key]) == harnesses {
			out = append(out, d)
			continue
		}
		for _, harness := range slices.Sorted(slices.Values(h.in[key])) {
			d.Harness = harness
			out = append(out, d)
		}
	}
	return out
}
