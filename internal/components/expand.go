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

	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/templating"
)

const markFunc = "henia_mark"

type Environment struct {
	Target    string
	Harnesses map[string]map[string]string
	Values    map[string]any
	Known     map[string]bool
	Resolve   func(from Unit, ref string) (Key, string, error)
}

type expander struct {
	decls   map[string]Declaration
	env     Environment
	done    map[*markup.Item]bool
	kept    map[*markup.Item]Declaration
	offsets map[string]int
	diags   []Diagnostic
	changed bool
	graph   *graph
	unit    *node
	sealed  map[*markup.Item]bool
}

func ExpandDocument(doc *markup.Document, decls map[string]Declaration, env Environment) []Diagnostic {
	return newExpander(decls, env).run(doc)
}

func newExpander(decls map[string]Declaration, env Environment) *expander {
	return &expander{decls: decls, env: env, done: map[*markup.Item]bool{}, kept: map[*markup.Item]Declaration{}, offsets: map[string]int{}, sealed: map[*markup.Item]bool{}}
}

func (x *expander) run(doc *markup.Document) []Diagnostic {
	for {
		x.changed = false
		doc.Children = x.items(doc.Children)
		if !x.changed {
			break
		}
	}
	x.validateKept(doc.Children)
	sortDiagnostics(x.diags)
	return x.diags
}

func sortDiagnostics(diags []Diagnostic) {
	slices.SortStableFunc(diags, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
	})
}

func (x *expander) items(items []*markup.Item) []*markup.Item {
	out := make([]*markup.Item, 0, len(items))
	for _, it := range items {
		out = append(out, x.item(it)...)
	}
	return out
}

func (x *expander) item(it *markup.Item) []*markup.Item {
	if x.sealed[it] {
		return []*markup.Item{it}
	}
	if it.Kind != "directive" || x.done[it] {
		x.descend(it)
		return []*markup.Item{it}
	}
	name := it.Attrs["name"]
	decl, ok := x.decls[name]
	if !ok {
		x.report(it, UnknownComponent, "unknown component %s", name)
		return x.keep(it)
	}
	switch {
	case name == "when" || name == "unless":
		return x.condition(it, decl)
	case name == "include":
		return x.include(it, decl)
	case decl.Path == "" || decl.Render == Keep:
		x.kept[it] = decl
		return x.keep(it)
	}
	if !x.settle(it) {
		return []*markup.Item{it}
	}
	values, ok := x.validate(it, decl, it.Attrs)
	if !ok {
		x.done[it] = true
		return []*markup.Item{it}
	}
	if name == Head {
		return x.head(it, decl, values)
	}
	out, ok := x.render(it, decl, values, it)
	if !ok {
		x.done[it] = true
		return []*markup.Item{it}
	}
	return out
}

func (x *expander) descend(it *markup.Item) {
	if it.Kind == "directive" {
		if decl, ok := x.decls[it.Attrs["name"]]; ok && decl.Body.Kind == "text" {
			return
		}
	}
	it.Children = x.items(it.Children)
}

func (x *expander) settle(it *markup.Item) bool {
	changed := x.changed
	x.changed = false
	x.descend(it)
	settled := !x.changed
	x.changed = x.changed || changed
	return settled
}

func (x *expander) validateKept(items []*markup.Item) {
	for _, it := range items {
		if decl, ok := x.kept[it]; ok {
			x.validate(it, decl, it.Attrs)
		}
		x.validateKept(it.Children)
	}
}

func (x *expander) keep(it *markup.Item) []*markup.Item {
	x.done[it] = true
	x.descend(it)
	return []*markup.Item{it}
}

func form(it *markup.Item) string {
	switch {
	case it.Attrs["inline"] == "true":
		return Text
	case it.Fenced():
		return Container
	}
	return Leaf
}

func (x *expander) validate(it *markup.Item, decl Declaration, attrs map[string]string) (map[string]any, bool) {
	before := len(x.diags)
	values := x.arguments(it, decl, attrs)
	f := form(it)
	if !decl.Allows(f) {
		x.report(it, WrongForm, "%s cannot be called in %s form; want %s", decl.Name, f, strings.Join(decl.Forms, ", "))
	} else {
		x.body(it, decl, f)
	}
	return values, len(x.diags) == before
}

