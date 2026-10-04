# Henia

These skills are not installed in this harness. When a task fits one, read it with `henia show <skill>` and follow it as if it were installed:

{{range .}}- `{{.Name}}`: {{.Description}}
{{end}}
`henia ls` lists every skill in the library, including other sources'. `henia show <skill>#<section>` reads a single section.
