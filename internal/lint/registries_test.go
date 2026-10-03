package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, frontmatter string) string {
	t.Helper()
	path := filepath.Join(root, "skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: A skill used by registry tests.\n"+frontmatter+"---\n\n# "+name+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRegistryReferencesResolveAcrossDocuments(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "owner", "metadata:\n  slots: \"code.style git.commits\"\n")
	writeSkill(t, root, "styled", "metadata:\n  provides: \"code.style.python git.commits\"\n")
	typo := writeSkill(t, root, "typo", "metadata:\n  provides: \"code.styles\"\n")
	opts := Options{Registries: []Registry{{
		ID:        "unknown-slot",
		Declare:   `frontmatter.metadata?.slots ?? ""`,
		Reference: `frontmatter.metadata?.provides ?? ""`,
		Match:     "dotted",
		Severity:  "error",
	}}}
	diagnostics, err := Run(t.Context(), []string{root}, opts)
	if err != nil {
		t.Fatal(err)
	}
	var found []Diagnostic
	for _, d := range diagnostics {
		if d.Rule == "unknown-slot" {
			found = append(found, d)
		}
	}
	if len(found) != 1 || found[0].Path != typo || found[0].Severity != "error" || !strings.Contains(found[0].Message, `"code.styles"`) || found[0].Line != 5 {
		t.Fatalf("unexpected registry diagnostics: %+v", found)
	}
}

func TestRegistryExactMatchAndLists(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "owner", "metadata:\n  slots: \"code.style\"\n")
	sub := writeSkill(t, root, "consumer", "uses:\n  - code.style\n  - code.style.python\n")
	opts := Options{Registries: []Registry{{
		ID:        "unknown-slot",
		Declare:   `frontmatter.metadata?.slots ?? ""`,
		Reference: `frontmatter.uses ?? []`,
	}}}
	diagnostics, err := Run(t.Context(), []string{root}, opts)
	if err != nil {
		t.Fatal(err)
	}
	var found []Diagnostic
	for _, d := range diagnostics {
		if d.Rule == "unknown-slot" {
			found = append(found, d)
		}
	}
	if len(found) != 1 || found[0].Path != sub || found[0].Severity != "warning" || !strings.Contains(found[0].Message, `"code.style.python"`) {
		t.Fatalf("exact matching should reject the sub-slot only: %+v", found)
	}
}

func TestInvalidRegistries(t *testing.T) {
	base := Registry{ID: "unknown-slot", Declare: `""`, Reference: `""`}
	for _, registry := range []Registry{
		{ID: "Bad", Declare: `""`, Reference: `""`},
		{ID: "broken-link", Declare: `""`, Reference: `""`},
		{ID: "unknown-slot", Reference: `""`},
		{ID: "unknown-slot", Declare: `""`, Reference: `(`},
		{ID: "unknown-slot", Declare: `""`, Reference: `""`, Match: "prefix"},
		{ID: "unknown-slot", Declare: `""`, Reference: `""`, Severity: "fatal"},
	} {
		if err := (Options{Registries: []Registry{registry}}).Validate(); err == nil {
			t.Fatalf("accepted %+v", registry)
		}
	}
	if err := (Options{Registries: []Registry{base, base}}).Validate(); err == nil {
		t.Fatal("accepted duplicate registry")
	}
	if err := (Options{Registries: []Registry{base}, Rules: []Rule{{ID: "unknown-slot", Select: "document", Assert: "true", Message: "m"}}}).Validate(); err == nil {
		t.Fatal("accepted registry id shared with a custom rule")
	}
	if err := (Options{Registries: []Registry{base}, Disable: []string{"unknown-slot"}}).Validate(); err != nil {
		t.Fatalf("registry id should be disableable: %v", err)
	}
}

func TestInvalidSlotEntries(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "owner", "metadata:\n  slots: \"code.style:keyed(list) review.criteria:unique lint.rules:map\"\n")
	writeSkill(t, root, "other", "metadata:\n  slots: \"review.criteria:list\"\n  provides: \"code.style.go@urgent git.commits@force {{.provided}}\"\n")
	diagnostics, err := Run(t.Context(), []string{root}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, d := range diagnostics {
		if d.Rule == "invalid-slot" {
			messages = append(messages, filepath.Base(filepath.Dir(d.Path))+": "+d.Message)
		}
	}
	want := []string{
		"other: slot review.criteria is declared with conflicting types list, unique",
		`other: slot code.style.go: unknown priority "urgent" (use force, normal or fallback)`,
		"owner: slot review.criteria is declared with conflicting types list, unique",
		`owner: slot lint.rules: unknown type "map" (use list, unique, keyed(list) or keyed(unique))`,
	}
	if strings.Join(messages, "\n") != strings.Join(want, "\n") {
		t.Fatalf("invalid-slot diagnostics:\n%s\nwant:\n%s", strings.Join(messages, "\n"), strings.Join(want, "\n"))
	}
}
