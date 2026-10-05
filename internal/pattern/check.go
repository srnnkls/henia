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
	output          bool
	member          int
	negated, absent bool
	prior           map[string]bool
}

func (q *Query) check() error {
	if !slices.ContainsFunc(q.Members, func(p *Pattern) bool { return !optionalPattern(p) }) {
		return &Error{Offset: q.Members[0].Pos, Message: "a query needs a pattern that is not optional", Hint: "drop the ? from one pattern, or add a required pattern that the optional ones join"}
	}
	root := &scope{}
	var mentions []mention
	var problem error
	fail := func(err error) {
		if problem == nil {
			problem = err
		}
	}
	var walk func(p *Pattern, s *scope, member int, negated, optional bool, prior map[string]bool)
	walk = func(p *Pattern, s *scope, member int, negated, optional bool, prior map[string]bool) {
		optional = optional || optionalPattern(p)
		enter := func(child *Pattern, avail func() map[string]bool) map[string]bool {
			if optionalPattern(child) {
				return avail()
			}
			return prior
		}
		if negated && p.Capture != "" {
			fail(&Error{Offset: p.Pos, Message: fmt.Sprintf("@%s is inside (not ...), so it never prints", p.Capture), Hint: "capture a node outside the negation"})
		}
		for _, a := range p.Attrs {
			if a.Var != "" {
				mentions = append(mentions, mention{name: a.Var, pos: a.Pos, scope: s, binds: !a.Negate && a.Relate == "", optional: optional, member: member, negated: negated, absent: member < 0, prior: prior})
			}
			if a.Score != "" {
				mentions = append(mentions, mention{name: a.Score, pos: a.Pos, scope: s, binds: true, output: true, optional: optional, member: member, negated: negated, absent: member < 0, prior: prior})
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
			walk(alt, s, member, negated, optional, prior)
		}
		for i, chain := range p.Chains {
			for _, link := range chain.Links {
				walk(link.Pattern, s, member, negated, optional, enter(link.Pattern, func() map[string]bool { return before(p, i, 0) }))
			}
		}
		for i, relation := range p.Relations {
			walk(relation.Target, s, member, negated, optional, enter(relation.Target, func() map[string]bool { return before(p, len(p.Chains), i) }))
		}
		for _, not := range p.Nots {
			target := not.Pattern
			if not.Relation != nil {
				target = not.Relation.Target
			}
			walk(target, &scope{parent: s}, member, true, optional, enter(target, func() map[string]bool { return map[string]bool{} }))
		}
	}
	for i, p := range q.Members {
		prior := map[string]bool{}
		if optionalPattern(p) {
			for j, other := range q.Members {
				if j != i && !optionalPattern(other) {
					maps.Copy(prior, required(other))
				}
			}
		}
		walk(p, root, i, false, false, prior)
	}
	scopes := make([]*scope, len(q.Absent))
	for i, p := range q.Absent {
		scopes[i] = &scope{parent: root}
		walk(p, scopes[i], -1, true, false, map[string]bool{})
	}
	if problem != nil {
		return problem
	}
	if err := q.Group.check(q, mentions); err != nil {
		return err
	}
	uses := q.Group.uses()
	count := map[string]int{}
	for _, m := range mentions {
		count[m.name]++
	}
	for _, m := range mentions {
		if count[m.name]+uses[m.name] == 1 && !m.output {
			return &Error{Offset: m.pos, Message: fmt.Sprintf("?%s appears only once, so it matches anything", m.name), Hint: fmt.Sprintf("use ?%s again to join on it, or drop it", m.name)}
		}
	}
	for _, m := range mentions {
		if m.binds && m.optional && count[m.name] > 1 && !m.prior[m.name] {
			return &Error{Offset: m.pos, Message: fmt.Sprintf("?%s is bound in an optional pattern, so it may be unbound where it is used again", m.name), Hint: fmt.Sprintf("bind ?%s first on the pattern that holds the optional one, or in a required pattern written before it", m.name)}
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
		shares := func(required bool) bool {
			return slices.ContainsFunc(mentions, func(m mention) bool {
				return m.scope.within(scopes[i]) && slices.ContainsFunc(mentions, func(o mention) bool {
					return o.name == m.name && o.binds && o.scope == root && (!required || !o.optional)
				})
			})
		}
		if !shares(false) {
			return &Error{Offset: p.Pos, Message: "(not ...) shares no variable with the patterns of the query, so it drops every row or none", Hint: "use a ?variable bound by another pattern"}
		}
		if !shares(true) {
			return &Error{Offset: p.Pos, Message: "(not ...) shares only variables bound by optional patterns, so it cannot decide", Hint: "use a ?variable bound by a pattern that is not optional"}
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

func optionalPattern(p *Pattern) bool { return p.Quant == '?' || p.Quant == '*' }

func before(p *Pattern, chains, relations int) map[string]bool {
	vars := map[string]bool{}
	for _, a := range p.Attrs {
		if a.Var != "" && !a.Negate && a.Relate == "" {
			vars[a.Var] = true
		}
		if a.Score != "" && !a.Negate {
			vars[a.Score] = true
		}
	}
	for _, chain := range p.Chains[:chains] {
		for _, link := range chain.Links {
			maps.Copy(vars, required(link.Pattern))
		}
	}
	for _, relation := range p.Relations[:relations] {
		maps.Copy(vars, required(relation.Target))
	}
	return vars
}

func required(p *Pattern) map[string]bool {
	if optionalPattern(p) {
		return map[string]bool{}
	}
	vars := before(p, len(p.Chains), len(p.Relations))
	for _, alt := range p.Alts {
		maps.Copy(vars, required(alt))
	}
	return vars
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
