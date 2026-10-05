package lint

import (
	"bytes"
	"strings"

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
			head.Children = append(head.Children, &markup.Element{Type: "slot", Attrs: attrs, Text: attrs["entry"], Line: line, EndLine: line, Column: column, Parent: head, Index: len(head.Children)})
		}
		entries, err := slots.Entries(d.art.Frontmatter)
		if err != nil {
			add(map[string]string{"role": "metadata", "error": err.Error()}, 0)
			continue
		}
		for _, group := range []struct {
			role, cut string
			entries   []string
			parse     func(string) error
		}{
			{"declare", ":", entries.Declared, func(e string) error { _, err := slots.ParseDeclaration(e); return err }},
			{"provide", "@", entries.Provided, func(e string) error { _, err := slots.ParseDefinition(e, slots.Normal); return err }},
			{"apply", "", entries.Applied, func(e string) error { _, err := slots.ParseApplication(e); return err }},
		} {
			for _, entry := range group.entries {
				if strings.Contains(entry, "{{") {
					continue
				}
				attrs := map[string]string{"role": group.role, "entry": entry, "slot": entry}
				if group.cut != "" {
					attrs["slot"], _, _ = strings.Cut(entry, group.cut)
				}
				if group.role == "declare" {
					if declared, err := slots.ParseDeclaration(entry); err == nil {
						attrs["type"] = declared.Type.String()
					}
				}
				if err := group.parse(entry); err != nil {
					attrs["error"] = err.Error()
				}
				offset := 0
				if index := bytes.Index(d.source[:d.offset], []byte(entry)); index >= 0 {
					offset = index
				}
				add(attrs, offset)
			}
		}
	}
}
