package pattern

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

type Group struct {
	Keys       []GroupKey
	Aggregates []Aggregate
	Pos        int
}

type GroupKey struct {
	Var, Capture string
	Pos          int
	value        bool
}

type Aggregate struct {
	Op, Key      string
	Var, Capture string
	value        bool
	Out          string
	OutPos       int
	Filter       string
	Number       float64
	Param        string
	Pos          int
}

var aggregateOps = []string{"count", "sum", "min", "max"}

var aggregateKeys = map[string][]string{
	"count": nil,
	"sum":   {"words", "lines", "chars"},
	"min":   {"words", "lines", "chars", "level"},
	"max":   {"words", "lines", "chars", "level"},
}

func isGroup(t token) bool { return t.kind == tSymbol && t.text == "group" }

func (r *reader) group(open token) (*Group, error) {
	g := &Group{Pos: open.pos}
	for {
		t := r.next()
		switch t.kind {
		case tClose:
			return g, nil
		case tEOF:
			return nil, &Error{Offset: open.pos, Message: "unclosed (group", Hint: "add the matching )"}
		case tVar, tCapture:
			if len(g.Aggregates) > 0 {
				return nil, &Error{Offset: t.pos, Message: "group keys precede the aggregates", Hint: "e.g. (group ?l (count @c @n))"}
			}
			key := GroupKey{Pos: t.pos}
			if t.kind == tVar {
				key.Var = t.text
			} else {
				key.Capture = t.text
			}
			g.Keys = append(g.Keys, key)
		case tOpen:
			a, err := r.aggregate()
			if err != nil {
				return nil, err
			}
			g.Aggregates = append(g.Aggregates, a)
		default:
			return nil, r.unexpected(t, "a ?variable, a @capture, an aggregate such as (count @c @n) or )")
		}
	}
}

func (r *reader) aggregate() (Aggregate, error) {
	op := r.next()
	a := Aggregate{Op: op.text, Pos: op.pos}
	if op.kind != tSymbol || !slices.Contains(aggregateOps, op.text) {
		return a, &Error{Offset: op.pos, Message: fmt.Sprintf("unknown aggregate %q", op.text), Hint: suggest(op.text, aggregateOps)}
	}
	keys := aggregateKeys[op.text]
	if t := r.peek(); t.kind == tKey {
		r.next()
		if !slices.Contains(keys, t.text) {
			hint := "count takes no key; it counts nodes or values"
			if len(keys) > 0 {
				hint = fmt.Sprintf("(%s ...) takes :%s", op.text, strings.Join(keys, " :"))
			}
			return a, &Error{Offset: t.pos, Message: fmt.Sprintf("(%s ...) has no key :%s", op.text, t.text), Hint: hint}
		}
		a.Key = t.text
	}
	operand := r.next()
	switch operand.kind {
	case tCapture:
		a.Capture = operand.text
		if a.Key == "" && op.text != "count" {
			return a, &Error{Offset: operand.pos, Message: fmt.Sprintf("(%s @%s) needs a :key", op.text, operand.text), Hint: fmt.Sprintf("e.g. (%s :%s @%s @n)", op.text, keys[0], operand.text)}
		}
	case tVar:
		if op.text == "sum" {
			return a, &Error{Offset: operand.pos, Message: fmt.Sprintf("(sum ...) takes a @capture, not ?%s", operand.text), Hint: "equal values of distinct nodes would collapse; sum a key over a capture, as in (sum :words @p @n)"}
		}
		if a.Key != "" {
			return a, &Error{Offset: operand.pos, Message: fmt.Sprintf("(%s :%s ...) reads the key from a @capture, not ?%s", op.text, a.Key, operand.text), Hint: fmt.Sprintf("drop :%s to compare the values of ?%s", a.Key, operand.text)}
		}
		a.Var = operand.text
	default:
		return a, r.unexpected(operand, fmt.Sprintf("a @capture or ?variable, as in (%s @c @n)", op.text))
	}
	switch t := r.peek(); t.kind {
	case tCapture:
		r.next()
		if err := r.value(t); err != nil {
			return a, err
		}
		a.Out, a.OutPos = t.text, t.pos
		r.numbers[t.text] = true
	case tVar:
		return a, &Error{Offset: t.pos, Message: fmt.Sprintf("the result of (%s ...) is output, so it is a capture: @%s, not ?%s", op.text, t.text, t.text), Hint: fmt.Sprintf("e.g. (%s @c @%s)", op.text, t.text)}
	}
	if r.peek().kind == tOpen {
		r.next()
		form := r.next()
		if form.kind == tDirect {
			form.text = ">"
		}
		if !slices.Contains([]string{">", ">=", "<", "<="}, form.text) {
			return a, &Error{Offset: form.pos, Message: fmt.Sprintf("an aggregate filters with >, >=, < or <=, not %q", form.text), Hint: fmt.Sprintf("e.g. (%s @c @n (>= 3))", op.text)}
		}
		a.Filter = form.text
		limit := r.next()
		switch limit.kind {
		case tParam:
			a.Param = limit.text
			r.params[limit.text] = true
		case tInt, tFloat:
			a.Number, _ = strconv.ParseFloat(limit.text, 64)
		default:
			return a, r.unexpected(limit, fmt.Sprintf("a number or $param, as in (%s 3)", form.text))
		}
		if c := r.next(); c.kind != tClose {
			return a, r.unexpected(c, fmt.Sprintf(") closing (%s", form.text))
		}
	}
	if c := r.next(); c.kind != tClose {
		return a, r.unexpected(c, fmt.Sprintf(") closing (%s", op.text))
	}
	return a, nil
}

