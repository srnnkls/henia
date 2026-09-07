package lint

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/townsendmerino/aikit/embed"
)

func TestSemanticLocalModel(t *testing.T) {
	root := paragraphFiles(t, "Repair broken program.", "Fix faulty code.", "Simmer vegetable soup.", "Repair broken program.")
	options := Options{DuplicateMinWords: 1, DuplicateSimilarity: 0.7, Semantic: SemanticOptions{Enabled: true, ModelPath: "testdata/model", Threshold: 0.85}}
	diagnostics, err := Run(t.Context(), []string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("%+v", diagnostics)
	}
	d := diagnostics[0]
	if d.Rule != "semantic-content" || d.Method != "cosine" || d.Model != "testdata/model" || d.Similarity == nil || *d.Similarity != 1 || len(d.Related) != 1 || d.Related[0].Path != filepath.Join(root, "a.md") || d.Path != filepath.Join(root, "b.md") || d.Line != 2 {
		t.Fatalf("%+v", d)
	}
	if diagnostics[1].Rule != "duplicate-content" {
		t.Fatalf("%+v", diagnostics)
	}
}

func TestSemanticDoesNotLoadWhenUnneeded(t *testing.T) {
	for _, test := range []struct {
		name     string
		texts    []string
		disabled []string
		enabled  bool
		lexical  float64
		want     int
	}{
		{"off", []string{"repair broken program", "fix faulty code"}, nil, false, 0, 0},
		{"disabled", []string{"repair broken program", "fix faulty code"}, []string{"semantic-content"}, true, 0, 0},
		{"single", []string{"repair broken program"}, nil, true, 0, 0},
		{"exact", []string{"repair broken program", "repair broken program"}, nil, true, 0, 1},
		{"lexical", []string{"repair broken program and review changes", "repair broken program and review fixes"}, nil, true, 0.5, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := paragraphFiles(t, test.texts...)
			diagnostics, err := Run(t.Context(), []string{root}, Options{DuplicateMinWords: 1, DuplicateSimilarity: test.lexical, Disable: test.disabled, Semantic: SemanticOptions{Enabled: test.enabled, Threshold: 0.8, ModelPath: filepath.Join(t.TempDir(), "missing")}})
			if err != nil || len(diagnostics) != test.want {
				t.Fatalf("%+v (%v)", diagnostics, err)
			}
		})
	}
}

