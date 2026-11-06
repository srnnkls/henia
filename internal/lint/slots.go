package lint

import (
	"bytes"
	"maps"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/slots"
)

func addSlots(root *markup.Element, documents []document, provided []slots.Skill, setup *SlotSetup) {
	var declarations []*markup.Element
	var scanned []slots.Skill
	files := map[string]*markup.Element{}
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "file" {
			files[e.Attrs["file"]] = e
		}
		return true
	})
	for _, d := range documents {
		file := files[d.path]
		if file == nil {
			continue
		}
		head := file
		switch {
		case len(file.Children) > 0 && file.Children[0].Type == "frontmatter":
			head = file.Children[0]
		case file.Attrs["dependency"] != "true":
			continue
		}
		add := func(attrs map[string]string, offset int) {
			line := bytes.Count(d.source[:offset], []byte{'\n'}) + 1
			column := offset - bytes.LastIndexByte(d.source[:offset], '\n')
			e := &markup.Element{Type: "slot", Attrs: attrs, Text: attrs["slot"], Line: line, EndLine: line, Column: column, Parent: head, Index: len(head.Children)}
			head.Children = append(head.Children, e)
			if attrs["role"] == "declare" {
				declarations = append(declarations, e)
			}
		}
		inFrontmatter := func(slot string) int {
			if index := bytes.Index(d.source[:d.offset], []byte(slot)); index >= 0 {
				return index
			}
			return 0
		}
		body := d.art.Body
		metadata := slots.Read(d.art.Frontmatter, body, slots.Normal)
		tier := slots.Project
		if head == file {
			tier = slots.Dependency
		}
		scanned = append(scanned, slots.Skill{Path: d.path, Ref: filepath.Base(filepath.Dir(d.path)), Tier: tier, Metadata: metadata})
		if head == file {
			for _, declared := range metadata.Declared {
				add(map[string]string{"role": "declare", "slot": declared.Slot, "type": declared.Type.String()}, inFrontmatter(declared.Slot))
			}
			continue
		}
		for _, problem := range metadata.Problems {
			add(map[string]string{"role": problem.Role, "slot": problem.Slot, "error": problem.Message}, inFrontmatter(problem.Slot))
		}
		for _, declared := range metadata.Declared {
			add(map[string]string{"role": "declare", "slot": declared.Slot, "type": declared.Type.String()}, inFrontmatter(declared.Slot))
		}
		var anchors map[string]bool
		for _, offered := range metadata.Offered {
			attrs := map[string]string{"role": "provide", "slot": offered.Slot, "priority": offered.Priority.String()}
			at := inFrontmatter(offered.Slot)
			if offered.Section != "" {
				if anchors == nil {
					anchors = sectionAnchors(d)
				}
				attrs["section"], attrs["found"] = offered.Section, strconv.FormatBool(anchors[offered.Section])
				if provides := bytes.Index(d.source[:d.offset], []byte("provides:")); provides >= 0 {
					if key := bytes.Index(d.source[provides:d.offset], []byte(offered.Slot)); key >= 0 {
						key += provides + len(offered.Slot)
						if index := bytes.Index(d.source[key:d.offset], []byte(offered.Section)); index >= 0 {
							at = key + index
						}
					}
				}
			}
			add(attrs, at)
		}
		for _, s := range provided {
			if !same(s.Path, d.path) {
				continue
			}
			for _, offered := range s.Offered {
				attrs := map[string]string{"role": "provide", "slot": offered.Slot, "priority": offered.Priority.String()}
				if offered.Section != "" {
					if anchors == nil {
						anchors = sectionAnchors(d)
					}
					attrs["section"], attrs["found"] = offered.Section, strconv.FormatBool(anchors[offered.Section])
				}
				add(attrs, 0)
			}
		}
		if d.view == nil {
			for _, application := range slots.Applications(body) {
				for _, slot := range application.Slots {
					add(map[string]string{"role": "apply", "slot": slot}, d.offset+application.Start)
				}
			}
			continue
		}
		authored := map[string]int{}
		for _, application := range slots.Applications(body) {
			for _, slot := range application.Slots {
				if _, ok := authored[slot]; !ok {
					authored[slot] = d.offset + application.Start
				}
			}
		}
		for _, slot := range slices.Sorted(maps.Keys(d.view.applied)) {
			at, ok := authored[slot]
			if !ok {
				at = d.offset
				if index := bytes.Index(d.source[d.offset:], []byte(slot)); index >= 0 {
					at += index
				}
			}
			add(map[string]string{"role": "apply", "slot": slot}, at)
		}
	}
	resolution := resolve(scanned, provided, setup)
	for _, e := range declarations {
		e.Attrs["provided"] = strconv.FormatBool(served(resolution, e.Attrs["slot"]))
	}
}

// SlotSetup is what henia slots resolves: the library's skills, followed by
// the offers configurations attach to them, and the disabled providers.
type SlotSetup struct {
	Skills   []slots.Skill
	Disabled map[string][]string
}

// resolve evaluates the setup with each scanned skill in place of the library
// copy of the same name; without a setup, provided and the scanned skills.
func resolve(scanned, provided []slots.Skill, setup *SlotSetup) *slots.Resolution {
	if setup == nil {
		return slots.Evaluate(append(slices.Clone(provided), scanned...), nil)
	}
	skills := slices.Clone(setup.Skills)
	replaced := map[string]bool{}
	for _, s := range scanned {
		if s.Tier == slots.Dependency {
			if !slices.ContainsFunc(skills, func(b slots.Skill) bool { return same(b.Path, s.Path) }) {
				skills = append(skills, s)
			}
			continue
		}
		i := -1
		for _, tier := range []slots.Tier{slots.Project, slots.Global} {
			if i < 0 {
				i = slices.IndexFunc(skills, func(b slots.Skill) bool { return b.Tier == tier && b.Name() == s.Name() && !replaced[b.Path] })
			}
		}
		if i < 0 {
			skills = append(skills, s)
			continue
		}
		replaced[skills[i].Path] = true
		skills[i].Metadata = s.Metadata
	}
	return slots.Evaluate(skills, setup.Disabled)
}

func served(r *slots.Resolution, slot string) bool {
	return slices.ContainsFunc(r.Providers, func(p *slots.Provider) bool {
		return p.Status == slots.Selected && (slots.Within(p.Slot, slot) || slots.Within(slot, p.Slot))
	})
}

func sectionAnchors(d document) map[string]bool {
	if d.view != nil && d.view.anchors != nil {
		return d.view.anchors
	}
	anchors := map[string]bool{}
	sections, _ := markup.Sections(d.body)
	for _, section := range sections {
		anchors[section.Anchor] = true
	}
	return anchors
}

func same(a, b string) bool {
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	return errA == nil && errB == nil && a == b
}
