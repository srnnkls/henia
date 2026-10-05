// Package pattern reads and matches S-expression patterns over document trees.
package pattern

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Query struct {
	Members  []*Pattern
	Absent   []*Pattern
	Captures []string
	Semantic bool
	Group    *Group
	params   map[string]bool
	sorts    []sortKey
}

type Pattern struct {
	Type      string
	Alts      []*Pattern
	Attrs     []Attr
	Chains    []Chain
	Nots      []Negation
	Relations []Relation
	Quant     byte
	Capture   string
	Pos       int
}

type Chain struct {
	Links                 []Link
	FirstChild, LastChild bool
}

type Relation struct {
	Kind   string
	Target *Pattern
}

type Negation struct {
	Pattern  *Pattern
	Relation *Relation
}

var relations = []string{"reaches", "inbound", "to", "from"}

func isForm(t token) bool {
	return t.kind == tSymbol && (t.text == "not" || slices.Contains(relations, t.text))
}

type Link struct {
	Pattern *Pattern
	Direct  bool
}

type Attr struct {
	Key       string
	Value     string
	Lo, Hi    int
	Range     bool
	Re        *regexp.Regexp
	Var       string
	Negate    bool
	Relate    string
	Threshold float64
	Pos       int
	Param     string
	Op        string
	Number    float64
	Limit     string
	Score     string
}

type Error struct {
	Offset  int
	Message string
	Hint    string
}

func (e *Error) Error() string { return fmt.Sprintf("%s at offset %d", e.Message, e.Offset) }

func (e *Error) Explain(query string) string {
	line, column := query, e.Offset
	if start := strings.LastIndexByte(query[:min(e.Offset, len(query))], '\n'); start >= 0 {
		line, column = query[start+1:], e.Offset-start-1
	}
	if end := strings.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}
	text := fmt.Sprintf("henia query: %s\n  %s\n  %s^\n", e.Message, line, strings.Repeat(" ", max(column, 0)))
	if e.Hint != "" {
		text += "  " + e.Hint + "\n"
	}
	return text
}

var types = map[string][]string{
	"skill":       {"id", "name", "source"},
	"file":        {"path", "main", "kind", "name", "artifact", "file", "dependency"},
	"section":     {"id", "title", "level"},
	"heading":     {"level"},
	"paragraph":   nil,
	"list":        {"ordered"},
	"item":        nil,
	"quote":       nil,
	"table":       nil,
	"code":        {"lang"},
	"row":         {"table", "key", "value", "index"},
	"link":        {"target", "path", "anchor", "url", "ref", "name", "artifact", "dest", "exists", "valid", "anchored", "problem"},
	"image":       {"url", "dest", "exists", "valid", "anchored", "problem"},
	"entry":       {"key", "value", "index", "tag"},
	"problem":     {"kind", "message"},
	"slot":        {"role", "slot", "type", "entry", "error"},
	"directive":   nil,
	"frontmatter": nil,
}

var textKeys = []string{"text", "contains", "matches", "words", "node", "position", "lines", "chars", "norm", "dynamic"}

var numericKeys = []string{"level", "words", "lines", "chars"}

func openKeys(kind string) bool { return kind == "_" || kind == "directive" || kind == "frontmatter" }

type tokenKind int

const (
	tEOF tokenKind = iota
	tOpen
	tClose
	tOpenAlt
	tCloseAlt
	tDirect
	tAnchor
	tQuant
	tCapture
	tKey
	tString
	tInt
	tRange
	tSymbol
	tRegex
	tVar
	tFloat
	tParam
)

type token struct {
	kind   tokenKind
	text   string
	pos    int
	lo, hi int
}

func isName(c byte, first bool) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || !first && (c >= '0' && c <= '9' || c == '-' || c == '.')
}

func isSymbol(c byte, first bool) bool { return isName(c, first) || !first && c == '/' }

