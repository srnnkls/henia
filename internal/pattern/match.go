package pattern

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

const maxBindings = 100000

var ErrTooMany = errors.New("the query produces too many combinations; anchor it to a skill or section")

type Resolver interface {
	Skill(ref string) *markup.Element
}

type Cell struct {
	Name     string
	Elements []*markup.Element
}

type Row []Cell

type binding map[string][]*markup.Element

type matcher struct {
	resolve  Resolver
	memo     map[memoKey][]binding
	reached  map[relationKey][]*markup.Element
	linkers  map[*markup.Element][]*markup.Element
	root     *markup.Element
	produced int
}

type relationKey struct {
	kind  string
	skill *markup.Element
}

type memoKey struct {
	p *Pattern
	e *markup.Element
}

func (q *Query) Run(root *markup.Element, resolve Resolver) ([]Row, error) {
	m := &matcher{resolve: resolve, memo: map[memoKey][]binding{}, reached: map[relationKey][]*markup.Element{}, root: root}
	order := 0
	root.Walk(func(e *markup.Element) bool {
		e.Order = order
		order++
		return true
	})
	seen := map[string]bool{}
	var rows []Row
	for _, p := range q.Patterns {
		var failure error
		root.Walk(func(e *markup.Element) bool {
			bindings, err := m.at(p, e)
			if err != nil {
				failure = err
				return false
			}
			for _, b := range bindings {
				if q.Captures[0] == "" {
					b = binding{"": {e}}
				}
				row, key := q.row(b)
				if !seen[key] {
					seen[key] = true
					rows = append(rows, row)
				}
			}
			return true
		})
		if failure != nil {
			return nil, failure
		}
	}
	return rows, nil
}

func (q *Query) row(b binding) (Row, string) {
	row := make(Row, 0, len(q.Captures))
	var key strings.Builder
	for _, name := range q.Captures {
		row = append(row, Cell{Name: name, Elements: b[name]})
		for _, e := range b[name] {
			key.WriteString(strconv.Itoa(e.Order) + ",")
		}
		key.WriteByte(';')
	}
	return row, key.String()
}

func (m *matcher) matches(p *Pattern, e *markup.Element) bool {
	bindings, _ := m.at(p, e)
	return len(bindings) > 0
}

func (m *matcher) at(p *Pattern, e *markup.Element) ([]binding, error) {
	key := memoKey{p, e}
	if cached, ok := m.memo[key]; ok {
		return cached, nil
	}
	result, err := m.match(p, e)
	if err != nil {
		return nil, err
	}
	m.memo[key] = result
	return result, nil
}

func (m *matcher) match(p *Pattern, e *markup.Element) ([]binding, error) {
	if len(p.Alts) > 0 {
		var out []binding
		for _, alt := range p.Alts {
			bindings, err := m.at(alt, e)
			if err != nil {
				return nil, err
			}
			for _, b := range bindings {
				out = append(out, capture(b, p.Capture, e))
			}
		}
		return out, nil
	}
	if p.Type != "" && e.Type != p.Type || e.Type == "corpus" {
		return nil, nil
	}
	for _, a := range p.Attrs {
		if !a.test(e) {
			return nil, nil
		}
	}
	for _, not := range p.Nots {
		candidates, target := descendants(e, false), not.Pattern
		if not.Relation != nil {
			candidates, target = m.related(not.Relation.Kind, e), not.Relation.Target
		}
		for _, d := range candidates {
			if m.matches(target, d) {
				return nil, nil
			}
		}
	}
	type partial struct {
		b    binding
		last int
	}
	results := []partial{{binding{}, -1}}
	for _, chain := range p.Chains {
		matched, err := m.chain(chain, e)
		if err != nil {
			return nil, err
		}
		var next []partial
		for _, r := range results {
			for _, c := range matched {
				if c.first >= 0 && c.first <= r.last {
					continue
				}
				if m.produced++; m.produced > maxBindings {
					return nil, ErrTooMany
				}
				next = append(next, partial{merge(r.b, c.b), max(r.last, c.last)})
			}
		}
		results = next
	}
	for _, relation := range p.Relations {
		if e.Type != "skill" {
			return nil, nil
		}
		var targets []binding
		for _, s := range m.related(relation.Kind, e) {
			bindings, err := m.at(relation.Target, s)
			if err != nil {
				return nil, err
			}
			targets = append(targets, bindings...)
		}
		var next []partial
		for _, r := range results {
			for _, t := range targets {
				if m.produced++; m.produced > maxBindings {
					return nil, ErrTooMany
				}
				next = append(next, partial{merge(r.b, t), r.last})
			}
		}
		results = next
	}
	out := make([]binding, 0, len(results))
	for _, r := range results {
		out = append(out, capture(r.b, p.Capture, e))
	}
	return out, nil
}

type chainMatch struct {
	b           binding
	first, last int
}

