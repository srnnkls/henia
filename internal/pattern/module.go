package pattern

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Rule struct {
	ID       string
	Severity string
	Message  string
	At       []string
	Related  []string
	Focus    string
	Score    string
	Method   string
	Shared   []string
	Query    *Query
	Offset   int
}

type Define struct {
	Name   string
	Params []string
	Body   []token
}

type Rewrite struct {
	ID     string
	Output Template
	At     string
	Query  *Query
}

type Module struct {
	Rules    []*Rule
	Rewrites []*Rewrite
	Defines  map[string]Define
	Imports  []string
}

var ruleID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var placeholder = regexp.MustCompile(`\{([@?$])([A-Za-z_][A-Za-z0-9_-]*)(?:\.([A-Za-z0-9_][A-Za-z0-9_.-]*))?\}`)

func ReadModule(src string, imported func(name string) (map[string]Define, error)) (*Module, error) {
	tokens, err := lex(src)
	if err != nil {
		return nil, err
	}
	r := &reader{tokens: tokens, defines: map[string]Define{}}
	module := &Module{Defines: map[string]Define{}}
	for r.peek().kind != tEOF {
		open := r.next()
		form := r.next()
		if open.kind != tOpen || form.kind != tSymbol {
			return nil, &Error{Offset: open.pos, Message: "a module holds (rule ...), (rewrite ...), (define ...) and (import ...) forms"}
		}
		switch form.text {
		case "import":
			name := r.next()
			if name.kind != tSymbol && name.kind != tString {
				return nil, r.unexpected(name, "a module name")
			}
			if c := r.next(); c.kind != tClose {
				return nil, r.unexpected(c, ") closing (import")
			}
			defines, err := imported(name.text)
			if err != nil {
				return nil, &Error{Offset: name.pos, Message: err.Error()}
			}
			for key, d := range defines {
				if _, taken := r.defines[key]; taken {
					return nil, &Error{Offset: name.pos, Message: fmt.Sprintf("(import %s) defines %s, which is already defined here", name.text, key), Hint: "rename one of the defines"}
				}
				r.defines[key] = d
			}
			module.Imports = append(module.Imports, name.text)
		case "define":
			d, err := r.define(open)
			if err != nil {
				return nil, err
			}
			r.defines[d.Name], module.Defines[d.Name] = d, d
		case "rule":
			rule, err := r.rule(open)
			if err != nil {
				return nil, err
			}
			module.Rules = append(module.Rules, rule)
		case "rewrite":
			rewrite, err := r.rewrite(open)
			if err != nil {
				return nil, err
			}
			module.Rewrites = append(module.Rewrites, rewrite)
		default:
			return nil, &Error{Offset: form.pos, Message: fmt.Sprintf("unknown module form (%s ...)", form.text), Hint: suggest(form.text, []string{"rule", "rewrite", "define", "import"})}
		}
	}
	return module, nil
}

func (r *reader) define(open token) (Define, error) {
	if t := r.next(); t.kind != tOpen {
		return Define{}, r.unexpected(t, "a signature, as in (define (titled ?t) ...)")
	}
	name := r.next()
	if name.kind != tSymbol || isForm(name) || types[name.text] != nil || name.text == "_" {
		return Define{}, &Error{Offset: name.pos, Message: fmt.Sprintf("%q cannot name a pattern", name.text), Hint: "pick a name that is not a type or form"}
	}
	d := Define{Name: name.text}
	for r.peek().kind == tVar || r.peek().kind == tCapture {
		param := r.next()
		d.Params = append(d.Params, sigil(param))
	}
	if t := r.next(); t.kind != tClose {
		return Define{}, r.unexpected(t, "?variables, @captures and ) closing the signature")
	}
	start := r.at
	for r.peek().kind != tClose {
		if err := r.skip(); err != nil {
			return Define{}, err
		}
	}
	d.Body = slices.Clone(r.tokens[start:r.at])
	if len(d.Body) == 0 {
		return Define{}, &Error{Offset: open.pos, Message: fmt.Sprintf("(define (%s ...)) has no pattern", d.Name)}
	}
	for _, t := range d.Body {
		if t.kind == tCapture && !slices.Contains(d.Params, "@"+t.text) {
			return Define{}, &Error{Offset: t.pos, Message: fmt.Sprintf("@%s in (define (%s ...)) is not a parameter", t.text, d.Name), Hint: fmt.Sprintf("add @%s to the signature so callers name the capture", t.text)}
		}
	}
	r.next()
	return d, nil
}

func (r *reader) skip() error {
	open := r.next()
	if open.kind != tOpen && open.kind != tOpenAlt {
		return r.unexpected(open, "a pattern")
	}
	depth := 1
	for depth > 0 {
		switch r.next().kind {
		case tOpen, tOpenAlt:
			depth++
		case tClose, tCloseAlt:
			depth--
		case tEOF:
			return &Error{Offset: open.pos, Message: "unclosed (", Hint: "add the matching )"}
		}
	}
	for r.peek().kind == tQuant || r.peek().kind == tCapture {
		r.next()
	}
	return nil
}

