package components

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/templating"
)

type Document struct {
	Path   string
	Source []byte
}

func Check(docs []Document, decls map[string]Declaration, known map[string]bool) []Diagnostic {
	used := map[string]bool{}
	var diags []Diagnostic
	for _, doc := range docs {
		diags = append(diags, checkDocument(doc, decls, used)...)
	}
	names := slices.Sorted(maps.Keys(decls))
	for _, name := range names {
		decl := decls[name]
		if decl.Path == "" {
			continue
		}
		diags = append(diags, checkTemplate(decl, known)...)
		for use := range templateUses(decl) {
			if use != name {
				used[use] = true
			}
		}
	}
	for _, name := range names {
		if decl := decls[name]; !decl.Builtin && !used[name] {
			diags = append(diags, Diagnostic{ID: UnusedComponent, Level: LevelWarning, Path: decl.Path, Message: fmt.Sprintf("component %s is never called", name)})
		}
	}
	slices.SortStableFunc(diags, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
	})
	return diags
}

type document struct {
	path   string
	source []byte
	offset int
	diags  []Diagnostic
}

func (d *document) report(id string, offset int, format string, args ...any) {
	line, column := position(d.source, offset)
	d.diags = append(d.diags, Diagnostic{ID: id, Level: LevelError, Path: d.path, Line: d.offset + line, Column: column, Message: fmt.Sprintf(format, args...)})
}

func (d *document) at(id string, e *markup.Element, format string, args ...any) {
	d.diags = append(d.diags, Diagnostic{ID: id, Level: LevelError, Path: d.path, Line: d.offset + e.Line, Column: e.Column, Message: fmt.Sprintf(format, args...)})
}

func checkDocument(doc Document, decls map[string]Declaration, used map[string]bool) []Diagnostic {
	_, body, err := artifact.SplitFrontmatter(string(doc.Source))
	if err != nil {
		return []Diagnostic{{ID: InvalidMarkup, Level: LevelError, Path: doc.Path, Line: 1, Column: 1, Message: err.Error()}}
	}
	d := &document{path: doc.Path, source: []byte(body)}
	d.offset = bytes.Count(doc.Source[:len(doc.Source)-len(body)], []byte{'\n'})
	root, err := markup.Tree(d.source)
	d.markupErrors(err)
	var code [][2]int
	var visit func(e *markup.Element)
	visit = func(e *markup.Element) {
		switch e.Type {
		case "code":
			code = append(code, [2]int{e.Start, e.End})
		case "directive":
			name := e.Attrs["name"]
			used[name] = true
			if name == "include" && e.Attrs["template"] != "" {
				used[e.Attrs["template"]] = true
			}
			if decl, ok := decls[name]; !ok {
				d.at(UnknownComponent, e, "unknown component %s", name)
			} else if d.call(e, decl); decl.Body.Kind == "text" {
				return
			}
		}
		for _, c := range e.Children {
			visit(c)
		}
	}
	visit(root)
	d.templateActions(code)
	return d.diags
}

func (d *document) markupErrors(err error) {
	if err == nil {
		return
	}
	list := []error{err}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		list = multi.Unwrap()
	}
	for _, e := range list {
		var me *markup.Error
		if !errors.As(e, &me) {
			d.diags = append(d.diags, Diagnostic{ID: InvalidMarkup, Level: LevelError, Path: d.path, Line: d.offset + 1, Column: 1, Message: e.Error()})
			continue
		}
		d.diags = append(d.diags, Diagnostic{ID: cmp.Or(me.ID, InvalidMarkup), Level: LevelError, Path: d.path, Line: d.offset + me.Line, Column: me.Column, Message: me.Message})
	}
}

func (d *document) call(e *markup.Element, decl Declaration) {
	form := formOf(d.source, e)
	d.attributes(e, decl, form == Text)
	if !decl.Allows(form) {
		d.at(WrongForm, e, "%s cannot be called in %s form; want %s", decl.Name, form, strings.Join(decl.Forms, ", "))
		return
	}
	d.body(e, decl, form)
}

