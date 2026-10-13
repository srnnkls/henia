package components

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/templating"
)

func TemplateBody(name, body string, data map[string]any) (string, error) {
	if !strings.Contains(body, "{{") {
		return body, nil
	}
	tmpl, err := template.New(name).Funcs(templating.Funcs).Option("missingkey=error").Parse(literalCode(body))
	if err != nil {
		return "", err
	}
	templating.BlankAbsent(tmpl)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func literalCode(body string) string {
	root, _ := markup.Tree([]byte(body))
	var b strings.Builder
	at := 0
	root.Walk(func(e *markup.Element) bool {
		if e.Type != "code" || e.Start < at {
			return true
		}
		b.WriteString(body[at:e.Start])
		b.WriteString(strings.ReplaceAll(body[e.Start:e.End], "{{", `{{"{{"}}`))
		at = e.End
		return false
	})
	b.WriteString(body[at:])
	return b.String()
}
