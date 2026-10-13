package components

import (
	"slices"
	"strconv"

	"github.com/srnnkls/henia/internal/markup"
)

type Unit struct {
	Name, Package string
	Resource      string
	Path          string
	Offset        int
	Document      *markup.Document
	Components    map[string]Declaration
	Values        map[string]any
	Variables     map[string]string
	Finish        func(*markup.Document) []Diagnostic
}

type Key struct{ Package, Name, Resource string }

func (k Key) String() string {
	s := k.Package + ":" + k.Name
	return s + k.Resource
}

type state int

const (
	pending state = iota
	expanding
	expanded
)

type node struct {
	unit    Unit
	state   state
	anchors map[string][]*markup.Item
	indexed bool
}

type step struct {
	from *node
	call *markup.Item
}

type graph struct {
	env   Environment
	nodes map[Key]*node
	trail []step
	diags []Diagnostic
}

func ExpandDocuments(units []Unit, env Environment) []Diagnostic {
	g := &graph{env: env, nodes: map[Key]*node{}}
	order := make([]*node, 0, len(units))
	for _, u := range units {
		n := &node{unit: u}
		g.nodes[Key{u.Package, u.Name, u.Resource}] = n
		order = append(order, n)
	}
	for _, n := range order {
		g.expand(n)
	}
	sortDiagnostics(g.diags)
	return g.diags
}

func (g *graph) expand(n *node) {
	if n.state != pending {
		return
	}
	n.state = expanding
	defer func() { n.state = expanded }()
	if n.unit.Document == nil {
		return
	}
	env := g.env
	env.Values = n.unit.Values
	if n.unit.Variables != nil {
		env.Variables = n.unit.Variables
	}
	x := newExpander(n.unit.Components, env)
	x.graph, x.unit = g, n
	g.diags = append(g.diags, x.run(n.unit.Document)...)
	if n.unit.Finish != nil {
		g.diags = append(g.diags, n.unit.Finish(n.unit.Document)...)
	}
}

func (g *graph) node(at Key) (*node, bool) {
	if n, ok := g.nodes[at]; ok {
		return n, true
	}
	if g.env.Load == nil {
		return nil, false
	}
	u, ok := g.env.Load(at)
	if !ok {
		return nil, false
	}
	n := &node{unit: u}
	g.nodes[at] = n
	return n, true
}

func (n *node) key() Key { return Key{n.unit.Package, n.unit.Name, n.unit.Resource} }

func (n *node) section(anchor string) ([]*markup.Item, error) {
	doc := n.unit.Document
	if anchor == "" {
		return doc.Children, nil
	}
	if !n.indexed {
		anchors, err := sectionAnchors(doc)
		if err != nil {
			return nil, err
		}
		n.anchors, n.indexed = anchors, true
	}
	return n.anchors[anchor], nil
}

func sectionAnchors(doc *markup.Document) (map[string][]*markup.Item, error) {
	sections, err := markup.Sections(markup.Write(doc))
	if err != nil {
		return nil, err
	}
	anchors := make(map[string][]*markup.Item, len(sections))
	var walk func(items []*markup.Item)
	walk = func(items []*markup.Item) {
		items = dissolve(items)
		for i, it := range items {
			if it.Kind != "heading" {
				walk(it.Children)
				continue
			}
			at := slices.IndexFunc(sections, func(s markup.Section) bool { return s.Title == it.Text })
			if at < 0 {
				continue
			}
			anchors[sections[at].Anchor] = sectionOf(items[i:])
			sections = sections[at+1:]
		}
	}
	walk(doc.Children)
	return anchors, nil
}

func dissolve(items []*markup.Item) []*markup.Item {
	out := make([]*markup.Item, 0, len(items))
	for _, it := range items {
		if it.Kind == "section" {
			out = append(out, dissolve(it.Children)...)
			continue
		}
		out = append(out, it)
	}
	return out
}

func sectionOf(items []*markup.Item) []*markup.Item {
	level := headingLevel(items[0])
	for i, it := range items[1:] {
		if it.Kind == "heading" && headingLevel(it) <= level {
			return items[:i+1]
		}
	}
	return items
}

func headingLevel(it *markup.Item) int {
	level, _ := strconv.Atoi(it.Attrs["level"])
	return level
}
