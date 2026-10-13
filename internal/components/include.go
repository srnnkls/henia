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
	name, named := it.Attrs["component"]
	switch {
	case referenced == named:
		x.report(it, InvalidInclude, "include takes exactly one of ref and component")
		return x.keep(it)
	case referenced:
		return x.includeRef(it, ref)
	}
	target, ok := x.decls[name]
	if ok {
		x.called(target)
	}
	switch {
	case !ok:
		x.report(it, MissingInclude, "include names unknown component %s", name)
		return x.keep(it)
	case target.Path == "" || target.Body.Kind != "none":
		x.report(it, InvalidInclude, "include component %s must be a declared component whose body kind is none", name)
		return x.keep(it)
	}
	args := maps.Clone(it.Attrs)
	delete(args, "component")
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
	target, ok := g.node(at)
	if !ok {
		x.report(call, MissingInclude, "include names unknown %s", at)
		return nil, false
	}
	if g.env.Included != nil {
		g.env.Included(x.unit.key(), at, callSite(call))
	}
	if target.state == expanding {
		g.cycle(x, call, target)
		return nil, false
	}
	g.trail = append(g.trail, step{from: x.unit, call: call})
	g.expand(target)
	g.trail = g.trail[:len(g.trail)-1]
	if target.unit.Document == nil {
		return nil, false
	}
	selected, err := target.section(anchor)
	switch {
	case err != nil:
		x.report(call, InvalidMarkup, "include %s: %v", ref, err)
		return nil, false
	case selected == nil && anchor != "":
		x.report(call, MissingInclude, "%s has no section %s", at, anchor)
		return nil, false
	}
	return selected, true
}

func (g *graph) locate(from Unit, ref string) (Key, string, error) {
	if g.env.Resolve != nil {
		return g.env.Resolve(from, ref)
	}
	pkg, name, resource, anchor := ParseRef(ref)
	if pkg != "" {
		return Key{pkg, name, resource}, anchor, nil
	}
	if g.nodes[Key{from.Package, name, ""}] != nil {
		return Key{from.Package, name, resource}, anchor, nil
	}
	var found []Key
	for k := range g.nodes {
		if k.Name == name && k.Resource == "" {
			found = append(found, k)
		}
	}
	switch len(found) {
	case 0:
		return Key{}, "", fmt.Errorf("unknown skill %s", name)
	case 1:
		return Key{found[0].Package, name, resource}, anchor, nil
	}
	slices.SortFunc(found, func(a, b Key) int { return strings.Compare(a.Package, b.Package) })
	packages := make([]string, len(found))
	for i, k := range found {
		packages[i] = k.Package
	}
	return Key{}, "", fmt.Errorf("skill %s is ambiguous across packages %s", name, strings.Join(packages, ", "))
}

func (g *graph) cycle(x *expander, call *markup.Item, target *node) {
	x.report(call, IncludeCycle, "include of %s closes an include cycle", target.key())
	from := slices.IndexFunc(g.trail, func(s step) bool { return s.from == target })
	if from < 0 {
		return
	}
	d := &x.diags[len(x.diags)-1]
	for _, s := range g.trail[from:] {
		site := callSite(s.call)
		if site.File == s.from.unit.Path {
			site.Line += s.from.unit.Offset
		}
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

func ParseRef(ref string) (pkg, name, resource, anchor string) {
	ref, anchor, _ = strings.Cut(ref, "#")
	if p, rest, qualified := strings.Cut(ref, ":"); qualified {
		pkg, ref = p, rest
	}
	if skill, path, ok := strings.Cut(ref, "/"); ok {
		return pkg, skill, path, anchor
	}
	if skill, module, ok := strings.Cut(ref, "."); ok {
		return pkg, skill, module, anchor
	}
	return pkg, ref, "", anchor
}