func formOf(source []byte, e *markup.Element) string {
	switch {
	case e.Attrs["inline"] == "true":
		return Text
	case bytes.HasPrefix(source[e.Start:], []byte(":::")):
		return Container
	}
	return Leaf
}

func (d *document) attributes(e *markup.Element, decl Declaration, inline bool) {
	for _, name := range slices.Sorted(maps.Keys(e.Attrs)) {
		if name == "name" || inline && name == "inline" {
			continue
		}
		attr, ok := decl.Attributes[name]
		if !ok {
			if !decl.Open {
				d.at(UnknownAttribute, e, "%s does not declare attribute %s", decl.Name, name)
			}
			continue
		}
		if _, err := attr.Coerce(e.Attrs[name]); err != nil {
			d.at(InvalidAttribute, e, "%s attribute %s: %v", decl.Name, name, err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(decl.Attributes)) {
		if _, ok := e.Attrs[name]; decl.Attributes[name].Required && !ok {
			d.at(MissingAttribute, e, "%s requires attribute %s", decl.Name, name)
		}
	}
}

func (d *document) body(e *markup.Element, decl Declaration, form string) {
	empty := len(e.Children) == 0
	if form != Container {
		empty = strings.TrimSpace(label(d.source, e)) == ""
	}
	switch {
	case decl.Body.Kind == "none" && !empty:
		d.at(InvalidBody, e, "%s takes no body", decl.Name)
	case decl.Body.Required && empty:
		d.at(InvalidBody, e, "%s requires a body", decl.Name)
	}
	if decl.Body.Kind != "markdown" || len(decl.Body.Children) == 0 {
		return
	}
	for _, child := range children(e) {
		kind := child.Type
		if kind == "directive" {
			kind = child.Attrs["name"]
		}
		if !slices.Contains(decl.Body.Children, kind) {
			d.at(DisallowedChild, child, "%s does not allow %s children; want %s", decl.Name, kind, strings.Join(decl.Body.Children, ", "))
		}
	}
}

func children(e *markup.Element) []*markup.Element {
	var out []*markup.Element
	for _, c := range e.Children {
		if c.Type == "section" {
			out = append(out, children(c)...)
			continue
		}
		out = append(out, c)
	}
	return out
}

func label(source []byte, e *markup.Element) string {
	rest := bytes.TrimLeft(source[e.Start:e.End], ":")
	rest = rest[min(len(e.Attrs["name"]), len(rest)):]
	if len(rest) == 0 || rest[0] != '[' {
		return ""
	}
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '\\':
			i++
		case '[':
			depth++
		case ']':
			if depth--; depth == 0 {
				return string(rest[1:i])
			}
		}
	}
	return ""
}

func (d *document) templateActions(code [][2]int) {
	for i := 0; ; {
		j := bytes.Index(d.source[i:], []byte("{{"))
		if j < 0 {
			return
		}
		at := i + j
		i = at + 2
		if slices.ContainsFunc(code, func(span [2]int) bool { return span[0] <= at && at < span[1] }) {
			continue
		}
		d.report(TemplateAction, at, "skill bodies take no Go template actions; use a component")
	}
}

func position(source []byte, offset int) (int, int) {
	return bytes.Count(source[:offset], []byte{'\n'}) + 1, offset - bytes.LastIndexByte(source[:offset], '\n')
}

func checkTemplate(decl Declaration, known map[string]bool) []Diagnostic {
	source, err := os.ReadFile(decl.Path)
	if err != nil {
		return []Diagnostic{{ID: InvalidTemplate, Level: LevelError, Path: decl.Path, Message: err.Error()}}
	}
	text := decl.Template
	offset := 0
	if strings.HasSuffix(string(source), text) {
		offset = bytes.Count(source[:len(source)-len(text)], []byte{'\n'})
	}
	tmpl, err := template.New(decl.Name).Funcs(templating.Funcs).Parse(text)
	if err != nil {
		return []Diagnostic{{ID: InvalidTemplate, Level: LevelError, Path: decl.Path, Line: offset + parseLine(decl.Name, err), Column: 1, Message: err.Error()}}
	}
	if tmpl.Tree == nil {
		return nil
	}
	f := fields{decl: decl, known: known, text: text, tmpl: tmpl, visited: map[string]bool{tmpl.Name(): true}}
	f.node(tmpl.Tree.Root, true)
	for i := range f.diags {
		line, column := position([]byte(text), f.offsets[i])
		f.diags[i].Line, f.diags[i].Column = offset+line, column
	}
	return f.diags
}

