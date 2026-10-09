package lint

import (
	"bytes"
	"maps"
	"slices"
	"strconv"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/slots"
)

func addSlots(root *markup.Element, documents []document) {
	files := map[string]*markup.Element{}
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "file" {
			files[e.Attrs["file"]] = e
		}
		return true
	})
	for _, d := range documents {
		file := files[d.path]
		if file == nil || len(file.Children) == 0 || file.Children[0].Type != "frontmatter" {
			continue
		}
		head := file.Children[0]
		add := func(attrs map[string]string, offset int) {
			line := bytes.Count(d.source[:offset], []byte{'\n'}) + 1
			column := offset - bytes.LastIndexByte(d.source[:offset], '\n')
			head.Children = append(head.Children, &markup.Element{Type: "slot", Attrs: attrs, Text: attrs["slot"], Line: line, EndLine: line, Column: column, Parent: head, Index: len(head.Children)})
		}
		inFrontmatter := func(slot string) int {
			if index := bytes.Index(d.source[:d.offset], []byte(slot)); index >= 0 {
				return index
			}
			return 0
		}
		body := d.art.Body
		if masked, err := markup.MaskTemplates([]byte(body)); err == nil {
			body = string(masked)
		}
		metadata := slots.Read(d.art.Frontmatter, body, slots.Normal)
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
