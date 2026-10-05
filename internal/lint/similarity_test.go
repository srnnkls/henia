package lint

import (
	"math"
	"os"
	"path/filepath"
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
			config := map[string]map[string]any{"similar-content": {"similarity": test.similarity, "containment": test.containment}}
			diagnostics, err := Run(t.Context(), []string{root}, Options{Config: config})
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
			if d.Rule != "similar-content" || d.Method != test.method || d.Similarity == nil || math.Abs(*d.Similarity-test.want) > 1e-10 || d.Line != 2 || len(d.Related) != 1 || d.Related[0].Path != filepath.Join(root, "a.md") || len(d.SharedPhrases) == 0 || !strings.Contains(d.Message, "shared phrases: "+d.SharedPhrases[0]) {
				t.Fatalf("%+v", d)
			}
		})
	}
}

func TestLexicalThresholdsAndDisabling(t *testing.T) {
	one := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	root := paragraphFiles(t, one, strings.Replace(one, "every", "each", 1))
	for _, config := range []map[string]any{{}, {"similarity": 1}, {"similarity": 0.7, "min-words": 100}} {
		diagnostics, err := Run(t.Context(), []string{root}, Options{Config: map[string]map[string]any{"similar-content": config}})
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("%+v: %+v (%v)", config, diagnostics, err)
		}
	}
	diagnostics, err := Run(t.Context(), []string{root}, Options{Config: map[string]map[string]any{"similar-content": {"similarity": 0.7}}, Disable: []string{"similar-content"}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("disabled: %+v (%v)", diagnostics, err)
	}
}

func TestSemanticLocalModel(t *testing.T) {
	root := paragraphFiles(t, "Repair broken program.", "Fix faulty code.", "Simmer vegetable soup.", "Repair broken program.")
	options := Options{
		Config:   map[string]map[string]any{"duplicate-content": {"min-words": 1}, "semantic-content": {"min-words": 1, "threshold": 0.85}},
		Semantic: SemanticOptions{Enabled: true, ModelPath: "testdata/model"},
	}
	diagnostics, err := Run(t.Context(), []string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("%+v", diagnostics)
	}
	d := diagnostics[0]
	if d.Rule != "semantic-content" || d.Method != "cosine" || d.Model != "testdata/model" || d.Similarity == nil || math.Abs(*d.Similarity-1) > 1e-9 || len(d.Related) != 1 || d.Related[0].Path != filepath.Join(root, "a.md") || d.Path != filepath.Join(root, "b.md") || d.Line != 2 {
		t.Fatalf("%+v", d)
	}
	if diagnostics[1].Rule != "duplicate-content" {
		t.Fatalf("%+v", diagnostics)
	}
}

func TestSemanticModelLoading(t *testing.T) {
	root := paragraphFiles(t, "repair broken program", "fix faulty code")
	if err := (Options{Semantic: SemanticOptions{Enabled: true}}).Validate(); err == nil {
		t.Fatal("accepted missing model configuration")
	}
	config := map[string]map[string]any{"semantic-content": {"min-words": 1, "threshold": 0.8}}
	if diagnostics, err := Run(t.Context(), []string{root}, Options{Config: config, Semantic: SemanticOptions{ModelPath: t.TempDir()}}); err != nil || len(diagnostics) != 0 {
		t.Fatalf("model loaded while semantic checks are off: %+v (%v)", diagnostics, err)
	}
	if _, err := Run(t.Context(), []string{root}, Options{Config: config, Semantic: SemanticOptions{Enabled: true, ModelPath: t.TempDir()}}); err == nil || !strings.Contains(err.Error(), "load semantic model") {
		t.Fatalf("missing model: %v", err)
	}
}