func (m *matcher) chain(c Chain, e *markup.Element) ([]chainMatch, error) {
	pivot := -1
	for i, l := range c.Links {
		if l.Pattern.Quant == 0 || l.Pattern.Quant == '+' {
			pivot = i
			break
		}
	}
	optional := pivot < 0
	if optional {
		pivot = 0
	}
	candidates := descendants(e, c.Links[0].Direct && pivot == 0)
	var out []chainMatch
	for _, d := range candidates {
		pp := c.Links[pivot].Pattern
		if !m.matches(pp, d) {
			continue
		}
		siblings := d.Parent.Children
		if pp.Quant == '+' && d.Index > 0 && m.matches(pp, siblings[d.Index-1]) {
			continue
		}
		if c.Links[0].Direct && d.Parent != e {
			continue
		}
		consumed := make([][]*markup.Element, len(c.Links))
		consumed[pivot] = []*markup.Element{d}
		right := d.Index + 1
		if pp.Quant == '+' {
			for right < len(siblings) && m.matches(pp, siblings[right]) {
				consumed[pivot] = append(consumed[pivot], siblings[right])
				right++
			}
		}
		ok := true
		for j := pivot + 1; j < len(c.Links) && ok; j++ {
			consumed[j], right, ok = m.take(c.Links[j].Pattern, siblings, right, 1)
		}
		left := d.Index - 1
		for j := pivot - 1; j >= 0 && ok; j-- {
			consumed[j], left, ok = m.take(c.Links[j].Pattern, siblings, left, -1)
		}
		if !ok || c.FirstChild && left != -1 || c.LastChild && right != len(siblings) {
			continue
		}
		matches := []chainMatch{{binding{}, -1, -1}}
		for j, nodes := range consumed {
			link := c.Links[j].Pattern
			for _, n := range nodes {
				bindings, err := m.at(link, n)
				if err != nil {
					return nil, err
				}
				if link.Quant == '*' || link.Quant == '+' {
					bindings = bindings[:1]
				}
				var next []chainMatch
				for _, cm := range matches {
					for _, b := range bindings {
						if m.produced++; m.produced > maxBindings {
							return nil, ErrTooMany
						}
						first := cm.first
						if first < 0 || n.Order < first {
							first = n.Order
						}
						next = append(next, chainMatch{merge(cm.b, b), first, max(cm.last, n.Order)})
					}
				}
				matches = next
			}
		}
		out = append(out, matches...)
	}
	if optional && len(out) == 0 {
		out = []chainMatch{{binding{}, -1, -1}}
	}
	return out, nil
}

func (m *matcher) take(p *Pattern, siblings []*markup.Element, i, step int) ([]*markup.Element, int, bool) {
	var nodes []*markup.Element
	for i >= 0 && i < len(siblings) && m.matches(p, siblings[i]) {
		nodes = append(nodes, siblings[i])
		i += step
		if p.Quant == 0 || p.Quant == '?' {
			break
		}
	}
	if len(nodes) == 0 && (p.Quant == 0 || p.Quant == '+') {
		return nil, i, false
	}
	if step < 0 {
		for a, b := 0, len(nodes)-1; a < b; a, b = a+1, b-1 {
			nodes[a], nodes[b] = nodes[b], nodes[a]
		}
	}
	return nodes, i, true
}

func (m *matcher) related(kind string, skill *markup.Element) []*markup.Element {
	if skill.Type != "skill" || m.resolve == nil {
		return nil
	}
	key := relationKey{kind, skill}
	if cached, ok := m.reached[key]; ok {
		return cached
	}
	step := m.targets
	if kind == "inbound" {
		step = m.sources
	}
	seen := map[*markup.Element]bool{}
	var out []*markup.Element
	queue := []*markup.Element{skill}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, t := range step(current) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
				queue = append(queue, t)
			}
		}
	}
	slices.SortFunc(out, func(a, b *markup.Element) int { return a.Order - b.Order })
	m.reached[key] = out
	return out
}

func (m *matcher) targets(skill *markup.Element) []*markup.Element {
	var out []*markup.Element
	skill.Walk(func(n *markup.Element) bool {
		if target := n.Attrs["target"]; n.Type == "link" && target != "" {
			if t := m.resolve.Skill(target); t != nil {
				out = append(out, t)
			}
		}
		return true
	})
	return out
}

func (m *matcher) sources(skill *markup.Element) []*markup.Element {
	if m.linkers == nil {
		m.linkers = map[*markup.Element][]*markup.Element{}
		for _, s := range m.root.Children {
			if s.Type != "skill" {
				continue
			}
			for _, t := range m.targets(s) {
				if !slices.Contains(m.linkers[t], s) {
					m.linkers[t] = append(m.linkers[t], s)
				}
			}
		}
	}
	return m.linkers[skill]
}

func descendants(e *markup.Element, direct bool) []*markup.Element {
	if direct {
		return e.Children
	}
	var out []*markup.Element
	e.Walk(func(d *markup.Element) bool {
		if d != e {
			out = append(out, d)
		}
		return true
	})
	return out
}

func capture(b binding, name string, e *markup.Element) binding {
	if name == "" {
		return b
	}
	out := merge(b, nil)
	out[name] = append(out[name], e)
	return out
}

func merge(a, b binding) binding {
	out := make(binding, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = append(out[k][:len(out[k]):len(out[k])], v...)
	}
	return out
}

func (a Attr) test(e *markup.Element) bool {
	switch a.Key {
	case "contains":
		return strings.Contains(strings.ToLower(e.Text), strings.ToLower(a.Value))
	case "matches":
		return a.Re.MatchString(e.Text)
	}
	value, ok := e.Attrs[a.Key]
	if a.Key == "text" {
		value, ok = strings.TrimSpace(e.Text), true
	}
	if !ok {
		return false
	}
	if a.Re != nil {
		return a.Re.MatchString(value)
	}
	if a.Range {
		n, err := strconv.Atoi(value)
		return err == nil && a.Lo <= n && n <= a.Hi
	}
	return value == a.Value
}