func (r *reader) expand(d Define, open token) error {
	if r.expanded++; r.expanded > 1000 {
		return &Error{Offset: open.pos, Message: fmt.Sprintf("(%s ...) expands without end; a define may not use itself", d.Name)}
	}
	start := r.at - 2
	var args [][]token
	for r.peek().kind != tClose {
		if r.peek().kind == tEOF {
			return &Error{Offset: open.pos, Message: "unclosed (", Hint: "add the matching )"}
		}
		from := r.at
		if r.peek().kind == tOpen || r.peek().kind == tOpenAlt {
			if err := r.skip(); err != nil {
				return err
			}
		} else {
			r.next()
		}
		args = append(args, r.tokens[from:r.at])
	}
	end := r.at
	if len(args) != len(d.Params) {
		return &Error{Offset: open.pos, Message: fmt.Sprintf("(%s ...) takes %d arguments, got %d", d.Name, len(d.Params), len(args))}
	}
	suffix := "_" + strconv.Itoa(r.expanded)
	var body []token
	for _, t := range d.Body {
		if t.kind == tRegex {
			if re, err := regexp.Compile(t.text); err == nil {
				for _, name := range re.SubexpNames() {
					if name == "" {
						continue
					}
					if t.groups == nil {
						t.groups = map[string]string{}
					}
					t.groups[name] = "?" + name + suffix
					if i := slices.Index(d.Params, "?"+name); i >= 0 {
						arg := args[i]
						switch {
						case len(arg) == 1 && arg[0].kind == tVar:
							t.groups[name] = "?" + arg[0].text
						case len(arg) == 1 && (arg[0].kind == tString || arg[0].kind == tSymbol || arg[0].kind == tInt):
							t.groups[name] = "=" + arg[0].text
						default:
							return &Error{Offset: open.pos, Message: fmt.Sprintf("(%s ...) binds ?%s from a regexp group, so it takes a ?variable or a value there", d.Name, name)}
						}
					}
				}
			}
		}
		if t.kind == tVar || t.kind == tCapture {
			if i := slices.Index(d.Params, sigil(t)); i >= 0 {
				body = append(body, args[i]...)
				continue
			}
			t.text += suffix
		}
		t.pos = open.pos
		body = append(body, t)
	}
	r.tokens = slices.Concat(r.tokens[:start], body, r.tokens[end+1:])
	r.at = start
	return nil
}

func (r *reader) rule(open token) (*Rule, error) {
	id := r.next()
	if id.kind != tSymbol || !ruleID.MatchString(id.text) {
		return nil, &Error{Offset: id.pos, Message: "a rule needs an id of lowercase letters, digits and hyphens", Hint: "e.g. (rule large-skill ...)"}
	}
	rule := &Rule{ID: id.text, Severity: "warning", Offset: open.pos}
	for r.peek().kind == tKey {
		key := r.next()
		value := r.next()
		switch key.text {
		case "severity":
			if value.text != "warning" && value.text != "error" {
				return nil, &Error{Offset: value.pos, Message: ":severity is warning or error"}
			}
			rule.Severity = value.text
		case "message":
			if value.kind != tString {
				return nil, r.unexpected(value, "a \"message\"")
			}
			rule.Message = value.text
		case "at", "related":
			if value.kind != tCapture {
				return nil, r.unexpected(value, fmt.Sprintf("a @capture for :%s", key.text))
			}
			captures := []string{value.text}
			for r.peek().kind == tCapture {
				captures = append(captures, r.next().text)
			}
			if key.text == "related" {
				rule.Related = captures
			} else {
				rule.At = captures
			}
		case "focus", "score":
			if value.kind != tVar {
				return nil, r.unexpected(value, fmt.Sprintf("a ?variable for :%s", key.text))
			}
			if key.text == "focus" {
				rule.Focus = value.text
			} else {
				rule.Score = value.text
			}
		case "method":
			if value.kind != tSymbol && value.kind != tString {
				return nil, r.unexpected(value, "a method name for :method")
			}
			rule.Method = value.text
		case "shared":
			second := r.next()
			if value.kind != tCapture || second.kind != tCapture {
				return nil, r.unexpected(value, "two @captures for :shared, as in :shared @copy @original")
			}
			rule.Shared = []string{value.text, second.text}
		default:
			return nil, &Error{Offset: key.pos, Message: fmt.Sprintf("a rule has no key :%s", key.text), Hint: suggest(key.text, []string{"severity", "message", "at", "related", "focus", "score", "method", "shared"})}
		}
	}
	if rule.Message == "" {
		return nil, &Error{Offset: open.pos, Message: fmt.Sprintf("rule %s needs a :message", rule.ID)}
	}
	r.printed = nil
	for _, m := range placeholder.FindAllStringSubmatch(rule.Message, -1) {
		if m[1] == "?" {
			r.printed = append(r.printed, m[2])
		}
	}
	q, err := r.query(tClose)
	if err != nil {
		return nil, err
	}
	r.next()
	rule.Query = q
	for _, capture := range slices.Concat(rule.At, rule.Related, rule.Shared) {
		if capture != "" && !slices.Contains(q.Captures, capture) {
			return nil, &Error{Offset: open.pos, Message: fmt.Sprintf("rule %s points at @%s, which its query does not capture", rule.ID, capture)}
		}
	}
	for _, m := range placeholder.FindAllStringSubmatch(rule.Message, -1) {
		if m[1] == "@" && !slices.Contains(q.Captures, m[2]) {
			return nil, &Error{Offset: open.pos, Message: fmt.Sprintf("rule %s's message uses @%s, which its query does not capture", rule.ID, m[2])}
		}
	}
	return rule, nil
}