func lex(src string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == ';':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '$' && i+1 < len(src) && isName(src[i+1], true):
			j := i + 1
			for j < len(src) && isName(src[j], j == i+1) {
				j++
			}
			tokens = append(tokens, token{kind: tParam, text: src[i+1 : j], pos: i})
			i = j
		case c == '<' || c == '>' && i+1 < len(src) && src[i+1] == '=':
			j := i + 1
			if j < len(src) && src[j] == '=' {
				j++
			}
			tokens = append(tokens, token{kind: tSymbol, text: src[i:j], pos: i})
			i = j
		case c == '?' && i+1 < len(src) && isName(src[i+1], true):
			j := i + 1
			for j < len(src) && isName(src[j], j == i+1) {
				j++
			}
			tokens = append(tokens, token{kind: tVar, text: src[i+1 : j], pos: i})
			i = j
		case strings.IndexByte("()[]>?*+", c) >= 0:
			kind := map[byte]tokenKind{'(': tOpen, ')': tClose, '[': tOpenAlt, ']': tCloseAlt, '>': tDirect, '?': tQuant, '*': tQuant, '+': tQuant}[c]
			tokens = append(tokens, token{kind: kind, text: string(c), pos: i})
			i++
		case strings.HasPrefix(src[i:], "..") && i+2 < len(src) && src[i+2] >= '0' && src[i+2] <= '9':
			j := i + 2
			for j < len(src) && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			hi, _ := strconv.Atoi(src[i+2 : j])
			tokens = append(tokens, token{kind: tRange, text: src[i:j], pos: i, hi: hi})
			i = j
		case c == '.':
			tokens = append(tokens, token{kind: tAnchor, text: ".", pos: i})
			i++
		case c == '@' || c == ':':
			j := i + 1
			for j < len(src) && isName(src[j], j == i+1) {
				j++
			}
			if j == i+1 {
				what := map[byte]string{'@': "a capture name", ':': "a key"}[c]
				return nil, &Error{Offset: i, Message: fmt.Sprintf("expected %s after %q", what, c)}
			}
			kind := map[byte]tokenKind{'@': tCapture, ':': tKey}[c]
			tokens = append(tokens, token{kind: kind, text: src[i+1 : j], pos: i})
			i = j
		case c == '/':
			j := i + 1
			for ; j < len(src) && src[j] != '/'; j++ {
				if src[j] == '\\' {
					j++
				}
			}
			if j >= len(src) {
				return nil, &Error{Offset: i, Message: "unterminated /regexp/", Hint: `quote values that contain /, as in :path "ops/run.md"`}
			}
			tokens = append(tokens, token{kind: tRegex, text: strings.ReplaceAll(src[i+1:j], `\/`, "/"), pos: i})
			i = j + 1
		case c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(src) && src[j] != '"'; j++ {
				if src[j] == '\\' && j+1 < len(src) {
					j++
					switch src[j] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					default:
						b.WriteByte(src[j])
					}
					continue
				}
				b.WriteByte(src[j])
			}
			if j >= len(src) {
				return nil, &Error{Offset: i, Message: "unterminated string"}
			}
			tokens = append(tokens, token{kind: tString, text: b.String(), pos: i})
			i = j + 1
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			lo, _ := strconv.Atoi(src[i:j])
			if strings.HasPrefix(src[j:], "..") {
				k := j + 2
				for k < len(src) && src[k] >= '0' && src[k] <= '9' {
					k++
				}
				hi := math.MaxInt
				if k > j+2 {
					hi, _ = strconv.Atoi(src[j+2 : k])
				}
				tokens = append(tokens, token{kind: tRange, text: src[i:k], pos: i, lo: lo, hi: hi})
				i = k
				continue
			}
			if j+1 < len(src) && src[j] == '.' && src[j+1] >= '0' && src[j+1] <= '9' {
				k := j + 1
				for k < len(src) && src[k] >= '0' && src[k] <= '9' {
					k++
				}
				tokens = append(tokens, token{kind: tFloat, text: src[i:k], pos: i})
				i = k
				continue
			}
			tokens = append(tokens, token{kind: tInt, text: src[i:j], pos: i, lo: lo, hi: lo})
			i = j
		case isName(c, true):
			j := i
			for j < len(src) && isSymbol(src[j], j == i) {
				j++
			}
			tokens = append(tokens, token{kind: tSymbol, text: src[i:j], pos: i})
			i = j
		default:
			hint := "patterns use ( ) [ ] > . ? * + @capture :key \"string\" /regexp/"
			if c == '\'' {
				hint = "quote strings with \" inside the single-quoted shell argument"
			}
			return nil, &Error{Offset: i, Message: fmt.Sprintf("unexpected %q", c), Hint: hint}
		}
	}
	return append(tokens, token{kind: tEOF, pos: len(src)}), nil
}

