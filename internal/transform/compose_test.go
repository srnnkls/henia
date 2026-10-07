package transform

import (
	"strings"
	"testing"
)

func TestComposeIncludesSharedTemplate(t *testing.T) {
	shared := map[string]string{"head": "Context: {{.mode}}\n"}
	got, err := Compose("{{template \"head\" .}}# Body\n", map[string]string{"mode": "preload"}, shared, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Context: preload\n# Body\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComposeLayoutOrdersBlocks(t *testing.T) {
	shared := map[string]string{
		"skill": "# {{.name}}\n\n{{block \"lead\" .}}{{required \"lead\"}}{{end}}\n\n## Usage\n\n{{block \"usage\" .}}None.{{end}}\n",
	}
	body := "{{define \"usage\"}}Run it.{{end}}\n\n{{define \"lead\"}}Ships releases.{{end}}\n"
	got, err := Compose(body, map[string]string{"name": "deploy"}, shared, "skill")
	if err != nil {
		t.Fatal(err)
	}
	if want := "# deploy\n\nShips releases.\n\n## Usage\n\nRun it.\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComposeLayoutErrors(t *testing.T) {
	shared := map[string]string{"skill": "{{block \"lead\" .}}{{required \"lead\"}}{{end}}"}
	for _, tc := range []struct{ name, body, layout, want string }{
		{"stray text", "Loose prose.\n{{define \"lead\"}}x{{end}}", "skill", "text outside define blocks"},
		{"missing block", "{{define \"other\"}}x{{end}}", "skill", `block "lead" is required`},
		{"unknown layout", "{{define \"lead\"}}x{{end}}", "paper", `unknown layout "paper"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compose(tc.body, map[string]string{}, shared, tc.layout)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