func (r *reader) rewrite(open token) (*Rewrite, error) {
	id := r.next()
	if id.kind != tSymbol || !ruleID.MatchString(id.text) {
		return nil, &Error{Offset: id.pos, Message: "a rewrite needs an id of lowercase letters, digits and hyphens", Hint: "e.g. (rewrite served-skill ...)"}
	}
	rewrite := &Rewrite{ID: id.text}
	output := ""
	for r.peek().kind == tKey {
		key := r.next()
		value := r.next()
		switch key.text {
		case "output":
			if value.kind != tString {
				return nil, r.unexpected(value, "an \"output\" template")
			}
			output = value.text
		case "at":
			if value.kind != tCapture {
				return nil, r.unexpected(value, "a @capture for :at")
			}
			rewrite.At = value.text
		default:
			return nil, &Error{Offset: key.pos, Message: fmt.Sprintf("a rewrite has no key :%s", key.text), Hint: suggest(key.text, []string{"output", "at"})}
		}
	}
	r.printed = nil
	for _, m := range placeholder.FindAllStringSubmatch(output, -1) {
		if m[1] == "?" {
			r.printed = append(r.printed, m[2])
		}
	}
	q, err := r.query(tClose)
	if err != nil {
		return nil, err
	}
	r.next()
	rewrite.Query, rewrite.Output = q, ParseTemplate(output)
	if len(q.Captures) == 0 {
		return nil, &Error{Offset: open.pos, Message: fmt.Sprintf("rewrite %s captures no node to replace", rewrite.ID), Hint: "capture the replaced node, as in (reference @r ?kind ?name)"}
	}
	if rewrite.At == "" {
		rewrite.At = q.Captures[0]
	}
	if !slices.Contains(q.Captures, rewrite.At) {
		return nil, &Error{Offset: open.pos, Message: fmt.Sprintf("rewrite %s replaces @%s, which its query does not capture", rewrite.ID, rewrite.At)}
	}
	for _, m := range placeholder.FindAllStringSubmatch(output, -1) {
		if m[1] == "@" && !slices.Contains(q.Captures, m[2]) {
			return nil, &Error{Offset: open.pos, Message: fmt.Sprintf("rewrite %s's output uses @%s, which its query does not capture", rewrite.ID, m[2])}
		}
	}
	return rewrite, nil
}

func (rule *Rule) Params() map[string]bool {
	params := rule.Query.Params()
	for _, m := range placeholder.FindAllStringSubmatch(rule.Message, -1) {
		if _, used := params[m[2]]; m[1] == "$" && !used {
			params[m[2]] = false
		}
	}
	return params
}

func Placeholders(message string) [][]string { return placeholder.FindAllStringSubmatch(message, -1) }

func NewRule(id, severity, message, at, related, query string) (*Rule, error) {
	src := fmt.Sprintf("(rule %s :message %s", id, strconv.Quote(message))
	if severity != "" {
		src += " :severity " + severity
	}
	for _, capture := range strings.Fields(at) {
		src += " :at @" + strings.TrimPrefix(capture, "@")
	}
	for i, capture := range strings.Fields(related) {
		if i == 0 {
			src += " :related"
		}
		src += " @" + strings.TrimPrefix(capture, "@")
	}
	module, err := ReadModule(src+"\n"+query+"\n)", nil)
	if err != nil {
		var pe *Error
		if errors.As(err, &pe) && pe.Offset >= len(src)+1 {
			pe.Offset -= len(src) + 1
			return nil, fmt.Errorf("rule %s query: %s", id, strings.TrimPrefix(pe.Explain(query), "henia query: "))
		}
		return nil, fmt.Errorf("rule %s: %w", id, err)
	}
	return module.Rules[0], nil
}

func sigil(t token) string {
	if t.kind == tCapture {
		return "@" + t.text
	}
	return "?" + t.text
}
