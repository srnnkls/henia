package similarity

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/townsendmerino/aikit/embed"
)

func Words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r)
	})
}

func Shingles(text string, size int) map[string]bool {
	words := Words(text)
	shingles := map[string]bool{}
	for i := 0; i+size <= len(words); i++ {
		shingles[strings.Join(words[i:i+size], " ")] = true
	}
	return shingles
}

func Jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for shingle := range a {
		if b[shingle] {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

func Containment(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for shingle := range a {
		if b[shingle] {
			shared++
		}
	}
	return float64(shared) / float64(min(len(a), len(b)))
}

func Shared(a, b map[string]bool, limit int) []string {
	var phrases []string
	for shingle := range a {
		if b[shingle] {
			phrases = append(phrases, shingle)
		}
	}
	slices.Sort(phrases)
	return phrases[:min(limit, len(phrases))]
}

func Unit(raw []float32) ([]float64, error) {
	norm := 0.0
	for _, value := range raw {
		x := float64(value)
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, fmt.Errorf("semantic model returned a non-finite embedding")
		}
		norm = math.Hypot(norm, x)
	}
	if norm == 0 {
		return nil, nil
	}
	vector := make([]float64, len(raw))
	for i, value := range raw {
		vector[i] = float64(value) / norm
	}
	return vector, nil
}

func Cosine(a, b []float64) float64 {
	score := 0.0
	for i, value := range a {
		score += value * b[i]
	}
	return min(1, max(-1, score))
}

func Model(path string) (func(string) []float32, error) {
	model, err := embed.LoadFromFS(os.DirFS(path), ".")
	if err != nil {
		return nil, fmt.Errorf("load semantic model %q: %w", path, err)
	}
	if model.Dim() == 0 {
		return nil, fmt.Errorf("semantic model has no embedding dimensions")
	}
	return model.Encode, nil
}
