package lint_test

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/lint"
)

func TestDuplicateDirectiveContentAndRelatedLocations(t *testing.T) {
	root := t.TempDir()
	paragraph := "Inspect every changed function and report concrete failures with enough context to reproduce the problem."
	first := write(t, root, "a.md", ":::instruction\n"+paragraph+"\n:::\n")
	second := write(t, root, "b.md", ":::outer\n:::instruction\n"+strings.ToUpper(paragraph)+"\n:::\n:::\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Rule != "duplicate-content" || diagnostics[0].Path != second || diagnostics[0].Line != 3 {
		t.Fatalf("%+v", diagnostics)
	}
	if len(diagnostics[0].Related) != 1 || diagnostics[0].Related[0].Path != first || diagnostics[0].Related[0].Line != 2 || diagnostics[0].Related[0].Column != 1 {
		t.Fatalf("missing original location: %+v", diagnostics)
	}
}

func TestDuplicateContentRetainsLinkDestinations(t *testing.T) {
	root := t.TempDir()
	text := "Read this detailed guide before proceeding with any changes to the project configuration or dependencies."
	write(t, root, "a.md", "["+text+"](https://example.test/first)\n")
	write(t, root, "b.md", "["+text+"](https://example.test/second)\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("distinct links treated as copies: %+v (%v)", diagnostics, err)
	}
}

func TestDuplicateContentThresholdAndLiteralContexts(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.md", "Run the checks first.\n\nRun  the checks\nfirst.\n\n```md\nRun the checks first.\n```\n\nRun {{.checks}} first.\n\nRun {{.checks}} first.\n")
	diagnostics, err := lint.Run(t.Context(), []string{root}, lint.Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("default threshold: %+v (%v)", diagnostics, err)
	}
	diagnostics, err = lint.Run(t.Context(), []string{root}, lint.Options{DuplicateMinWords: 4})
	if err != nil || len(diagnostics) != 1 || diagnostics[0].Line != 3 {
		t.Fatalf("configured threshold: %+v (%v)", diagnostics, err)
	}
	diagnostics, err = lint.Run(t.Context(), []string{root}, lint.Options{DuplicateMinWords: 4, Disable: []string{"duplicate-content"}})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("disabled: %+v (%v)", diagnostics, err)
	}
	if err := (lint.Options{DuplicateMinWords: -1}).Validate(); err == nil {
		t.Fatal("accepted negative threshold")
	}
}
