package lint

import (
	"slices"
	"strings"
	"unicode"
)

type paragraph struct {
	text         string
	shingles     map[string]bool
	location     Location
	lexicalMatch bool
}

func newParagraph(source string, location Location, size int) paragraph {
	words := strings.FieldsFunc(strings.ToLower(source), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
	p := paragraph{text: strings.Join(strings.Fields(source), " "), shingles: map[string]bool{}, location: location}
	for i := 0; i+size <= len(words); i++ {
		p.shingles[strings.Join(words[i:i+size], " ")] = true
	}
	return p
}

type paragraphMatch struct {
	index  int
	score  float64
	method string
}

// Compare only paragraphs sharing at least one shingle. Jaccard takes priority;
// containment catches partial copies when whole-paragraph overlap is too low.
func (c *checker) closestLexical(current paragraph) (paragraphMatch, bool) {
	intersections := map[int]int{}
	for shingle := range current.shingles {
		for _, index := range c.shingleIndex[shingle] {
			intersections[index]++
		}
	}
	indices := make([]int, 0, len(intersections))
	for index := range intersections {
		indices = append(indices, index)
	}
	slices.Sort(indices)
	bestJaccard, bestContainment := paragraphMatch{index: -1}, paragraphMatch{index: -1}
	for _, index := range indices {
		intersection := intersections[index]
		previous := c.similarParagraphs[index]
		jaccard := float64(intersection) / float64(len(current.shingles)+len(previous.shingles)-intersection)
		containment := float64(intersection) / float64(min(len(current.shingles), len(previous.shingles)))
		if c.options.DuplicateSimilarity > 0 && jaccard >= c.options.DuplicateSimilarity && jaccard > bestJaccard.score {
			bestJaccard = paragraphMatch{index, jaccard, "jaccard"}
		}
		if c.options.DuplicateContainment > 0 && containment >= c.options.DuplicateContainment && containment > bestContainment.score {
			bestContainment = paragraphMatch{index, containment, "containment"}
		}
	}
	if bestJaccard.index >= 0 {
		return bestJaccard, true
	}
	return bestContainment, bestContainment.index >= 0
}

func sharedPhrases(a, b paragraph) []string {
	var phrases []string
	for shingle := range a.shingles {
		if b.shingles[shingle] {
			phrases = append(phrases, shingle)
		}
	}
	slices.Sort(phrases)
	return phrases[:min(5, len(phrases))]
}