func (x *expander) arguments(it *markup.Item, decl Declaration, attrs map[string]string) map[string]any {
	values := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		if name == "name" || name == "inline" && attrs[name] == "true" {
			continue
		}
		attr, ok := decl.Attributes[name]
		if !ok {
			if !decl.Open {
				x.report(it, UnknownAttribute, "%s does not declare attribute %s", decl.Name, name)
			}
			continue
		}
		value, err := attr.Coerce(attrs[name])
		if err != nil {
			x.report(it, InvalidAttribute, "%s attribute %s: %v", decl.Name, name, err)
			continue
		}
		values[name] = value
	}
	for _, name := range slices.Sorted(maps.Keys(decl.Attributes)) {
		if _, ok := attrs[name]; ok {
			continue
		}
		attr := decl.Attributes[name]
		if attr.Required {
			x.report(it, MissingAttribute, "%s requires attribute %s", decl.Name, name)
		}
		values[name] = attr.Zero()
	}
	return values
}

func (x *expander) body(it *markup.Item, decl Declaration, f string) {
	empty := len(it.Children) == 0
	if f != Container {
		empty = strings.TrimSpace(it.Text) == "" && empty
	}
	switch {
	case decl.Body.Kind == "none" && !empty:
		x.report(it, InvalidBody, "%s takes no body", decl.Name)
	case decl.Body.Required && empty:
		x.report(it, InvalidBody, "%s requires a body", decl.Name)
	}
	if decl.Body.Kind != "markdown" || len(decl.Body.Children) == 0 || f != Container {
		return
	}
	for _, child := range blockChildren(it.Children) {
		kind := child.Kind
		if kind == "directive" {
			kind = child.Attrs["name"]
		}
		if !slices.Contains(decl.Body.Children, kind) {
			x.report(child, DisallowedChild, "%s does not allow %s children; want %s", decl.Name, kind, strings.Join(decl.Body.Children, ", "))
		}
	}
}

func blockChildren(items []*markup.Item) []*markup.Item {
	var out []*markup.Item
	for _, c := range items {
		if c.Kind == "section" {
			out = append(out, blockChildren(c.Children)...)
			continue
		}
		out = append(out, c)
	}
	return out
}

func (x *expander) condition(it *markup.Item, decl Declaration) []*markup.Item {
	_, valid := x.validate(it, decl, it.Attrs)
	holds, known := x.holds(it)
	if !valid || !known {
		return x.keep(it)
	}
	x.changed = true
	if holds != (it.Attrs["name"] == "when") {
		return nil
	}
	return it.Replace(x.items(it.Children)...)
}

func (x *expander) holds(it *markup.Item) (bool, bool) {
	holds, known := true, true
	for _, name := range slices.Sorted(maps.Keys(it.Attrs)) {
		raw := it.Attrs[name]
		switch name {
		case "name":
		case "target":
			listed := false
			for _, target := range strings.Fields(raw) {
				if _, ok := x.env.Harnesses[target]; !ok {
					x.report(it, UnknownTarget, "unknown target %s; want one of %s", target, strings.Join(slices.Sorted(maps.Keys(x.env.Harnesses)), ", "))
					known = false
				}
				listed = listed || target == x.env.Target
			}
			holds = holds && listed
		default:
			if !x.variable(name) {
				x.report(it, UnknownVariable, "no harness declares variable %s", name)
				known = false
				continue
			}
			value, ok := x.env.Harnesses[x.env.Target][name]
			holds = holds && ok && value == raw
		}
	}
	return holds, known
}

func (x *expander) variable(name string) bool {
	for _, vars := range x.env.Harnesses {
		if _, ok := vars[name]; ok {
			return true
		}
	}
	return false
}

func (x *expander) head(it *markup.Item, decl Declaration, values map[string]any) []*markup.Item {
	x.done[it] = true
	doc, ok := x.output(it, decl, values, it)
	if !ok {
		return []*markup.Item{it}
	}
	x.changed = true
	it.Children = doc.Children
	return []*markup.Item{it}
}