func parseLine(name string, err error) int {
	rest, _ := strings.CutPrefix(err.Error(), "template: "+name+":")
	digits, _, _ := strings.Cut(rest, ":")
	if line, convErr := strconv.Atoi(digits); convErr == nil {
		return line
	}
	return 1
}

type fields struct {
	decl    Declaration
	text    string
	known   map[string]bool
	tmpl    *template.Template
	visited map[string]bool
	diags   []Diagnostic
	offsets []int
}

func (f *fields) invoke(n *parse.TemplateNode, root bool) {
	if !root || f.visited[n.Name] || !rootData(n.Pipe) {
		return
	}
	f.visited[n.Name] = true
	if t := f.tmpl.Lookup(n.Name); t != nil && t.Tree != nil {
		f.node(t.Tree.Root, true)
	}
}

func rootData(pipe *parse.PipeNode) bool {
	if pipe == nil || len(pipe.Decl) > 0 || len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return false
	}
	switch arg := pipe.Cmds[0].Args[0].(type) {
	case *parse.DotNode:
		return true
	case *parse.VariableNode:
		return len(arg.Ident) == 1 && arg.Ident[0] == "$"
	}
	return false
}

func (f *fields) declared(name string) bool {
	_, ok := f.decl.Attributes[name]
	return ok || f.known[name] || name == "body" && f.decl.Body.Kind == "text"
}

func (f *fields) check(pos parse.Pos, name string) {
	if f.declared(name) {
		return
	}
	f.diags = append(f.diags, Diagnostic{ID: UnknownField, Level: LevelError, Path: f.decl.Path, Message: fmt.Sprintf("field %s is neither an attribute of %s nor a known skill value", name, f.decl.Name)})
	f.offsets = append(f.offsets, int(pos))
}

func (f *fields) node(node parse.Node, root bool) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			f.node(child, root)
		}
	case *parse.ActionNode:
		f.node(n.Pipe, root)
	case *parse.TemplateNode:
		f.node(n.Pipe, root)
		f.invoke(n, root)
	case *parse.IfNode:
		f.node(n.Pipe, root)
		f.node(n.List, root)
		f.node(n.ElseList, root)
	case *parse.RangeNode:
		f.node(n.Pipe, root)
		f.node(n.List, false)
		f.node(n.ElseList, root)
	case *parse.WithNode:
		f.node(n.Pipe, root)
		f.node(n.List, false)
		f.node(n.ElseList, root)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, cmd := range n.Cmds {
			f.node(cmd, root)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			f.node(arg, root)
		}
	case *parse.ChainNode:
		f.node(n.Node, root)
	case *parse.FieldNode:
		if root {
			f.check(n.Pos, n.Ident[0])
		}
	case *parse.VariableNode:
		if n.Ident[0] == "$" && len(n.Ident) > 1 {
			f.check(parse.Pos(strings.LastIndexByte(f.text[:n.Pos+1], '$')), n.Ident[1])
		}
	}
}

func templateUses(decl Declaration) map[string]bool {
	uses := map[string]bool{}
	root, _ := markup.Tree([]byte(decl.Template))
	root.Walk(func(e *markup.Element) bool {
		if e.Type == "directive" {
			uses[e.Attrs["name"]] = true
			if e.Attrs["name"] == "include" && e.Attrs["template"] != "" {
				uses[e.Attrs["template"]] = true
			}
		}
		return true
	})
	return uses
}
