package templating

import "testing"

func TestExecuteTemplatePrintsAbsentValueBlank(t *testing.T) {
	vars := map[string]any{"name": "pr", "context_commands": nil}
	got, err := ExecuteTemplate("[{{.context_commands}}]{{range .context_commands}}x{{end}} {{.name}}", vars)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	if want := "[] pr"; got != want {
		t.Errorf("ExecuteTemplate() = %q, want %q", got, want)
	}
}