func TestSemanticModelErrors(t *testing.T) {
	root := paragraphFiles(t, "repair broken program", "fix faulty code")
	if err := (Options{Semantic: SemanticOptions{Enabled: true}}).Validate(); err == nil {
		t.Fatal("accepted missing model configuration")
	}
	_, err := Run(t.Context(), []string{root}, Options{DuplicateMinWords: 1, Semantic: SemanticOptions{Enabled: true, Threshold: 0.8, ModelPath: t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), "load semantic model") {
		t.Fatalf("missing model: %v", err)
	}
	modelDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modelDir, "tokenizer.json"), []byte("broken json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = Run(t.Context(), []string{root}, Options{DuplicateMinWords: 1, Semantic: SemanticOptions{Enabled: true, Threshold: 0.8, ModelPath: modelDir}})
	if err == nil {
		t.Fatal("accepted malformed local model")
	}
}

func TestSemanticZeroVectorAndCancellation(t *testing.T) {
	root := paragraphFiles(t, "repair broken program", "zzzzzzzzzzzzzzzz")
	diagnostics, err := Run(t.Context(), []string{root}, Options{DuplicateMinWords: 1, Semantic: SemanticOptions{Enabled: true, ModelPath: "testdata/model", Threshold: 0.8}})
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Similarity != nil || !strings.Contains(diagnostics[0].Message, "zero embedding") {
		t.Fatalf("%+v (%v)", diagnostics, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	c := checker{similarParagraphs: []paragraph{{text: "one"}, {text: "two"}}}
	err = c.compareSemantic(ctx, func(string) []float32 { cancel(); return []float32{1, 0} })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestSemanticVectorValidationAndCosine(t *testing.T) {
	for _, v := range [][]float32{{float32(math.NaN())}, {float32(math.Inf(1))}} {
		if _, err := unitVector(v); err == nil {
			t.Fatal("accepted non-finite vector")
		}
	}
	a, err := unitVector([]float32{3, 4})
	if err != nil {
		t.Fatal(err)
	}
	b, err := unitVector([]float32{0, 2})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(cosine(a, b)-0.8) > 1e-12 {
		t.Fatal(cosine(a, b))
	}
	c := checker{similarParagraphs: []paragraph{{text: "one"}, {text: "two"}}}
	calls := 0
	if err := c.compareSemantic(t.Context(), func(string) []float32 { calls++; return make([]float32, calls) }); err == nil {
		t.Fatal("accepted inconsistent dimensions")
	}
}

func TestSemanticClosestTieAndLexicalSuppression(t *testing.T) {
	c := checker{options: Options{Semantic: SemanticOptions{Threshold: 0.8}}, similarParagraphs: []paragraph{
		{text: "weaker", location: Location{Path: "a"}},
		{text: "closest", location: Location{Path: "b"}, lexicalMatch: true},
		{text: "tie", location: Location{Path: "c"}, lexicalMatch: true},
		{text: "current", location: Location{Path: "d"}},
	}}
	vectors := map[string][]float32{"weaker": {3, 4}, "closest": {0, 1}, "tie": {0, 2}, "current": {0, 3}}
	if err := c.compareSemantic(t.Context(), func(s string) []float32 { return vectors[s] }); err != nil {
		t.Fatal(err)
	}
	if len(c.diagnostics) != 1 || c.diagnostics[0].Related[0].Path != "b" {
		t.Fatalf("%+v", c.diagnostics)
	}
}

// Run explicitly against an installed model; ordinary tests never download one.
func TestSemanticPretrainedModel(t *testing.T) {
	path := os.Getenv("HENIA_TEST_MODEL")
	if path == "" {
		t.Skip("set HENIA_TEST_MODEL to a local Potion model directory")
	}
	model, err := embed.LoadFromFS(os.DirFS(path), ".")
	if err != nil {
		t.Fatal(err)
	}
	texts := []string{
		"Inspect every changed function and report concrete failures with enough context to reproduce the problem.",
		"Examine modified routines, identify reproducible defects, and explain each issue clearly so another developer can investigate it.",
		"Prepare the vegetable broth by simmering chopped carrots and onions together in a large covered pot.",
		"Do not inspect changed functions or report failures, and never provide steps to reproduce the problem.",
	}
	vectors := make([][]float64, len(texts))
	for i, text := range texts {
		vectors[i], err = unitVector(model.Encode(text))
		if err != nil || vectors[i] == nil {
			t.Fatalf("embed: %v", err)
		}
	}
	paraphrase, unrelated, negation := cosine(vectors[0], vectors[1]), cosine(vectors[0], vectors[2]), cosine(vectors[0], vectors[3])
	t.Logf("model=%s dimensions=%d paraphrase=%.4f unrelated=%.4f negation=%.4f", path, model.Dim(), paraphrase, unrelated, negation)
	if paraphrase <= unrelated+0.1 {
		t.Fatalf("paraphrase %.4f does not separate from unrelated %.4f", paraphrase, unrelated)
	}
	root := paragraphFiles(t, texts[:3]...)
	diagnostics, err := Run(t.Context(), []string{root}, Options{Semantic: SemanticOptions{Enabled: true, ModelPath: path, Threshold: (paraphrase + unrelated) / 2}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range diagnostics {
		if d.Rule == "semantic-content" && d.Path == filepath.Join(root, "b.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing paraphrase candidate: %+v", diagnostics)
	}
}
