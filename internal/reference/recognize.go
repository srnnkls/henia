package reference

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/lint/std"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
)

type recognizer struct {
	query    *pattern.Query
	shown    *pattern.Query
	tables   map[string]pattern.Table
	rewrites []*pattern.Rewrite
	defines  map[string]pattern.Define
}

var recognition = sync.OnceValues(func() (*recognizer, error) {
	data, err := std.FS.ReadFile("references.md")
	if err != nil {
		return nil, err
	}
	art, err := artifact.Parse(data)
	if err != nil {
		return nil, err
	}
	tree, err := markup.Tree([]byte(art.Body))
	if err != nil {
		return nil, err
	}
	source, _ := pattern.Source(tree, 0)
	module, err := pattern.ReadModule(source+`(rule reference :message "{?kind} {?name}" (reference @r ?kind ?name))
(rule show :message "{?command} {?address} {?resource} {?anchor}" (show-address @r ?command ?address ?resource ?anchor))`, nil)
	if err != nil {
		return nil, fmt.Errorf("std/references: %w", err)
	}
	r := &recognizer{query: module.Rules[len(module.Rules)-2].Query, shown: module.Rules[len(module.Rules)-1].Query, tables: map[string]pattern.Table{}, rewrites: module.Rewrites, defines: module.Defines}
	raw, _ := art.Frontmatter["data"].(map[string]any)
	for name, rows := range raw {
		if r.tables[name], err = pattern.ParseTable(rows); err != nil {
			return nil, fmt.Errorf("std/references: data %s: %w", name, err)
		}
	}
	return r, nil
})

func Recognize(file *markup.Element) ([]Reference, error) {
	r, err := recognition()
	if err != nil {
		return nil, err
	}
	rows, err := r.query.Run(file, pattern.Environment{Data: pattern.DataRoot(r.tables)})
	if err != nil {
		return nil, err
	}
	refs := make([]Reference, 0, len(rows))
	for _, row := range rows {
		code := row.Cells[0].Elements[0]
		refs = append(refs, Reference{Type: Type(row.Vars["kind"]), Name: row.Vars["name"], Raw: code.Text, Start: code.Start, End: code.End})
	}
	slices.SortFunc(refs, func(a, b Reference) int { return a.Start - b.Start })
	return refs, nil
}

func Addresses(file *markup.Element) ([]Reference, error) {
	r, err := recognition()
	if err != nil {
		return nil, err
	}
	rows, err := r.shown.Run(file, pattern.Environment{})
	if err != nil {
		return nil, err
	}
	refs := make([]Reference, 0, len(rows))
	for _, row := range rows {
		code := pattern.Captured(row, "r")
		ticks, _ := strconv.Atoi(code.Attrs["ticks"])
		start := code.Start + ticks
		ref := Reference{Type: TypeSkill, Name: row.Vars["address"], Raw: row.Vars["command"], Resource: row.Vars["resource"], Anchor: row.Vars["anchor"], Start: start - 1, End: start + len(row.Vars["command"])}
		if ref.Resource == "" {
			prefix, name := "", ref.Name
			if pkg, rest, qualified := strings.Cut(name, ":"); qualified {
				prefix, name = pkg+":", rest
			}
			if skill, module, dotted := strings.Cut(name, "."); dotted {
				ref.Name, ref.Module = prefix+skill, module
			}
		}
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b Reference) int { return a.Start - b.Start })
	return refs, nil
}
