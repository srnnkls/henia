package lint

import "math"

type paragraph struct {
	text     []rune
	counts   map[rune]int
	location Location
}

func newParagraph(text string, location Location) paragraph {
	p := paragraph{text: []rune(text), counts: map[rune]int{}, location: location}
	for _, r := range p.text {
		p.counts[r]++
	}
	return p
}

func closestParagraph(current paragraph, candidates []paragraph, threshold float64) (Location, float64, bool) {
	var location Location
	best := -1.0
	for _, previous := range candidates {
		length := max(len(current.text), len(previous.text))
		limit := int(math.Floor((1-threshold)*float64(length) + 1e-9))
		if length-min(len(current.text), len(previous.text)) > limit {
			continue
		}
		// Character-count differences bound the required edits, cheaply excluding
		// unrelated paragraphs before the more expensive sequence comparison.
		missing, extra := 0, 0
		for r, count := range current.counts {
			extra += max(0, count-previous.counts[r])
		}
		for r, count := range previous.counts {
			missing += max(0, count-current.counts[r])
		}
		if max(missing, extra) > limit {
			continue
		}
		distance := boundedLevenshtein(current.text, previous.text, limit)
		if distance > limit {
			continue
		}
		score := 1 - float64(distance)/float64(length)
		if score+1e-12 >= threshold && score > best {
			best, location = score, previous.location
		}
	}
	return location, best, best >= 0
}

// boundedLevenshtein computes rune edit distance inside a threshold-width band.
// It returns limit+1 when the true distance exceeds the requested limit.
func boundedLevenshtein(a, b []rune, limit int) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	if len(a)-len(b) > limit {
		return limit + 1
	}
	if len(b) == 0 {
		return len(a)
	}
	infinity := limit + 1
	previous, current := make([]int, len(b)+1), make([]int, len(b)+1)
	for j := range previous {
		previous[j] = min(j, infinity)
	}
	for i := 1; i <= len(a); i++ {
		current[0] = min(i, infinity)
		start, end := max(1, i-limit), min(len(b), i+limit)
		if start > 1 {
			current[start-1] = infinity
		}
		minimum := infinity
		for j := start; j <= end; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
			minimum = min(minimum, current[j])
		}
		if minimum > limit {
			return infinity
		}
		if end < len(b) {
			current[end+1] = infinity
		}
		previous, current = current, previous
	}
	return min(previous[len(b)], infinity)
}