func (g *Group) uses() map[string]int {
	uses := map[string]int{}
	if g == nil {
		return uses
	}
	for _, k := range g.Keys {
		if k.Var != "" {
			uses[k.Var]++
		}
	}
	for _, a := range g.Aggregates {
		if a.Var != "" {
			uses[a.Var]++
		}
	}
	return uses
}

func (g *Group) check(q *Query, mentions []mention) error {
	if g == nil {
		return nil
	}
	repeated := repeatedCaptures(q.Members)
	binds := func(name string) bool {
		return slices.ContainsFunc(mentions, func(m mention) bool { return m.name == name && m.binds && !m.negated })
	}
	captured := func(name string, pos int) error {
		if !slices.Contains(q.Captures, name) {
			return &Error{Offset: pos, Message: fmt.Sprintf("@%s is not captured by a pattern of the query", name), Hint: suggest(name, q.Captures)}
		}
		return nil
	}
	outputs := map[string]bool{}
	for _, a := range g.Aggregates {
		outputs[a.Out] = a.Out != ""
	}
	for i := range g.Keys {
		k := &g.Keys[i]
		if k.Var != "" && !binds(k.Var) {
			return &Error{Offset: k.Pos, Message: fmt.Sprintf("?%s is a group key but no pattern binds it", k.Var), Hint: fmt.Sprintf("bind it with :key ?%s in a pattern of the query", k.Var)}
		}
		if k.Capture == "" {
			continue
		}
		if err := captured(k.Capture, k.Pos); err != nil {
			return err
		}
		if outputs[k.Capture] {
			return &Error{Offset: k.Pos, Message: fmt.Sprintf("@%s is an aggregate of the group, so it cannot key it", k.Capture), Hint: "key the group on a ?variable or a capture of a pattern"}
		}
		k.value = q.Values[k.Capture]
		if repeated[k.Capture] {
			return &Error{Offset: k.Pos, Message: fmt.Sprintf("@%s is captured under * or +, so it cannot key a group", k.Capture), Hint: fmt.Sprintf("key the group on a capture that holds one node per match, or aggregate @%s", k.Capture)}
		}
	}
	for i := range g.Aggregates {
		a := &g.Aggregates[i]
		if a.Var != "" && !binds(a.Var) {
			return &Error{Offset: a.Pos, Message: fmt.Sprintf("?%s is aggregated but no pattern binds it", a.Var), Hint: fmt.Sprintf("bind it with :key ?%s in a pattern of the query", a.Var)}
		}
		if a.Capture != "" {
			if err := captured(a.Capture, a.Pos); err != nil {
				return err
			}
			if outputs[a.Capture] {
				return &Error{Offset: a.Pos, Message: fmt.Sprintf("@%s is an aggregate of the group, so (%s ...) cannot read it", a.Capture, a.Op), Hint: "aggregate a capture or ?variable of the patterns"}
			}
			a.value = q.Values[a.Capture]
			if a.value && (a.Op == "sum" || a.Key != "") {
				return &Error{Offset: a.Pos, Message: fmt.Sprintf("@%s captures a value, so (%s ...) cannot read a key from it", a.Capture, a.Op), Hint: fmt.Sprintf("count it or take its min or max, as in (max @%s @m)", a.Capture)}
			}
		}
	}
	return nil
}

func repeatedCaptures(members []*Pattern) map[string]bool {
	repeated := map[string]bool{}
	var walk func(p *Pattern, under bool)
	walk = func(p *Pattern, under bool) {
		under = under || p.Quant == '*' || p.Quant == '+'
		if under && p.Capture != "" {
			repeated[p.Capture] = true
		}
		for _, a := range p.Attrs {
			for _, name := range []string{a.Capture, a.Score} {
				if under && name != "" {
					repeated[name] = true
				}
			}
		}
		for _, alt := range p.Alts {
			walk(alt, under)
		}
		for _, chain := range p.Chains {
			for _, link := range chain.Links {
				walk(link.Pattern, under)
			}
		}
		for _, relation := range p.Relations {
			walk(relation.Target, under)
		}
	}
	for _, p := range members {
		walk(p, false)
	}
	return repeated
}

