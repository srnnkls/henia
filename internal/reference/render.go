package reference

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"text/template"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
)

type Rendering struct {
	Tools      map[string]string
	Served     map[string]bool
	References map[string]string
}

type Rendered struct {
	Reference
	Rewrite string
	Address bool `json:",omitempty"`
}

type harnessRule struct {
	rewrite *pattern.Rewrite
	format  *template.Template
}

var harnessRules sync.Map

func harnessRewrite(kind, output string) (harnessRule, error) {
	key := kind + "\x00" + output
	if cached, ok := harnessRules.Load(key); ok {
		return cached.(harnessRule), nil
	}
	r, err := recognition()
	if err != nil {
		return harnessRule{}, err
	}
	module, err := pattern.ReadModule(fmt.Sprintf(`(import std/references) (rewrite %s-reference :output "" (reference @r %q ?name))`, kind, kind), func(string) (map[string]pattern.Define, error) { return r.defines, nil })
	if err != nil {
		return harnessRule{}, err
	}
	rule := harnessRule{rewrite: module.Rewrites[0]}
	if strings.Contains(output, "{{") {
		if rule.format, err = template.New(kind).Option("missingkey=error").Parse(output); err != nil {
			return harnessRule{}, fmt.Errorf("references.%s: %w", kind, err)
		}
	} else {
		output = strings.ReplaceAll(output, "{?kind}", kind)
		rule.rewrite.Output = pattern.ParseTemplate(output)
		for _, name := range rule.rewrite.Output.Placeholders() {
			if name != "?name" && name != "@r" && name != "@r.text" {
				return harnessRule{}, fmt.Errorf("references.%s: {%s} is not a reference field; use {?name}, {?kind} or {@r.text}", kind, name)
			}
		}
	}
	harnessRules.Store(key, rule)
	return rule, nil
}

func wrap(output string) string {
	if strings.ContainsAny(output, "*[]") {
		return output
	}
	return "`" + output + "`"
}

func (h harnessRule) render(row pattern.Row, kind string) (string, error) {
	if h.format == nil {
		return wrap(h.rewrite.Output.Render(row, nil, nil)), nil
	}
	code := pattern.Captured(row, "r")
	var b bytes.Buffer
	err := h.format.Execute(&b, map[string]string{"Name": row.Vars["name"], "Type": kind, "Raw": code.Text})
	return wrap(b.String()), err
}

func Rewrites(tree *markup.Element, rendering Rendering) ([]markup.Edit, []Rendered, error) {
	r, err := recognition()
	if err != nil {
		return nil, nil, err
	}
	refs, err := Recognize(tree)
	if err != nil {
		return nil, nil, err
	}
	addresses, err := Addresses(tree)
	if err != nil {
		return nil, nil, err
	}
	sources := make([]Rendered, 0, len(refs)+len(addresses))
	for _, ref := range refs {
		sources = append(sources, Rendered{Reference: ref})
	}
	for _, address := range addresses {
		sources = append(sources, Rendered{Reference: address, Address: true})
	}
	if len(refs) == 0 {
		return nil, sources, nil
	}
	tables := maps.Clone(r.tables)
	tools := pattern.Table{Keys: slices.Sorted(maps.Keys(rendering.Tools))}
	for _, name := range tools.Keys {
		tools.Values = append(tools.Values, rendering.Tools[name])
	}
	tables["tools"] = tools
	tables["served"] = pattern.Table{List: true, Values: slices.Sorted(maps.Keys(rendering.Served))}
	var edits []markup.Edit
	apply := func(rewrite *pattern.Rewrite, output func(pattern.Row) (string, error)) error {
		rows, err := rewrite.Query.Run(tree, pattern.Environment{Data: pattern.DataRoot(tables)})
		if err != nil {
			return fmt.Errorf("rewrite %s: %w", rewrite.ID, err)
		}
		for _, row := range rows {
			target := pattern.Captured(row, rewrite.At)
			text, err := output(row)
			if err != nil {
				return fmt.Errorf("rewrite %s: %w", rewrite.ID, err)
			}
			edits = append(edits, markup.Edit{Start: target.Start, End: target.End, Text: text, Rule: rewrite.ID})
		}
		return nil
	}
	for _, rewrite := range r.rewrites {
		if err := apply(rewrite, func(row pattern.Row) (string, error) { return rewrite.Output.Render(row, nil, nil), nil }); err != nil {
			return nil, nil, err
		}
	}
	for _, kind := range slices.Sorted(maps.Keys(rendering.References)) {
		rule, err := harnessRewrite(kind, rendering.References[kind])
		if err != nil {
			return nil, nil, err
		}
		if err := apply(rule.rewrite, func(row pattern.Row) (string, error) { return rule.render(row, kind) }); err != nil {
			return nil, nil, err
		}
	}
	return edits, sources, nil
}

func Locate(applied markup.Applied, sources []Rendered) []Rendered {
	var rendered []Rendered
	for _, source := range sources {
		start, end, rule, ok := applied.Span(source.Start, source.End)
		if !ok {
			continue
		}
		source.Start, source.End, source.Rewrite = start, end, rule
		rendered = append(rendered, source)
	}
	slices.SortFunc(rendered, func(a, b Rendered) int { return a.Start - b.Start })
	return rendered
}

func Render(body string, rendering Rendering) (string, []string, error) {
	tree, err := markup.Tree([]byte(body))
	if err != nil {
		return "", nil, err
	}
	edits, _, err := Rewrites(tree, rendering)
	if err != nil {
		return "", nil, err
	}
	applied := markup.Apply(body, edits)
	return applied.Text, Warnings(body, applied.Dropped), nil
}

func Warnings(body string, dropped []markup.Edit) []string {
	var warnings []string
	for _, e := range dropped {
		warnings = append(warnings, fmt.Sprintf("line %d: %s overlaps an earlier rewrite and was dropped", strings.Count(body[:e.Start], "\n")+1, e.Rule))
	}
	return warnings
}