func (x *expander) render(call *markup.Item, decl Declaration, values map[string]any, body *markup.Item) ([]*markup.Item, bool) {
	doc, ok := x.output(call, decl, values, body)
	if !ok {
		return nil, false
	}
	if form(call) == Text {
		if len(doc.Children) > 1 || len(doc.Children) == 1 && doc.Children[0].Kind != "paragraph" {
			x.report(call, WrongForm, "text component %s must produce inline Markdown", decl.Name)
			x.note(doc.Children[len(doc.Children)-1].Origin)
			return nil, false
		}
	}
	x.changed = true
	return call.Replace(doc.Children...), true
}

func (x *expander) output(call *markup.Item, decl Declaration, values map[string]any, body *markup.Item) (*markup.Document, bool) {
	if slices.ContainsFunc(call.Backtrace, func(c markup.Call) bool { return c.Component.File == decl.Path }) {
		x.report(call, ComponentCycle, "component %s reaches its own call again", decl.Name)
		return nil, false
	}
	offset, err := x.offset(decl)
	if err != nil {
		x.report(call, InvalidTemplate, "component %s: %v", decl.Name, err)
		return nil, false
	}
	data := x.data(decl, values, body)
	text, marks, err := execute(decl, data)
	if err != nil {
		x.report(call, InvalidTemplate, "component %s: %v", decl.Name, err)
		return nil, false
	}
	doc, err := markup.Parse(decl.Path, []byte(text))
	if err != nil {
		x.report(call, InvalidMarkup, "component %s output: %v", decl.Name, err)
		return nil, false
	}
	trace := append(slices.Clone(call.Backtrace), markup.Call{Directive: call.Origin})
	var stamp func([]*markup.Item)
	stamp = func(items []*markup.Item) {
		for _, it := range items {
			origin := markup.Origin{File: decl.Path, Line: offset + marks.line(decl.Template, it.Origin.Start), Column: it.Origin.Column}
			it.Origin = origin
			it.Backtrace = slices.Clone(trace)
			it.Backtrace[len(trace)-1].Component = origin
			stamp(it.Children)
		}
	}
	stamp(doc.Children)
	grafts := 0
	doc.Children = graft(doc.Children, bodyItems(call, decl, body), &grafts)
	if grafts > 1 {
		x.report(call, InvalidBody, "component %s places ::body %d times", decl.Name, grafts)
		x.note(markup.Origin{File: decl.Path, Line: offset + 1})
		return nil, false
	}
	if grafts == 0 && decl.Name != Head && body != nil && decl.Body.Kind == "markdown" && len(body.Children) > 0 {
		x.report(call, InvalidBody, "component %s does not place its body with ::body", decl.Name)
		x.note(markup.Origin{File: decl.Path, Line: offset + 1})
		return nil, false
	}
	return doc, true
}

func bodyItems(call *markup.Item, decl Declaration, body *markup.Item) []*markup.Item {
	switch {
	case body == nil || decl.Body.Kind != "markdown":
		return nil
	case call.Fenced():
		return body.Children
	case strings.TrimSpace(body.Text) != "":
		return []*markup.Item{body.Label()}
	}
	return nil
}

func graft(items, body []*markup.Item, grafts *int) []*markup.Item {
	out := make([]*markup.Item, 0, len(items))
	for _, it := range items {
		if it.Kind == "directive" && it.Attrs["name"] == "body" && form(it) == Leaf {
			*grafts++
			out = append(out, it.Replace(body...)...)
			continue
		}
		it.Children = graft(it.Children, body, grafts)
		out = append(out, it)
	}
	return out
}

func (x *expander) data(decl Declaration, values map[string]any, body *markup.Item) map[string]any {
	data := map[string]any{}
	for name := range x.env.Known {
		data[name] = nil
	}
	for _, vars := range x.env.Harnesses {
		for name := range vars {
			data[name] = nil
		}
	}
	for name, value := range x.env.Harnesses[x.env.Target] {
		data[name] = value
	}
	maps.Copy(data, x.env.Values)
	maps.Copy(data, values)
	if decl.Body.Kind == "text" {
		data["body"] = ""
		if body != nil {
			data["body"] = body.Content()
		}
	}
	return data
}