type reader struct {
	tokens   []token
	at       int
	captures []string
	semantic bool
	params   map[string]bool
	defines  map[string]Define
	expanded int
	sorts    []sortKey
}

func Read(src string, sort ...string) (*Query, error) {
	tokens, err := lex(src)
	if err != nil {
		return nil, err
	}
	r := &reader{tokens: tokens}
	for _, text := range sort {
		key, err := parseSort(text, len(src))
		if err != nil {
			return nil, err
		}
		r.sorts = append(r.sorts, key)
	}
	return r.query(tEOF)
}

func (r *reader) query(end tokenKind) (*Query, error) {
	r.captures, r.semantic, r.params = nil, false, map[string]bool{}
	start := r.peek().pos
	q := &Query{}
	for r.peek().kind != end {
		if r.peek().kind == tEOF {
			return nil, &Error{Offset: start, Message: "unclosed (rule", Hint: "add the matching )"}
		}
		head := r.tokens[r.at+1]
		if define, ok := r.defines[head.text]; ok && r.peek().kind == tOpen {
			r.at += 2
			if err := r.expand(define, r.tokens[r.at-2]); err != nil {
				return nil, err
			}
			continue
		}
		if r.peek().kind == tOpen && isGroup(head) {
			open := r.next()
			r.next()
			if q.Group != nil {
				return nil, &Error{Offset: open.pos, Message: "a query takes one (group ...)", Hint: "list every key and aggregate in one (group ...)"}
			}
			g, err := r.group(open)
			if err != nil {
				return nil, err
			}
			q.Group = g
			continue
		}
		if q.Group != nil {
			return nil, &Error{Offset: r.peek().pos, Message: "(group ...) follows the patterns of a query", Hint: "move the pattern before (group ...)"}
		}
		if r.peek().kind == tOpen && head.kind == tSymbol && head.text == "not" {
			r.at += 2
			p, err := r.pattern("")
			if err != nil {
				return nil, err
			}
			if c := r.next(); c.kind != tClose {
				return nil, r.unexpected(c, ") closing (not")
			}
			q.Absent = append(q.Absent, p)
			continue
		}
		p, err := r.pattern("?")
		if err != nil {
			return nil, err
		}
		q.Members = append(q.Members, p)
	}
	if len(q.Members) == 0 {
		return nil, &Error{Offset: start, Message: "a query needs a pattern outside (not ...)", Hint: `try (skill :id "NAME" (heading) @h)`}
	}
	q.Captures, q.Semantic, q.params, q.sorts = r.captures, r.semantic, r.params, r.sorts
	if len(q.Captures) == 0 {
		if len(q.Members) > 1 || len(q.Absent) > 0 {
			return nil, &Error{Offset: start, Message: "a query of several patterns prints its captures; name one with @", Hint: `e.g. (link :target ?s) @l (skill :id ?s)`}
		}
		q.Captures = []string{""}
	}
	if err := q.check(); err != nil {
		return nil, err
	}
	return q, nil
}

func (r *reader) peek() token { return r.tokens[r.at] }
func (r *reader) next() token {
	t := r.tokens[r.at]
	if t.kind != tEOF {
		r.at++
	}
	return t
}

const nested = "?*+"

