package lint

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func paragraphFiles(t *testing.T, texts ...string) string {
	t.Helper()
	root := t.TempDir()
	for i, text := range texts {
		if err := os.WriteFile(filepath.Join(root, string(rune('a'+i))+".md"), []byte(":::instruction\n"+text+"\n:::\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLexicalParagraphs(t *testing.T) {
	base := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	reorderedA := "Inspect every changed function and report concrete failures. Include enough context to reproduce the problem."
	reorderedB := "Include enough context to reproduce the problem. Inspect every changed function and report concrete failures."
	for _, test := range []struct {
		name, a, b, method            string
		similarity, containment, want float64
	}{
		{"edited", base, strings.Replace(base, "every", "each", 1), "jaccard", 0.7, 0, 11.0 / 15},
		{"reordered", reorderedA, reorderedB, "jaccard", 0.7, 0, 11.0 / 15},
		{"contained", base, "Before opening a pull request, follow these steps. " + base + " Afterwards, summarize your findings clearly for the reviewer.", "containment", 0.9, 1, 1},
		{"contained-reverse", "Additional setup instructions precede the main task. " + base, base, "containment", 0, 1, 1},
		{"different", base, "Prepare the vegetable broth by simmering chopped carrots and onions together in a large covered pot.", "", 0.7, 0.9, 0},
		{"paraphrase", base, "Examine modified routines, identify reproducible defects, and explain each issue clearly so another developer can investigate it.", "", 0.7, 0.9, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := paragraphFiles(t, test.a, test.b)
			diagnostics, err := Run(t.Context(), []string{root}, Options{DuplicateSimilarity: test.similarity, DuplicateContainment: test.containment})
			if err != nil {
				t.Fatal(err)
			}
			if test.method == "" {
				if len(diagnostics) != 0 {
					t.Fatalf("unexpected match: %+v", diagnostics)
				}
				return
			}
			if len(diagnostics) != 1 {
				t.Fatalf("%+v", diagnostics)
			}
			d := diagnostics[0]
			if d.Rule != "similar-content" || d.Method != test.method || d.Similarity == nil || math.Abs(*d.Similarity-test.want) > 1e-10 || d.Line != 2 || len(d.Related) != 1 || d.Related[0].Path != filepath.Join(root, "a.md") || len(d.SharedPhrases) == 0 {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestLexicalThresholdsAndDisabling(t *testing.T) {
	one := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	root := paragraphFiles(t, one, strings.Replace(one, "every", "each", 1))
	for _, options := range []Options{{}, {DuplicateSimilarity: 1}, {DuplicateSimilarity: 0.7, DuplicateMinWords: 100}, {DuplicateSimilarity: 0.7, Disable: []string{"similar-content"}}} {
		diagnostics, err := Run(t.Context(), []string{root}, options)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("%+v: %+v (%v)", options, diagnostics, err)
		}
	}
	for _, threshold := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		for _, options := range []Options{{DuplicateSimilarity: threshold}, {DuplicateContainment: threshold}, {Semantic: SemanticOptions{Threshold: threshold}}} {
			if err := options.Validate(); err == nil {
				t.Fatalf("accepted %+v", options)
			}
		}
	}
	if err := (Options{DuplicateShingleWords: -1}).Validate(); err == nil {
		t.Fatal("accepted negative shingle size")
	}
}

func TestShinglesUnicodeAndMultiplicity(t *testing.T) {
	p := newParagraph("CAFÉ naïve résumé! café naïve résumé", Location{}, 3)
	if len(p.shingles) != 3 || !p.shingles["café naïve résumé"] {
		t.Fatalf("%v", p.shingles)
	}
	if len(newParagraph("too short", Location{}, 3).shingles) != 0 {
		t.Fatal("short input must not create a partial shingle")
	}
}

func TestLexicalClosestMatchAndStableTie(t *testing.T) {
	c := checker{options: Options{DuplicateSimilarity: 0.5}, shingleIndex: map[string][]int{}}
	for _, text := range []string{"one two three four x y", "one two three four five x", "one two three four five y"} {
		p := newParagraph(text, Location{}, 2)
		for shingle := range p.shingles {
			c.shingleIndex[shingle] = append(c.shingleIndex[shingle], len(c.similarParagraphs))
		}
		c.similarParagraphs = append(c.similarParagraphs, p)
	}
	p := newParagraph("one two three four five six", Location{}, 2)
	match, ok := c.closestLexical(p)
	if !ok || match.index != 1 || math.Abs(match.score-2.0/3) > 1e-10 {
		t.Fatalf("%+v %v", match, ok)
	}
	phrases := sharedPhrases(p, c.similarParagraphs[match.index])
	if !slices.IsSorted(phrases) {
		t.Fatalf("unstable explanation: %v", phrases)
	}
	c.options.DuplicateSimilarity = 0.67
	if _, ok := c.closestLexical(p); ok {
		t.Fatal("matched below threshold")
	}
}

func TestExactTakesPriorityOverLexical(t *testing.T) {
	p := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	root := paragraphFiles(t, p, strings.ToUpper(p))
	diagnostics, err := Run(t.Context(), []string{root}, Options{DuplicateSimilarity: 0.5, DuplicateContainment: 0.5})
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Rule != "duplicate-content" {
		t.Fatalf("%+v (%v)", diagnostics, err)
	}
}
