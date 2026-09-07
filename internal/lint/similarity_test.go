package lint

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundedLevenshtein(t *testing.T) {
	for _, test := range []struct {
		a, b     string
		distance int
	}{
		{"", "", 0}, {"", "abc", 3}, {"kitten", "sitting", 3},
		{"café", "cafe", 1}, {"😀ab", "😃ab", 1}, {"abc", "abc", 0},
		{"abc", "xyz", 3}, {"ab", "ba", 2}, {"same words", "same words!", 1},
	} {
		for limit := 0; limit <= test.distance+1; limit++ {
			want := min(test.distance, limit+1)
			if got := boundedLevenshtein([]rune(test.a), []rune(test.b), limit); got != want {
				t.Fatalf("distance(%q,%q, limit=%d)=%d; want %d", test.a, test.b, limit, got, want)
			}
		}
	}
}

func TestBoundedDistanceMatchesFullMatrix(t *testing.T) {
	words := []string{""}
	for range 4 {
		previous := append([]string(nil), words...)
		for _, word := range previous {
			for _, letter := range []string{"a", "b", "é"} {
				words = append(words, word+letter)
			}
		}
	}
	for _, a := range words {
		for _, b := range words {
			distance := referenceDistance([]rune(a), []rune(b))
			for limit := range 5 {
				if got := boundedLevenshtein([]rune(a), []rune(b), limit); got != min(distance, limit+1) {
					t.Fatalf("%q / %q limit %d: got %d want %d", a, b, limit, got, min(distance, limit+1))
				}
			}
		}
	}
}

func referenceDistance(a, b []rune) int {
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			matrix[i][j] = min(matrix[i-1][j]+1, matrix[i][j-1]+1, matrix[i-1][j-1]+cost)
		}
	}
	return matrix[len(a)][len(b)]
}

func TestSimilarParagraphsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	one := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	two := strings.Replace(one, "every", "each", 1)
	for name, text := range map[string]string{"a": one, "b": two} {
		if err := os.WriteFile(filepath.Join(root, name+".md"), []byte(":::instruction\n"+text+"\n:::\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	diagnostics, err := Run(t.Context(), []string{root}, Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("similarity should be opt-in: %+v (%v)", diagnostics, err)
	}
	diagnostics, err = Run(t.Context(), []string{root}, Options{DuplicateSimilarity: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "similar-content" || diagnostics[0].Line != 2 || diagnostics[0].Similarity == nil || *diagnostics[0].Similarity < 0.9 || *diagnostics[0].Similarity >= 1 || diagnostics[0].Related[0].Path != filepath.Join(root, "a.md") {
		t.Fatalf("%+v", diagnostics)
	}
	for _, options := range []Options{{DuplicateSimilarity: 1}, {DuplicateSimilarity: 0.9, DuplicateMinWords: 100}, {DuplicateSimilarity: 0.9, Disable: []string{"similar-content"}}} {
		diagnostics, err := Run(t.Context(), []string{root}, options)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("unexpected warning for %+v: %+v (%v)", options, diagnostics, err)
		}
	}
}

func TestSimilarityThresholdBoundaryAndClosestMatch(t *testing.T) {
	current := newParagraph("abcdefghij", Location{})
	candidates := []paragraph{newParagraph("abcdefghzz", Location{Path: "less-similar"}), newParagraph("abcdefghiz", Location{Path: "closest"}), newParagraph("abcdefghix", Location{Path: "later-tie"})}
	first, score, ok := closestParagraph(current, candidates, 0.9)
	if !ok || score != 0.9 || first.Path != "closest" {
		t.Fatalf("%+v %f %v", first, score, ok)
	}
	if _, _, ok := closestParagraph(current, candidates, 0.91); ok {
		t.Fatal("matched below threshold")
	}
	for _, threshold := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		if err := (Options{DuplicateSimilarity: threshold}).Validate(); err == nil {
			t.Fatalf("accepted %v", threshold)
		}
	}
}
