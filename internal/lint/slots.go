package lint

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/slots"
)

func (c *checker) checkSlots(documents []document) {
	type declaration struct {
		document document
		offset   int
		slot     slots.Declaration
	}
	var declarations []declaration
	for _, d := range documents {
		entries, err := slots.Entries(d.art.Frontmatter)
		if err != nil {
			c.add(d, 0, "error", "invalid-slot", err.Error())
			continue
		}
		for _, entry := range entries.Declared {
			if strings.Contains(entry, "{{") {
				continue
			}
			offset := entryOffset(d, entry)
			parsed, err := slots.ParseDeclaration(entry)
			if err != nil {
				c.add(d, offset, "error", "invalid-slot", err.Error())
				continue
			}
			declarations = append(declarations, declaration{d, offset, parsed})
		}
		for _, entry := range entries.Provided {
			if strings.Contains(entry, "{{") {
				continue
			}
			if _, err := slots.ParseDefinition(entry, slots.Normal); err != nil {
				c.add(d, entryOffset(d, entry), "error", "invalid-slot", err.Error())
			}
		}
		for _, entry := range entries.Applied {
			if strings.Contains(entry, "{{") {
				continue
			}
			if _, err := slots.ParseApplication(entry); err != nil {
				c.add(d, entryOffset(d, entry), "error", "invalid-slot", err.Error())
			}
		}
	}
	types := make(map[string]map[slots.Type]bool)
	for _, d := range declarations {
		if types[d.slot.Slot] == nil {
			types[d.slot.Slot] = make(map[slots.Type]bool)
		}
		types[d.slot.Slot][d.slot.Type] = true
	}
	for _, d := range declarations {
		if len(types[d.slot.Slot]) > 1 {
			var found []string
			for t := range types[d.slot.Slot] {
				found = append(found, t.String())
			}
			slices.Sort(found)
			c.add(d.document, d.offset, "error", "invalid-slot", fmt.Sprintf("slot %s is declared with conflicting types %s", d.slot.Slot, strings.Join(found, ", ")))
		}
	}
}

func entryOffset(d document, entry string) int {
	if index := bytes.Index(d.source[:d.offset], []byte(entry)); index >= 0 {
		return index
	}
	return 0
}
