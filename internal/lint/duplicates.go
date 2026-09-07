package lint

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

func sourceLocation(d document, offset int) Location {
	offset = min(max(offset, 0), len(d.source))
	return Location{Path: d.path, Line: bytes.Count(d.source[:offset], []byte{'\n'}) + 1, Column: offset - bytes.LastIndexByte(d.source[:offset], '\n')}
}

func (c *checker) addDuplicate(d document, offset int, rule, message string, first Location) {
	previous := len(c.diagnostics)
	c.add(d, offset, "warning", rule, message)
	if len(c.diagnostics) > previous {
		c.diagnostics[previous].Related = []Location{first}
	}
}

func (c *checker) checkDuplicates(d document, markupSource []byte) {
	similarity := c.options.DuplicateSimilarity > 0 && !slices.Contains(c.options.Disable, "similar-content")
	if slices.Contains(c.options.Disable, "duplicate-content") && !similarity {
		return
	}
	nodes, err := markup.Inspect(markupSource)
	if err != nil {
		return
	} // The markup checker already reports malformed input.
	for _, node := range nodes {
		if node.Kind != "paragraph" {
			continue
		}
		source := string(d.body[node.Start:node.End])
		if strings.Contains(source, "{{") || len(strings.Fields(node.Text)) < c.options.DuplicateMinWords {
			continue
		}
		// Keep Markdown syntax in the identity: equal link labels with different
		// destinations or different inline directive attributes are not copies.
		key := normalize(source)
		offset := d.offset + node.Start
		if first, ok := c.paragraphs[key]; ok {
			c.addDuplicate(d, offset, "duplicate-content", fmt.Sprintf("paragraph duplicates %s:%d", first.Path, first.Line), Location{Path: first.Path, Line: first.Line, Column: first.Column})
		} else {
			location := sourceLocation(d, offset)
			c.paragraphs[key] = Diagnostic{Path: location.Path, Line: location.Line, Column: location.Column}
			if similarity {
				current := newParagraph(key, location)
				if first, score, ok := closestParagraph(current, c.similarParagraphs, c.options.DuplicateSimilarity); ok {
					c.addDuplicate(d, offset, "similar-content", fmt.Sprintf("paragraph is %.1f%% similar to %s:%d (normalized Levenshtein)", score*100, first.Path, first.Line), first)
					c.diagnostics[len(c.diagnostics)-1].Similarity = new(score)
				}
				c.similarParagraphs = append(c.similarParagraphs, current)
			}
		}
	}
}