func (g *Group) rows(q *Query, bindings []binding, params map[string]Value) []Row {
	index := map[string]int{}
	var groups [][]binding
	if len(g.Keys) == 0 {
		index[""], groups = 0, [][]binding{nil}
	}
	for _, b := range bindings {
		key := g.key(b)
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], b)
	}
	var rows []Row
	for _, members := range groups {
		if row, ok := g.row(q, members, params); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func (g *Group) key(b binding) string {
	var key strings.Builder
	for _, k := range g.Keys {
		if k.Var != "" {
			if value, ok := b.vars[k.Var]; ok {
				key.WriteString(strconv.Quote(value))
			} else {
				key.WriteByte('-')
			}
		}
		if k.value {
			if value, ok := b.vars["@"+k.Capture]; ok {
				key.WriteString(strconv.Quote(value))
			} else {
				key.WriteByte('-')
			}
		} else if k.Capture != "" {
			for _, e := range distinct(b.captures[k.Capture]) {
				key.WriteString(strconv.Itoa(e.Order) + ",")
			}
		}
		key.WriteByte(';')
	}
	return key.String()
}

func (g *Group) row(q *Query, members []binding, params map[string]Value) (Row, bool) {
	nodes := func(name string) []*markup.Element {
		var all []*markup.Element
		for _, b := range members {
			all = append(all, b.captures[name]...)
		}
		return distinct(all)
	}
	values := func(name string) []string {
		var all []string
		for _, b := range members {
			if value, ok := b.vars["@"+name]; ok && !slices.Contains(all, value) {
				all = append(all, value)
			}
		}
		return all
	}
	row := Row{Cells: make([]Cell, 0, len(q.Captures)), Vars: map[string]string{}}
	for _, k := range g.Keys {
		if k.Var == "" {
			continue
		}
		row.Shown = append(row.Shown, k.Var)
		if len(members) > 0 {
			if value, ok := members[0].vars[k.Var]; ok {
				row.Vars[k.Var] = value
			}
		}
	}
	results := map[string]string{}
	for _, a := range g.Aggregates {
		value, ok := a.evaluate(members, nodes)
		if a.Filter != "" && (!ok || !a.passes(value, params)) {
			return Row{}, false
		}
		if a.Out != "" && ok {
			results[a.Out] = strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	for _, name := range q.Captures {
		keyed := slices.ContainsFunc(g.Keys, func(k GroupKey) bool { return k.Capture != "" && k.Capture == name })
		cell := Cell{Name: name, Collected: !keyed}
		switch {
		case slices.ContainsFunc(g.Aggregates, func(a Aggregate) bool { return a.Out == name }):
			cell.Value, cell.Number, cell.Collected = true, true, false
			if value, ok := results[name]; ok {
				cell.Values = []string{value}
			}
		case q.Values[name]:
			cell.Value, cell.Number, cell.Values = true, q.numbers[name], values(name)
		default:
			cell.Elements = nodes(name)
		}
		row.Cells = append(row.Cells, cell)
	}
	return row, true
}

func (a Aggregate) evaluate(members []binding, nodes func(string) []*markup.Element) (float64, bool) {
	var operands []float64
	if a.Capture != "" && !a.value {
		elements := nodes(a.Capture)
		if a.Op == "count" {
			return float64(len(elements)), true
		}
		for _, e := range elements {
			if value, ok := (Attr{Key: a.Key}).value(e); ok {
				if n, ok := finite(value); ok {
					operands = append(operands, n)
				}
			}
		}
	} else {
		name := a.Var
		if a.value {
			name = "@" + a.Capture
		}
		seen := map[string]bool{}
		for _, b := range members {
			if value, ok := b.vars[name]; ok && !seen[value] {
				seen[value] = true
				if n, ok := finite(value); ok {
					operands = append(operands, n)
				}
			}
		}
		if a.Op == "count" {
			return float64(len(seen)), true
		}
	}
	if len(operands) == 0 {
		return 0, false
	}
	switch a.Op {
	case "min":
		return slices.Min(operands), true
	case "max":
		return slices.Max(operands), true
	}
	total := 0.0
	for _, n := range operands {
		total += n
	}
	return total, true
}

func (a Aggregate) passes(value float64, params map[string]Value) bool {
	limit := a.Number
	if a.Param != "" {
		limit = params[a.Param].Number
	}
	switch a.Filter {
	case ">":
		return value > limit
	case ">=":
		return value >= limit
	case "<":
		return value < limit
	}
	return value <= limit
}

func finite(value string) (float64, bool) {
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}

func distinct(elements []*markup.Element) []*markup.Element {
	out := slices.Clone(elements)
	slices.SortFunc(out, func(a, b *markup.Element) int { return a.Order - b.Order })
	return slices.Compact(out)
}
