package pattern

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

type scope struct{ parent *scope }

func (s *scope) within(outer *scope) bool {
	for ; s != nil; s = s.parent {
		if s == outer {
			return true
		}
	}
	return false
}

type mention struct {
	name            string
	pos             int
	scope           *scope
	binds, optional bool
	member          int
	negated, absent bool
}

func (q *Query) check() error {
	root := &scope{}
	var mentions []mention
	var problem error
	fail := func(err error) {
		if problem == nil {
			problem = err
		}
	}
	var walk func(p *Pattern, s *scope, member int, negated, optional bool)
	walk = func(p *Pattern, s *scope, member int, negated, optional bool) {
		optional = optional || p.Quant == '?' || p.Quant == '*'
		if negated && p.Capture != "" {
			fail(&Error{Offset: p.Pos, Message: fmt.Sprintf("@%s is inside (not ...), so it never prints", p.Capture), Hint: "capture a node outside the negation"})
		}
		for _, a := range p.Attrs {
			if a.Var != "" {
				mentions = append(mentions, mention{name: a.Var, pos: a.Pos, scope: s, binds: !a.Negate && a.Relate == "", optional: optional, member: member, negated: negated, absent: member < 0})
			}
		}
		if len(p.Alts) > 1 {
			first := bound(p.Alts[0])
			for _, alt := range p.Alts[1:] {
				if other := bound(alt); !maps.Equal(first, other) {
					fail(&Error{Offset: alt.Pos, Message: fmt.Sprintf("alternatives bind different variables: %s and %s", names(first), names(other)), Hint: "bind the same ?variables in every alternative of [ ]"})
				}
			}
		}
		for _, alt := range p.Alts {
			walk(alt, s, member, negated, optional)
		}
		for _, chain := range p.Chains {
			for _, link := range chain.Links {
				walk(link.Pattern, s, member, negated, optional)
			}
		}
		for _, relation := range p.Relations {
			walk(relation.Target, s, member, negated, optional)
		}
		for _, not := range p.Nots {
			target := not.Pattern
			if not.Relation != nil {
				target = not.Relation.Target
			}
			walk(target, &scope{parent: s}, member, true, optional)
		}
	}
	for i, p := range q.Members {
		walk(p, root, i, false, false)
	}
	scopes := make([]*scope, len(q.Absent))
	for i, p := range q.Absent {
		scopes[i] = &scope{parent: root}
		walk(p, scopes[i], -1, true, false)
	}
	if problem != nil {
		return problem
	}
	for _, m := range mentions {
		if !slices.ContainsFunc(mentions, func(o mention) bool { return o.name == m.name && o.pos != m.pos }) {
			return &Error{Offset: m.pos, Message: fmt.Sprintf("?%s appears only once, so it matches anything", m.name), Hint: fmt.Sprintf("use ?%s again to join on it, or drop it", m.name)}
		}
	}
	for _, m := range mentions {
		if m.binds && m.optional && slices.ContainsFunc(mentions, func(o mention) bool { return o.name == m.name && o.pos != m.pos }) {
			return &Error{Offset: m.pos, Message: fmt.Sprintf("?%s is bound in an optional pattern, so it may be unbound where it is used again", m.name), Hint: "bind it in a pattern that always matches, or use it only once"}
		}
	}
	for _, m := range mentions {
		if m.binds {
			continue
		}
		if !slices.ContainsFunc(mentions, func(o mention) bool { return o.name == m.name && o.binds && !o.optional && m.scope.within(o.scope) }) {
			return &Error{Offset: m.pos, Message: fmt.Sprintf("?%s is only compared, never bound", m.name), Hint: fmt.Sprintf("bind it with :key ?%s in a pattern that encloses the comparison", m.name)}
		}
	}
	for i, p := range q.Absent {
		shared := slices.ContainsFunc(mentions, func(m mention) bool {
			return m.scope.within(scopes[i]) && slices.ContainsFunc(mentions, func(o mention) bool { return o.name == m.name && o.binds && o.scope == root })
		})
		if !shared {
			return &Error{Offset: p.Pos, Message: "(not ...) shares no variable with the patterns of the query, so it drops every row or none", Hint: "use a ?variable bound by another pattern"}
		}
	}
	component := make([]int, len(q.Members))
	for i := range component {
		component[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for component[i] != i {
			i = component[i]
		}
		return i
	}
	for _, a := range mentions {
		for _, b := range mentions {
			if a.name == b.name && a.member >= 0 && b.member >= 0 && !a.negated && !b.negated {
				component[find(a.member)] = find(b.member)
			}
		}
	}
	for i := 1; i < len(q.Members); i++ {
		if find(i) != find(0) {
			return &Error{Offset: q.Members[i].Pos, Message: "this pattern shares no variable with the first, so every combination would match: a cartesian product between disconnected patterns", Hint: "share a ?variable between the patterns, or list alternatives in [ ]"}
		}
	}
	return nil
}

func bound(p *Pattern) map[string]bool {
	vars := map[string]bool{}
	var walk func(*Pattern)
	walk = func(p *Pattern) {
		for _, a := range p.Attrs {
			if a.Var != "" && !a.Negate && a.Relate == "" {
				vars[a.Var] = true
			}
		}
		for _, alt := range p.Alts {
			walk(alt)
		}
		for _, chain := range p.Chains {
			for _, link := range chain.Links {
				walk(link.Pattern)
			}
		}
		for _, relation := range p.Relations {
			walk(relation.Target)
		}
	}
	walk(p)
	return vars
}

func names(vars map[string]bool) string {
	if len(vars) == 0 {
		return "none"
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		out = append(out, "?"+name)
	}
	return strings.Join(out, " ")
}