func (x *expander) offset(decl Declaration) (int, error) {
	if offset, ok := x.offsets[decl.Path]; ok {
		return offset, nil
	}
	source, err := os.ReadFile(decl.Path)
	if err != nil {
		return 0, err
	}
	if !bytes.HasSuffix(source, []byte(decl.Template)) {
		return 0, errors.New("template no longer matches its file")
	}
	offset := bytes.Count(source[:len(source)-len(decl.Template)], []byte{'\n'})
	x.offsets[decl.Path] = offset
	return offset, nil
}

type mark struct{ out, pos, size int }

type marks []mark

func (m marks) line(text string, out int) int {
	pos := 0
	for _, k := range m {
		if k.out > out {
			break
		}
		pos = k.pos + min(out-k.out, k.size)
	}
	return strings.Count(text[:min(pos, len(text))], "\n") + 1
}

func execute(decl Declaration, data map[string]any) (string, marks, error) {
	var buf bytes.Buffer
	var found marks
	record := func(pos, size int) string {
		found = append(found, mark{out: buf.Len(), pos: pos, size: size})
		return ""
	}
	tmpl, err := template.New(decl.Name).
		Funcs(templating.Funcs).
		Funcs(template.FuncMap{markFunc: record}).
		Option("missingkey=error").
		Parse(decl.Template)
	if err != nil {
		return "", nil, err
	}
	templating.BlankAbsent(tmpl)
	for _, t := range tmpl.Templates() {
		if t.Tree != nil {
			markText(t.Tree.Root)
		}
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", nil, err
	}
	return buf.String(), found, nil
}

func markText(node parse.Node) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		nodes := make([]parse.Node, 0, 2*len(n.Nodes))
		for _, child := range n.Nodes {
			if text, ok := child.(*parse.TextNode); ok {
				nodes = append(nodes, markAction(text))
			}
			markText(child)
			nodes = append(nodes, child)
		}
		n.Nodes = nodes
	case *parse.IfNode:
		markText(n.List)
		markText(n.ElseList)
	case *parse.RangeNode:
		markText(n.List)
		markText(n.ElseList)
	case *parse.WithNode:
		markText(n.List)
		markText(n.ElseList)
	}
}

func markAction(text *parse.TextNode) *parse.ActionNode {
	number := func(v int) *parse.NumberNode {
		return &parse.NumberNode{NodeType: parse.NodeNumber, Pos: text.Pos, IsInt: true, Int64: int64(v), Text: strconv.Itoa(v)}
	}
	args := []parse.Node{parse.NewIdentifier(markFunc).SetPos(text.Pos), number(int(text.Pos)), number(len(text.Text))}
	command := &parse.CommandNode{NodeType: parse.NodeCommand, Pos: text.Pos, Args: args}
	pipe := &parse.PipeNode{NodeType: parse.NodePipe, Pos: text.Pos, Cmds: []*parse.CommandNode{command}}
	return &parse.ActionNode{NodeType: parse.NodeAction, Pos: text.Pos, Pipe: pipe}
}

func (x *expander) report(it *markup.Item, id, format string, args ...any) {
	site := callSite(it)
	var notes []string
	for _, c := range it.Backtrace {
		notes = append(notes, fmt.Sprintf("expanded from component %s:%d", c.Component.File, c.Component.Line))
	}
	x.diags = append(x.diags, Diagnostic{ID: id, Level: LevelError, Path: site.File, Line: site.Line, Column: site.Column, Message: fmt.Sprintf(format, args...), Notes: notes})
}

func (x *expander) note(component markup.Origin) {
	d := &x.diags[len(x.diags)-1]
	d.Notes = append(d.Notes, fmt.Sprintf("produced by component %s:%d", component.File, component.Line))
}

func callSite(it *markup.Item) markup.Origin {
	if len(it.Backtrace) > 0 {
		return it.Backtrace[0].Directive
	}
	return it.Origin
}
