package components

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

func (x *expander) include(it *markup.Item, decl Declaration) []*markup.Item {
	if _, ok := x.validate(it, decl, it.Attrs); !ok {
		return x.keep(it)
	}
	ref, referenced := it.Attrs["ref"]
	name, named := it.Attrs["template"]
	switch {
	case referenced == named:
		x.report(it, InvalidInclude, "include takes exactly one of ref and template")
		return x.keep(it)
	case referenced:
		return x.includeRef(it, ref)
	}
	target, ok := x.decls[name]
	switch {
	case !ok:
		x.report(it, MissingInclude, "include names unknown component %s", name)
		return x.keep(it)
	case target.Path == "" || target.Body.Kind != "none":
		x.report(it, InvalidInclude, "include template %s must be a declared component whose body kind is none", name)
		return x.keep(it)
	}
	args := maps.Clone(it.Attrs)
	delete(args, "template")
	before := len(x.diags)
	values := x.arguments(it, target, args)
	if len(x.diags) > before {
		return x.keep(it)
	}
	out, ok := x.render(it, target, values, nil)
	if !ok {
		return x.keep(it)
	}
	return out
}

func (x *expander) includeRef(call *markup.Item, ref string) []*markup.Item {
	if x.graph == nil {
		return x.keep(call)
	}
	selected, ok := x.graph.resolve(x, call, ref)
	if !ok {
		return x.keep(call)
	}
	grafted := transplant(call, selected)
	for _, it := range grafted {
		x.sealed[it] = true
	}
	x.changed = true
	return call.Replace(grafted...)
}

func (g *graph) resolve(x *expander, call *markup.Item, ref string) ([]*markup.Item, bool) {
	at, anchor, err := g.locate(x.unit.unit, ref)
	if err != nil {
		x.report(call, MissingInclude, "include %s: %v", ref, err)
		return nil, false
	}
	target, ok := g.nodes[at]
	if !ok {
		x.report(call, MissingInclude, "include names unknown skill %s:%s", at.Package, at.Name)
		return nil, false
	}
	if target.state == expanding {
		g.cycle(x, call, target)
		return nil, false
	}
	g.trail = append(g.trail, step{from: x.unit, call: call})
	g.expand(target)
	g.trail = g.trail[:len(g.trail)-1]
	selected, err := target.section(anchor)
	switch {
	case err != nil:
		x.report(call, InvalidMarkup, "include %s: %v", ref, err)
		return nil, false
	case selected == nil && anchor != "":
		x.report(call, MissingInclude, "skill %s:%s has no section %s", at.Package, at.Name, anchor)
		return nil, false
	}
	return selected, true
}

func (g *graph) locate(from Unit, ref string) (Key, string, error) {
	if g.env.Resolve != nil {
		return g.env.Resolve(from, ref)
	}
	name, anchor, _ := strings.Cut(ref, "#")
	if pkg, skill, qualified := strings.Cut(name, ":"); qualified {
		return Key{pkg, skill}, anchor, nil
	}
	if local := (Key{from.Package, name}); g.nodes[local] != nil {
		return local, anchor, nil
	}
	var found []Key
	for k := range g.nodes {
		if k.Name == name {
			found = append(found, k)
		}
	}
	switch len(found) {
	case 0:
		return Key{}, "", fmt.Errorf("unknown skill %s", name)
	case 1:
		return found[0], anchor, nil
	}
	slices.SortFunc(found, func(a, b Key) int { return strings.Compare(a.Package, b.Package) })
	packages := make([]string, len(found))
	for i, k := range found {
		packages[i] = k.Package
	}
	return Key{}, "", fmt.Errorf("skill %s is ambiguous across packages %s", name, strings.Join(packages, ", "))
}

func (g *graph) cycle(x *expander, call *markup.Item, target *node) {
	x.report(call, IncludeCycle, "include of %s closes an include cycle", target.unit.Name)
	from := slices.IndexFunc(g.trail, func(s step) bool { return s.from == target })
	if from < 0 {
		return
	}
	d := &x.diags[len(x.diags)-1]
	for _, s := range g.trail[from:] {
		site := callSite(s.call)
		d.Notes = append(d.Notes, fmt.Sprintf("included from %s:%d:%d", site.File, site.Line, site.Column))
	}
}

func transplant(call *markup.Item, items []*markup.Item) []*markup.Item {
	out := make([]*markup.Item, 0, len(items))
	for _, it := range items {
		inclusion := markup.Call{Directive: call.Origin, Component: callSite(it)}
		out = append(out, replica(it, append(slices.Clone(call.Backtrace), inclusion)))
	}
	return out
}

func replica(it *markup.Item, trace []markup.Call) *markup.Item {
	c := *it
	c.Attrs = maps.Clone(it.Attrs)
	c.Backtrace = append(slices.Clone(trace), it.Backtrace...)
	c.Children = make([]*markup.Item, 0, len(it.Children))
	for _, child := range it.Children {
		c.Children = append(c.Children, replica(child, trace))
	}
	return it.Replace(&c)[0]
}
