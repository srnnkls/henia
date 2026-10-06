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

type edit struct {
	start, end int
	text, rule string
}

func Render(body string, rendering Rendering) (string, []Rendered, []string, error) {
	r, err := recognition()
	if err != nil {
		return "", nil, nil, err
	}
	tree, err := markup.Tree([]byte(body))
	if err != nil {
		return "", nil, nil, err
	}
	refs, err := Recognize(tree)
	if err != nil {
		return "", nil, nil, err
	}
	tables := maps.Clone(r.tables)
	tools := pattern.Table{Keys: slices.Sorted(maps.Keys(rendering.Tools))}
	for _, name := range tools.Keys {
		tools.Values = append(tools.Values, rendering.Tools[name])
	}
	tables["tools"] = tools
	tables["served"] = pattern.Table{List: true, Values: slices.Sorted(maps.Keys(rendering.Served))}
	var edits []edit
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
			edits = append(edits, edit{target.Start, target.End, text, rewrite.ID})
		}
		return nil
	}
	for _, rewrite := range r.rewrites {
		if err := apply(rewrite, func(row pattern.Row) (string, error) { return rewrite.Output.Render(row, nil, nil), nil }); err != nil {
			return "", nil, nil, err
		}
	}
	for _, kind := range slices.Sorted(maps.Keys(rendering.References)) {
		rule, err := harnessRewrite(kind, rendering.References[kind])
		if err != nil {
			return "", nil, nil, err
		}
		if err := apply(rule.rewrite, func(row pattern.Row) (string, error) { return rule.render(row, kind) }); err != nil {
			return "", nil, nil, err
		}
	}
	slices.SortStableFunc(edits, func(a, b edit) int { return a.start - b.start })
	var out strings.Builder
	var kept []edit
	var dropped []string
	position, shifts := 0, map[int]int{}
	for _, e := range edits {
		if len(kept) > 0 && kept[len(kept)-1].start == e.start && kept[len(kept)-1].end == e.end {
			continue
		}
		if e.start < position {
			line := strings.Count(body[:e.start], "\n") + 1
			dropped = append(dropped, fmt.Sprintf("line %d: rewrite %s overlaps an earlier rewrite and was dropped", line, e.rule))
			continue
		}
		out.WriteString(body[position:e.start])
		shifts[e.start] = out.Len()
		out.WriteString(e.text)
		position = e.end
		kept = append(kept, e)
	}
	out.WriteString(body[position:])
	rendered := make([]Rendered, 0, len(refs))
	for _, ref := range refs {
		start, end := ref.Start, ref.End
		delta, rule, inside := 0, "", false
		for _, e := range kept {
			switch {
			case e.start == start && e.end == end:
				rule = e.rule
				ref.Start, ref.End = shifts[e.start], shifts[e.start]+len(e.text)
			case e.end <= start:
				delta += len(e.text) - (e.end - e.start)
			case e.start < end:
				inside = true
			}
		}
		if inside {
			continue
		}
		if rule == "" {
			ref.Start, ref.End = start+delta, end+delta
		}
		rendered = append(rendered, Rendered{ref, rule})
	}
	return out.String(), rendered, dropped, nil
}
