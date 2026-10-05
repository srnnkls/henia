package pattern

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/similarity"
)

const (
	maxBindings  = 100000
	shingleWords = 3
)

var ErrTooMany = errors.New("the query produces too many combinations; anchor it to a skill or section")

var ErrNoModel = errors.New("similar needs a local Model2Vec model; pass --model or set [lint.semantic] model_path")

type Resolver interface {
	Skill(ref string) *markup.Element
}

type Cell struct {
	Name     string
	Elements []*markup.Element
}

type Row []Cell

type binding struct {
	captures map[string][]*markup.Element
	vars     map[string]string
	unequal  []constraint
}

type constraint struct {
	name, value, relate string
	threshold           float64
}

type Environment struct {
	Resolve Resolver
	Embed   func(string) []float32
	Params  map[string]string
	Data    *markup.Element
	Now     time.Time
}

type matcher struct {
	env      Environment
	resolve  Resolver
	embed    func(string) []float32
	shingles map[string]map[string]bool
	vectors  map[string][]float64
	failure  error
	memo     map[memoKey][]binding
	reached  map[relationKey][]*markup.Element
	linkers  map[*markup.Element][]*markup.Element
	incoming map[*markup.Element][]*markup.Element
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

func (q *Query) Run(root *markup.Element, env Environment) ([]Row, error) {
	if q.Semantic && env.Embed == nil {
		return nil, ErrNoModel
	}
	for _, name := range q.params {
		if _, ok := env.Params[name]; !ok {
			return nil, fmt.Errorf("$%s has no value; set it in the module frontmatter or [lint.config]", name)
		}
	}
	if env.Data != nil {
		root = &markup.Element{Type: "corpus", Attrs: map[string]string{}, Children: append(slices.Clip(root.Children), env.Data)}
	}
	if env.Now.IsZero() {
		env.Now = time.Now()
	}
	m := &matcher{env: env, resolve: env.Resolve, embed: env.Embed, memo: map[memoKey][]binding{}, reached: map[relationKey][]*markup.Element{}, root: root, shingles: map[string]map[string]bool{}, vectors: map[string][]float64{}}
	order := 0
	root.Walk(func(e *markup.Element) bool {
		e.Order = order
		order++
		return true
	})
	seen := map[string]bool{}
	var rows []Row
	var bindings []binding
	var err error
	if len(q.Members) == 1 && len(q.Absent) == 0 {
		bindings, err = m.everywhere(q.Members[0], q.Captures[0] == "")
	} else {
		bindings, err = m.join(q.Members, q.Absent)
	}
	if err == nil {
		err = m.failure
	}
	if err != nil {
		return nil, err
	}
	for _, b := range bindings {
		row, key := q.row(b)
		if !seen[key] {
			seen[key] = true
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (m *matcher) everywhere(p *Pattern, implicit bool) ([]binding, error) {
	var out []binding
	var failure error
	m.root.Walk(func(e *markup.Element) bool {
		bindings, err := m.at(p, e)
		if err != nil {
			failure = err
			return false
		}
		for _, b := range bindings {
			if implicit {
				b.captures = map[string][]*markup.Element{"": {e}}
			}
			out = append(out, b)
		}
		return true
	})
	return out, failure
}

func (m *matcher) join(members, absent []*Pattern) ([]binding, error) {
	rows := []binding{{}}
	for _, member := range members {
		bindings, err := m.everywhere(member, false)
		if err != nil {
			return nil, err
		}
		index := indexBy(bindings, shared(rows, bindings))
		candidates := index.candidates
		if near, ok := m.nearIndex(rows, bindings); ok && len(index.keys) == 0 {
			candidates = near
		}
		var next []binding
		for _, row := range rows {
			for _, b := range candidates(row) {
				joined, ok := m.unify(row, b)
				if !ok {
					continue
				}
				if m.produced++; m.produced > maxBindings {
					return nil, ErrTooMany
				}
				next = append(next, joined)
			}
		}
		rows = next
	}
	for _, absent := range absent {
		bindings, err := m.everywhere(absent, false)
		if err != nil {
			return nil, err
		}
		index := indexBy(bindings, shared(rows, bindings))
		rows = slices.DeleteFunc(rows, func(row binding) bool {
			for _, b := range index.candidates(row) {
				if _, ok := m.unify(row, b); ok {
					return true
				}
			}
			return false
		})
	}
	return rows, nil
}

func (m *matcher) nearIndex(rows, bindings []binding) (func(binding) []binding, bool) {
	if len(rows) == 0 || len(bindings) == 0 {
		return nil, false
	}
	name := ""
	for _, c := range bindings[0].unequal {
		if _, bound := rows[0].vars[c.name]; bound && c.relate == "near" {
			name = c.name
		}
	}
	if name == "" {
		return nil, false
	}
	postings := map[string][]int{}
	var loose []binding
	for i, b := range bindings {
		at := slices.IndexFunc(b.unequal, func(c constraint) bool { return c.name == name && c.relate == "near" })
		if at < 0 {
			loose = append(loose, b)
			continue
		}
		for shingle := range m.shingle(b.unequal[at].value) {
			postings[shingle] = append(postings[shingle], i)
		}
	}
	return func(row binding) []binding {
		seen := map[int]bool{}
		out := slices.Clone(loose)
		for shingle := range m.shingle(row.vars[name]) {
			for _, i := range postings[shingle] {
				if !seen[i] {
					seen[i] = true
					out = append(out, bindings[i])
				}
			}
		}
		return out
	}, true
}

type bindingIndex struct {
	keys    []string
	buckets map[string][]binding
	loose   []binding
}

func shared(left, right []binding) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	var keys []string
	for name := range left[0].vars {
		if _, ok := right[0].vars[name]; ok {
			keys = append(keys, name)
		}
	}
	slices.Sort(keys)
	return keys
}

func indexBy(bindings []binding, keys []string) bindingIndex {
	index := bindingIndex{keys: keys, buckets: map[string][]binding{}}
	for _, b := range bindings {
		if key, ok := b.key(keys); ok && len(keys) > 0 {
			index.buckets[key] = append(index.buckets[key], b)
		} else {
			index.loose = append(index.loose, b)
		}
	}
	return index
}

func (i bindingIndex) candidates(row binding) []binding {
	if key, ok := row.key(i.keys); ok && len(i.keys) > 0 {
		return slices.Concat(i.buckets[key], i.loose)
	}
	var all []binding
	for _, bucket := range i.buckets {
		all = append(all, bucket...)
	}
	return append(all, i.loose...)
}

func (b binding) key(names []string) (string, bool) {
	var key strings.Builder
	for _, name := range names {
		value, ok := b.vars[name]
		if !ok {
			return "", false
		}
		key.WriteString(strconv.Quote(value))
	}
	return key.String(), true
}

func (q *Query) row(b binding) (Row, string) {
	row := make(Row, 0, len(q.Captures))
	var key strings.Builder
	for _, name := range q.Captures {
		row = append(row, Cell{Name: name, Elements: b.captures[name]})
		for _, e := range b.captures[name] {
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
				out = append(out, b.capture(p.Capture, e))
			}
		}
		return out, nil
	}
	if p.Type != "" && e.Type != p.Type || e.Type == "corpus" {
		return nil, nil
	}
	seed := binding{}
	for _, a := range p.Attrs {
		if a.Var == "" {
			if m.test(a, e) == a.Negate {
				return nil, nil
			}
			continue
		}
		value, ok := a.value(e)
		if !ok {
			return nil, nil
		}
		bound := binding{vars: map[string]string{a.Var: value}}
		switch {
		case a.Negate:
			bound = binding{unequal: []constraint{{name: a.Var, value: value, relate: "not"}}}
		case a.Relate != "":
			bound = binding{unequal: []constraint{{name: a.Var, value: value, relate: a.Relate, threshold: a.Threshold}}}
		}
		if seed, ok = m.unify(seed, bound); !ok {
			return nil, nil
		}
	}
	type partial struct {
		b    binding
		last int
	}
	results := []partial{{seed, -1}}
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
				if joined, ok := m.unify(r.b, c.b); ok {
					next = append(next, partial{joined, max(r.last, c.last)})
				}
			}
		}
		results = next
	}
	for _, relation := range p.Relations {
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
				if joined, ok := m.unify(r.b, t); ok {
					next = append(next, partial{joined, r.last})
				}
			}
		}
		results = next
	}
	for _, not := range p.Nots {
		candidates, target := descendants(e, false), not.Pattern
		if not.Relation != nil {
			candidates, target = m.related(not.Relation.Kind, e), not.Relation.Target
		}
		var found []binding
		for _, d := range candidates {
			bindings, err := m.at(target, d)
			if err != nil {
				return nil, err
			}
			found = append(found, bindings...)
		}
		results = slices.DeleteFunc(results, func(r partial) bool {
			for _, b := range found {
				if _, ok := m.unify(r.b, b); ok {
					return true
				}
			}
			return false
		})
	}
	out := make([]binding, 0, len(results))
	for _, r := range results {
		out = append(out, r.b.capture(p.Capture, e))
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
						joined, ok := m.unify(cm.b, b)
						if !ok {
							continue
						}
						first := cm.first
						if first < 0 || n.Order < first {
							first = n.Order
						}
						next = append(next, chainMatch{joined, first, max(cm.last, n.Order)})
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
	switch kind {
	case "to":
		if target := m.dereference(skill); target != nil {
			return []*markup.Element{target}
		}
		return nil
	case "from":
		return m.referrers()[skill]
	}
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

func (m *matcher) dereference(link *markup.Element) *markup.Element {
	if link.Type != "link" || link.Attrs["target"] == "" || m.resolve == nil {
		return nil
	}
	target := m.resolve.Skill(link.Attrs["target"])
	path, anchor := link.Attrs["path"], link.Attrs["anchor"]
	if target == nil || path == "" && anchor == "" {
		return target
	}
	at := slices.IndexFunc(target.Children, func(f *markup.Element) bool { return f.Type == "file" && f.Attrs["path"] == path })
	if at < 0 {
		return nil
	}
	file := target.Children[at]
	if anchor == "" {
		return file
	}
	var section *markup.Element
	file.Walk(func(e *markup.Element) bool {
		if e.Type == "section" && e.Attrs["id"] == anchor {
			section = e
		}
		return section == nil
	})
	return section
}

func (m *matcher) referrers() map[*markup.Element][]*markup.Element {
	if m.incoming == nil {
		m.incoming = map[*markup.Element][]*markup.Element{}
		m.root.Walk(func(e *markup.Element) bool {
			if target := m.dereference(e); target != nil {
				m.incoming[target] = append(m.incoming[target], e)
			}
			return true
		})
	}
	return m.incoming
}

func (m *matcher) targets(skill *markup.Element) []*markup.Element {
	var out []*markup.Element
	skill.Walk(func(n *markup.Element) bool {
		if target := n.Attrs["target"]; n.Type == "link" && target != "" {
			if t := m.resolve.Skill(target); t != nil && t != skill {
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

func (b binding) capture(name string, e *markup.Element) binding {
	if name == "" {
		return b
	}
	captures := maps.Clone(b.captures)
	if captures == nil {
		captures = map[string][]*markup.Element{}
	}
	captures[name] = append(slices.Clip(captures[name]), e)
	return binding{captures: captures, vars: b.vars, unequal: b.unequal}
}

func (m *matcher) unify(b, other binding) (binding, bool) {
	lookup := func(name string) (string, bool) {
		if value, ok := b.vars[name]; ok {
			return value, true
		}
		value, ok := other.vars[name]
		return value, ok
	}
	for name, value := range other.vars {
		if existing, ok := b.vars[name]; ok && existing != value {
			return binding{}, false
		}
	}
	for _, constraints := range [][]constraint{b.unequal, other.unequal} {
		for _, c := range constraints {
			if value, bound := lookup(c.name); bound && !m.satisfies(c, value) {
				return binding{}, false
			}
		}
	}
	out := binding{vars: maps.Clone(b.vars), unequal: slices.Concat(b.unequal, other.unequal), captures: maps.Clone(b.captures)}
	for name, value := range other.vars {
		if existing, ok := out.vars[name]; ok && existing != value {
			return binding{}, false
		}
		if out.vars == nil {
			out.vars = map[string]string{}
		}
		out.vars[name] = value
	}
	for _, c := range out.unequal {
		if value, bound := out.vars[c.name]; bound && !m.satisfies(c, value) {
			return binding{}, false
		}
	}
	for name, elements := range other.captures {
		if out.captures == nil {
			out.captures = map[string][]*markup.Element{}
		}
		out.captures[name] = append(slices.Clip(out.captures[name]), elements...)
	}
	return out, true
}

func (a Attr) value(e *markup.Element) (string, bool) {
	switch a.Key {
	case "text":
		return strings.TrimSpace(e.Text), true
	case "words":
		return strconv.Itoa(len(similarity.Words(e.Text))), true
	case "lines":
		return strconv.Itoa(e.EndLine - e.Line + 1), true
	case "chars":
		return strconv.Itoa(utf8.RuneCountInString(e.Text)), true
	case "norm":
		return strings.ToLower(strings.Join(strings.Fields(e.Text), " ")), true
	case "node":
		return fmt.Sprintf("%09d", e.Order), true
	}
	value, ok := e.Attrs[a.Key]
	return value, ok
}

func (m *matcher) test(a Attr, e *markup.Element) bool {
	switch a.Key {
	case "contains":
		needle := a.Value
		if a.Param != "" {
			needle = m.env.Params[a.Param]
		}
		return strings.Contains(strings.ToLower(e.Text), strings.ToLower(needle))
	case "matches":
		return a.Re.MatchString(e.Text)
	}
	value, ok := a.value(e)
	if !ok {
		return false
	}
	if a.Op != "" {
		limit := a.Number
		if a.Param != "" {
			limit, _ = strconv.ParseFloat(m.env.Params[a.Param], 64)
		}
		if a.Op == "older" {
			date, err := time.Parse("2006-01-02", value)
			return err == nil && m.env.Now.Sub(date) > time.Duration(limit*24)*time.Hour
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		switch a.Op {
		case ">":
			return n > limit
		case ">=":
			return n >= limit
		case "<":
			return n < limit
		}
		return n <= limit
	}
	if a.Param != "" {
		return value == m.env.Params[a.Param]
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

func (m *matcher) satisfies(c constraint, bound string) bool {
	switch c.relate {
	case "after":
		return c.value > bound
	case "before":
		return c.value < bound
	case "contains":
		return strings.Contains(strings.ToLower(c.value), strings.ToLower(bound))
	case "near":
		return similarity.Jaccard(m.shingle(c.value), m.shingle(bound)) >= c.threshold
	case "similar":
		a, b := m.vector(c.value), m.vector(bound)
		return a != nil && b != nil && similarity.Cosine(a, b) >= c.threshold
	}
	return c.value != bound
}

func (m *matcher) shingle(text string) map[string]bool {
	if cached, ok := m.shingles[text]; ok {
		return cached
	}
	shingles := similarity.Shingles(text, shingleWords)
	m.shingles[text] = shingles
	return shingles
}

func (m *matcher) vector(text string) []float64 {
	if cached, ok := m.vectors[text]; ok {
		return cached
	}
	vector, err := similarity.Unit(m.embed(text))
	if err != nil && m.failure == nil {
		m.failure = err
	}
	m.vectors[text] = vector
	return vector
}
