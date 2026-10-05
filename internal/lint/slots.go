package lint

import (
	"bytes"

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
		metadata := slots.Read(d.art.Frontmatter, d.art.Body, slots.Normal)
		for _, problem := range metadata.Problems {
			add(map[string]string{"role": problem.Role, "slot": problem.Slot, "error": problem.Message}, inFrontmatter(problem.Slot))
		}
		for _, declared := range metadata.Declared {
			add(map[string]string{"role": "declare", "slot": declared.Slot, "type": declared.Type.String()}, inFrontmatter(declared.Slot))
		}
		for _, offered := range metadata.Offered {
			add(map[string]string{"role": "provide", "slot": offered.Slot, "priority": offered.Priority.String()}, inFrontmatter(offered.Slot))
		}
		for _, application := range slots.Applications(d.art.Body) {
			for _, slot := range application.Slots {
				add(map[string]string{"role": "apply", "slot": slot}, d.offset+application.Start)
			}
		}
	}
}