func (r *reader) pattern(quants string) (*Pattern, error) {
	open := r.next()
	p := &Pattern{Pos: open.pos}
	switch open.kind {
	case tOpenAlt:
		for r.peek().kind != tCloseAlt {
			if r.peek().kind == tEOF {
				return nil, &Error{Offset: open.pos, Message: "unclosed [", Hint: "close the alternation with ]"}
			}
			alt, err := r.pattern(nested)
			if err != nil {
				return nil, err
			}
			p.Alts = append(p.Alts, alt)
		}
		r.next()
		if len(p.Alts) == 0 {
			return nil, &Error{Offset: open.pos, Message: "empty alternation", Hint: "list patterns inside [ ], e.g. [(code) (quote)]"}
		}
	case tOpen:
		head := r.next()
		if head.kind != tSymbol {
			return nil, &Error{Offset: head.pos, Message: "expected a type after (", Hint: "types: " + strings.Join(typeNames(), " ")}
		}
		if define, ok := r.defines[head.text]; ok {
			if err := r.expand(define, open); err != nil {
				return nil, err
			}
			return r.pattern(quants)
		}
		if isGroup(head) {
			return nil, &Error{Offset: head.pos, Message: "(group ...) belongs at the top level of a query, after its patterns", Hint: `e.g. (code :lang ?l) @c (group ?l (count @c ?n))`}
		}
		if isForm(head) {
			return nil, &Error{Offset: head.pos, Message: fmt.Sprintf("(%s ...) belongs inside a pattern", head.text), Hint: `e.g. (skill :id "X" (` + head.text + ` (skill) @t))`}
		}
		if _, known := types[head.text]; !known && head.text != "_" {
			return nil, &Error{Offset: head.pos, Message: fmt.Sprintf("unknown type %q", head.text), Hint: suggest(head.text, typeNames())}
		}
		p.Type = head.text
		if err := r.body(p, open); err != nil {
			return nil, err
		}
		if p.Type == "_" {
			p.Type = ""
		}
	default:
		return nil, r.unexpected(open, "a pattern such as (heading)")
	}
	if t := r.peek(); t.kind == tQuant {
		if !strings.Contains(quants, t.text) {
			hint := "nest it in another pattern, as (skill (code)+ @c)"
			if strings.Contains(quants, "?") {
				hint += ", or use ? for an optional top-level pattern"
			}
			return nil, &Error{Offset: t.pos, Message: "quantifiers apply to patterns nested in another pattern", Hint: hint}
		}
		p.Quant = r.next().text[0]
	}
	if t := r.peek(); t.kind == tCapture {
		r.next()
		if p.Capture != "" {
			return nil, &Error{Offset: t.pos, Message: "a pattern takes one capture"}
		}
		r.name(p, t.text)
	}
	return p, nil
}

func (r *reader) name(p *Pattern, capture string) {
	p.Capture = capture
	if !slices.Contains(r.captures, capture) {
		r.captures = append(r.captures, capture)
	}
}

