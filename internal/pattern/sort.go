package pattern

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/srnnkls/henia/internal/markup"
)

type sortKey struct {
	text      string
	desc      bool
	variable  string
	capture   string
	attribute string
	pos       int
}

func parseSort(text string, at int) (sortKey, error) {
	s := sortKey{text: text, pos: at}
	rest, desc := strings.CutPrefix(text, "-")
	s.desc = desc
	switch {
	case strings.HasPrefix(rest, "?") && validName(rest[1:]):
		s.variable = rest[1:]
		return s, nil
	case strings.HasPrefix(rest, "@"):
		if i := strings.LastIndexByte(rest, '.'); i > 1 && validName(rest[1:i]) && validName(rest[i+1:]) {
			s.capture, s.attribute = rest[1:i], rest[i+1:]
			return s, nil
		}
		if validName(rest[1:]) {
			s.capture = rest[1:]
			return s, nil
		}
	}
	return s, &Error{Offset: at, Message: fmt.Sprintf("--sort %s is neither ?var, @value nor @capture.key", text), Hint: "e.g. --sort -@n, --sort ?l or --sort @s.words"}
}

func validName(name string) bool {
	for i := range len(name) {
		if !isName(name[i], i == 0) {
			return false
		}
	}
	return name != ""
}

func (q *Query) checkSorts(mentions []mention) error {
	for _, s := range q.sorts {
		if s.capture != "" {
			if !slices.Contains(q.Captures, s.capture) {
				return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s names @%s, which no pattern captures", s.text, s.capture), Hint: suggest(s.capture, q.Captures)}
			}
			value := q.Values[s.capture] || q.Group != nil && slices.ContainsFunc(q.Group.Aggregates, func(a Aggregate) bool { return a.Out == s.capture })
			switch {
			case value && s.attribute != "":
				return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s reads a key, but @%s captures a value", s.text, s.capture), Hint: fmt.Sprintf("sort by the value itself: --sort @%s", s.capture)}
			case !value && s.attribute == "":
				return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s names @%s, which captures nodes", s.text, s.capture), Hint: fmt.Sprintf("sort by a key of its nodes, as in --sort @%s.words", s.capture)}
			}
			continue
		}
		bound := slices.ContainsFunc(mentions, func(m mention) bool { return m.name == s.variable && m.binds && !m.negated })
		if g := q.Group; g != nil {
			if slices.ContainsFunc(g.Aggregates, func(a Aggregate) bool { return a.Out == s.variable }) {
				return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s names ?%s, but aggregates are captures", s.text, s.variable), Hint: fmt.Sprintf("sort by --sort %s@%s", map[bool]string{true: "-"}[s.desc], s.variable)}
			}
			if !slices.ContainsFunc(g.Keys, func(k GroupKey) bool { return k.Var == s.variable }) {
				if bound {
					return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s names ?%s, which (group ...) drops", s.text, s.variable), Hint: fmt.Sprintf("key the group on ?%s, or sort by a key or aggregate of the group", s.variable)}
				}
				return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s names ?%s, which no pattern or aggregate binds", s.text, s.variable), Hint: "sort by a key or aggregate of the group"}
			}
			continue
		}
		if !bound {
			return &Error{Offset: s.pos, Message: fmt.Sprintf("--sort %s names ?%s, which no pattern binds", s.text, s.variable), Hint: fmt.Sprintf("bind it with :key ?%s in a pattern of the query", s.variable)}
		}
	}
	return nil
}

func (q *Query) sortUses(uses map[string]int) {
	for _, s := range q.sorts {
		if s.variable != "" {
			uses[s.variable]++
		}
	}
}

func (q *Query) sortRows(rows []Row) {
	if len(q.sorts) == 0 {
		return
	}
	if q.Group == nil {
		for i := range rows {
			for _, s := range q.sorts {
				if s.variable != "" && !slices.Contains(rows[i].Shown, s.variable) {
					rows[i].Shown = append(rows[i].Shown, s.variable)
				}
			}
		}
	}
	type keyed struct {
		values  []string
		present []bool
		row     Row
	}
	items := make([]keyed, len(rows))
	numeric := make([]bool, len(q.sorts))
	for k, s := range q.sorts {
		numeric[k] = true
		for i, row := range rows {
			value, ok := s.value(row)
			items[i].values = append(items[i].values, value)
			items[i].present = append(items[i].present, ok)
			if _, err := strconv.ParseFloat(value, 64); ok && err != nil {
				numeric[k] = false
			}
		}
	}
	for i, row := range rows {
		items[i].row = row
	}
	slices.SortStableFunc(items, func(a, b keyed) int {
		for k, s := range q.sorts {
			switch {
			case a.present[k] != b.present[k]:
				if a.present[k] {
					return -1
				}
				return 1
			case !a.present[k]:
				continue
			}
			var c int
			if numeric[k] {
				x, _ := strconv.ParseFloat(a.values[k], 64)
				y, _ := strconv.ParseFloat(b.values[k], 64)
				c = cmp.Compare(x, y)
			} else {
				c = strings.Compare(a.values[k], b.values[k])
			}
			if s.desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		return 0
	})
	for i := range items {
		rows[i] = items[i].row
	}
}

func (s sortKey) value(row Row) (string, bool) {
	if s.variable != "" {
		value, ok := row.Vars[s.variable]
		return value, ok
	}
	if s.attribute == "" {
		return row.Value(s.capture)
	}
	for _, cell := range row.Cells {
		if cell.Name != s.capture || len(cell.Elements) == 0 {
			continue
		}
		first := slices.MinFunc(cell.Elements, func(a, b *markup.Element) int { return cmp.Compare(a.Order, b.Order) })
		return Attr{Key: s.attribute}.value(first)
	}
	return "", false
}