func (r *reader) body(p *Pattern, open token) error {
	afterPattern, connect, firstChild := false, false, false
	for {
		t := r.peek()
		switch t.kind {
		case tClose:
			r.next()
			if connect {
				p.Chains[len(p.Chains)-1].LastChild = true
			} else if firstChild {
				return &Error{Offset: t.pos, Message: "anchor . needs a pattern after it"}
			}
			return nil
		case tEOF:
			return &Error{Offset: open.pos, Message: "unclosed (", Hint: "add the matching )"}
		case tKey:
			attr, err := r.attr(p.Type)
			if err != nil {
				return err
			}
			p.Attrs = append(p.Attrs, attr)
			afterPattern = false
		case tCapture:
			r.next()
			if p.Capture != "" {
				return &Error{Offset: t.pos, Message: "a pattern takes one capture"}
			}
			r.name(p, t.text)
		case tAnchor:
			r.next()
			if connect || firstChild {
				return &Error{Offset: t.pos, Message: "two anchors in a row"}
			}
			connect, firstChild = afterPattern, !afterPattern
		case tOpen, tOpenAlt, tDirect:
			direct := t.kind == tDirect
			if direct {
				r.next()
			}
			if form := r.tokens[r.at+1]; r.peek().kind == tOpen && isForm(form) {
				if direct || connect || firstChild {
					return &Error{Offset: t.pos, Message: fmt.Sprintf("(%s ...) cannot be anchored or marked direct", form.text)}
				}
				if form.text != "not" {
					relation, err := r.relation()
					if err != nil {
						return err
					}
					p.Relations = append(p.Relations, relation)
					afterPattern = false
					continue
				}
				r.at += 2
				var negation Negation
				if inner := r.tokens[r.at+1]; r.peek().kind == tOpen && isForm(inner) && inner.text != "not" {
					relation, err := r.relation()
					if err != nil {
						return err
					}
					negation.Relation = &relation
				} else {
					pattern, err := r.pattern(nested)
					if err != nil {
						return err
					}
					negation.Pattern = pattern
				}
				if c := r.next(); c.kind != tClose {
					return r.unexpected(c, ") closing (not")
				}
				p.Nots = append(p.Nots, negation)
				afterPattern = false
				continue
			}
			child, err := r.pattern(nested)
			if err != nil {
				return err
			}
			if connect {
				if direct {
					return &Error{Offset: t.pos, Message: "> marks the first pattern of a sibling chain only"}
				}
				last := &p.Chains[len(p.Chains)-1]
				last.Links = append(last.Links, Link{Pattern: child})
			} else {
				p.Chains = append(p.Chains, Chain{Links: []Link{{Pattern: child, Direct: direct}}, FirstChild: firstChild})
			}
			afterPattern, connect, firstChild = true, false, false
		default:
			return r.unexpected(r.next(), "a key, a nested pattern or )")
		}
	}
}

func (r *reader) relation() (Relation, error) {
	r.next()
	form := r.next()
	target, err := r.pattern(nested)
	if err != nil {
		return Relation{}, err
	}
	if c := r.next(); c.kind != tClose {
		return Relation{}, r.unexpected(c, fmt.Sprintf(") closing (%s", form.text))
	}
	return Relation{Kind: form.text, Target: target}, nil
}

func (r *reader) attr(kind string) (Attr, error) {
	key := r.next()
	value := r.next()
	attr := Attr{Key: key.text}
	known := slices.Concat(types[kind], textKeys)
	if !openKeys(kind) && !slices.Contains(known, key.text) {
		return attr, &Error{Offset: key.pos, Message: fmt.Sprintf("%s has no key :%s", kind, key.text), Hint: suggest(key.text, known) + "; keys: :" + strings.Join(known, " :")}
	}
	if value.kind == tOpen {
		form := r.next()
		if form.kind == tDirect {
			form.text = ">"
		}
		switch form.text {
		case "not":
			attr.Negate = true
			value = r.next()
		case ">", ">=", "<", "<=", "older":
			attr.Op = form.text
			operand := r.next()
			switch operand.kind {
			case tParam:
				attr.Param = operand.text
				r.params[operand.text] = true
			case tInt, tFloat:
				attr.Number, _ = strconv.ParseFloat(operand.text, 64)
			default:
				return attr, r.unexpected(operand, fmt.Sprintf("a number or $param, as in (%s 500)", form.text))
			}
			if c := r.next(); c.kind != tClose {
				return attr, r.unexpected(c, fmt.Sprintf(") closing (%s", form.text))
			}
			return attr, nil
		case "contains", "covers":
			attr.Relate = form.text
			value = r.next()
			if value.kind != tVar {
				return attr, r.unexpected(value, fmt.Sprintf("a ?variable, as in (%s ?x)", form.text))
			}
		case "after", "before", "near", "overlap", "similar":
			attr.Relate = form.text
			value = r.next()
			if value.kind != tVar {
				return attr, r.unexpected(value, fmt.Sprintf("a ?variable, as in (%s ?x%s)", form.text, map[string]string{"after": "", "before": "", "near": " 0.7", "overlap": " 0.9", "similar": " 0.8"}[form.text]))
			}
			if form.text == "near" || form.text == "overlap" || form.text == "similar" {
				threshold := r.next()
				if threshold.kind == tParam {
					attr.Limit = threshold.text
					r.params[threshold.text] = true
				} else {
					t, err := strconv.ParseFloat(threshold.text, 64)
					if threshold.kind != tFloat && threshold.kind != tInt || err != nil || t <= 0 || t > 1 {
						return attr, &Error{Offset: threshold.pos, Message: fmt.Sprintf("(%s ?x T) takes a threshold above 0 and at most 1", form.text), Hint: fmt.Sprintf("e.g. (%s ?t 0.7)", form.text)}
					}
					attr.Threshold = t
				}
				if r.peek().kind == tVar {
					attr.Score = r.next().text
				}
				r.semantic = r.semantic || form.text == "similar"
			}
		default:
			return attr, r.unexpected(form, "not, after, before, contains, covers, near, overlap, similar, older or a comparison, as in (not ?x), (near ?t 0.7) or (> 500)")
		}
		if c := r.next(); c.kind != tClose {
			return attr, r.unexpected(c, fmt.Sprintf(") closing (%s", form.text))
		}
	}
	if value.kind == tVar {
		if key.text == "contains" || key.text == "matches" {
			return attr, &Error{Offset: value.pos, Message: fmt.Sprintf(":%s takes a string, not a variable", key.text), Hint: "bind the text with :text ?x"}
		}
		attr.Var, attr.Pos = value.text, value.pos
		return attr, nil
	}
	switch value.kind {
	case tParam:
		if key.text == "matches" {
			return attr, &Error{Offset: value.pos, Message: ":matches takes a /regexp/ or string, not a $param"}
		}
		attr.Param = value.text
		r.params[value.text] = r.params[value.text] || false
		return attr, nil
	case tString, tSymbol, tInt:
		attr.Value = value.text
		attr.Lo, attr.Hi = value.lo, value.hi
	case tRange:
		attr.Range, attr.Lo, attr.Hi = true, value.lo, value.hi
	case tRegex:
		re, err := regexp.Compile(value.text)
		if err != nil {
			return attr, &Error{Offset: value.pos, Message: "invalid regular expression: " + err.Error()}
		}
		attr.Re = re
		return attr, nil
	default:
		return attr, r.unexpected(value, fmt.Sprintf("a value for :%s", key.text))
	}
	if attr.Range && !slices.Contains(numericKeys, key.text) {
		return attr, &Error{Offset: value.pos, Message: "ranges apply to :level and :words only"}
	}
	if slices.Contains(numericKeys, key.text) && !attr.Range {
		n, err := strconv.Atoi(attr.Value)
		if err != nil {
			return attr, &Error{Offset: value.pos, Message: fmt.Sprintf(":%s takes a number or a range", key.text), Hint: fmt.Sprintf(":%s 2, :%s 1..3 or :%s 80..", key.text, key.text, key.text)}
		}
		attr.Range, attr.Lo, attr.Hi = true, n, n
	}
	if key.text == "matches" {
		re, err := regexp.Compile(attr.Value)
		if err != nil {
			return attr, &Error{Offset: value.pos, Message: "invalid regular expression: " + err.Error()}
		}
		attr.Re = re
	}
	return attr, nil
}

func (r *reader) unexpected(t token, want string) error {
	got := fmt.Sprintf("%q", t.text)
	if t.kind == tEOF {
		got = "end of query"
	}
	return &Error{Offset: t.pos, Message: fmt.Sprintf("expected %s, found %s", want, got)}
}

func typeNames() []string {
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func Suggest(word string, candidates []string) string { return suggest(word, candidates) }

func suggest(word string, candidates []string) string {
	best, distance := "", 3
	for _, c := range candidates {
		if strings.HasPrefix(word, c) || strings.HasPrefix(c, word) {
			return fmt.Sprintf("did you mean %s?", c)
		}
		if d := levenshtein(word, c); d < distance {
			best, distance = c, d
		}
	}
	if best == "" {
		return "valid: " + strings.Join(candidates, " ")
	}
	return fmt.Sprintf("did you mean %s?", best)
}

func levenshtein(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		previous := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			previous, row[j] = row[j], min(row[j]+1, row[j-1]+1, previous+cost)
		}
	}
	return row[len(b)]
}
