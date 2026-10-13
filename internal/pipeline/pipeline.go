package pipeline

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/components"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/transform"
)

type Dependency struct {
	Name      string
	Root      string
	Harnesses map[string]henia.Harness
	// Project marks the building project's own package rendered under a
	// dependency's harness; its artifacts keep the project's strictness.
	Project bool
}

func Layered(harness henia.Harness, name, path string, dependencies []Dependency) henia.Harness {
	for _, d := range dependencies {
		if !strings.HasPrefix(path, d.Root+string(filepath.Separator)) {
			continue
		}
		own, ok := d.Harnesses[name]
		if d.Project {
			harness.Strict = own.Strict
		}
		if !ok {
			continue
		}
		for _, field := range []struct{ project, dependency *map[string]string }{
			{&harness.Variables, &own.Variables}, {&harness.Tools, &own.Tools}, {&harness.References, &own.References},
		} {
			merged := maps.Clone(*field.dependency)
			if merged == nil {
				merged = map[string]string{}
			}
			maps.Copy(merged, *field.project)
			*field.project = merged
		}
	}
	return harness
}

func Root(project, path string, dependencies []Dependency) string {
	for _, d := range dependencies {
		if strings.HasPrefix(path, d.Root+string(filepath.Separator)) {
			return d.Root
		}
	}
	return project
}

func heniaSettings(frontmatter map[string]any) map[string]any {
	settings, _ := frontmatter["henia"].(map[string]any)
	return settings
}

type Unit struct {
	Artifact    *artifact.Artifact
	Transformer *transform.Transformer
	Package     string
	Resource    string
	Root, User  string
	Known       map[string]bool
	Variables   map[string]string
	Err         error

	result *artifact.Artifact
	values map[string]any
	doc    *markup.Document
	path   string
	source []byte
	offset int
	err    error
	diags  []components.Diagnostic
	calls  []string
	edges  []edge
	done   bool
	failed Diagnostics
}

type edge struct {
	to   *Unit
	call markup.Origin
}

type Target struct {
	Name      string
	Harnesses map[string]map[string]string
	Resolve   func(from components.Unit, ref string) (components.Key, string, error)
	Load      func(components.Key) *Unit
	Expanded  func(*Unit, *markup.Document)
}

type componentSet struct {
	decls    map[string]components.Declaration
	problems []components.Diagnostic
	known    map[string]bool
	docs     []components.Document
	checked  bool
}

func loadComponents(root, user string) *componentSet {
	decls, problems := components.Load(root, user)
	set := &componentSet{decls: decls, known: map[string]bool{}}
	for _, p := range problems {
		set.problems = append(set.problems, components.Diagnostic{ID: p.ID, Level: components.LevelError, Path: p.Path, Message: p.Message})
	}
	return set
}

type expansion struct {
	sets   map[[2]string]*componentSet
	byKey  map[components.Key]*Unit
	byPath map[string]*Unit
	decl   map[string][]components.Diagnostic
	units  []*Unit
	target Target
}

func Expand(units []*Unit, t Target) {
	e := &expansion{sets: map[[2]string]*componentSet{}, byKey: map[components.Key]*Unit{}, byPath: map[string]*Unit{}, decl: map[string][]components.Diagnostic{}, target: t}
	expandable := make([]components.Unit, 0, len(units))
	for _, u := range units {
		expandable = append(expandable, e.add(u))
	}
	for _, set := range e.sets {
		e.check(set, set.docs)
	}
	env := components.Environment{
		Target: t.Name, Harnesses: t.Harnesses, Resolve: t.Resolve,
		Load: func(k components.Key) (components.Unit, bool) {
			var u *Unit
			switch {
			case k.Resource != "":
				u = e.resource(k)
			case t.Load != nil:
				u = t.Load(k)
			}
			if u == nil {
				return components.Unit{}, false
			}
			loaded := e.add(u)
			if u.doc != nil {
				e.check(e.sets[[2]string{u.Root, u.User}], []components.Document{{Path: u.path, Source: u.source}})
			}
			return loaded, true
		},
		Included: func(from, to components.Key, call markup.Origin) {
			if u, dep := e.byKey[from], e.byKey[to]; u != nil && dep != nil && dep != u {
				u.edges = append(u.edges, edge{dep, call})
			}
		},
		Called: func(from components.Key, decl components.Declaration) {
			if u := e.byKey[from]; u != nil && !slices.Contains(u.calls, decl.Path) {
				u.calls = append(u.calls, decl.Path)
			}
		},
	}
	for _, d := range components.ExpandDocuments(expandable, env) {
		if u := e.byPath[d.Path]; u != nil {
			d.Line += u.offset
			u.report(d)
			continue
		}
		e.decl[d.Path] = append(e.decl[d.Path], d)
	}
	for _, u := range e.units {
		e.failures(u, map[*Unit]bool{})
	}
}

func (e *expansion) add(u *Unit) components.Unit {
	e.units = append(e.units, u)
	art := u.Artifact
	key := components.Key{Package: u.Package, Name: UnitName(art), Resource: u.Resource}
	e.byKey[key] = u
	u.path = art.SourcePath
	if art.IsDirectory {
		u.path = filepath.Join(u.path, artifact.MainFileName(art.Type))
	}
	if u.err = u.Err; u.err == nil {
		u.prepare()
	}
	if u.err != nil {
		return components.Unit{Name: key.Name, Package: key.Package}
	}
	set := e.sets[[2]string{u.Root, u.User}]
	if set == nil {
		set = loadComponents(u.Root, u.User)
		e.sets[[2]string{u.Root, u.User}] = set
	}
	maps.Copy(set.known, u.Known)
	u.diags = append(u.diags, set.problems...)
	set.docs = append(set.docs, components.Document{Path: u.path, Source: u.source})
	e.byPath[u.path] = u
	values := maps.Clone(u.values)
	for name := range u.Known {
		if _, ok := values[name]; !ok {
			values[name] = nil
		}
	}
	return components.Unit{
		Name: key.Name, Package: key.Package, Resource: key.Resource, Path: u.path, Offset: u.offset, Document: u.doc, Components: set.decls, Values: values, Variables: u.Variables,
		Finish: func(doc *markup.Document) []components.Diagnostic {
			if e.target.Expanded != nil {
				e.target.Expanded(u, doc)
			}
			transform.Listings(doc, art.Name, art.Type == artifact.TypeSkill)
			return nil
		},
	}
}

func (e *expansion) resource(k components.Key) *Unit {
	owner := e.byKey[components.Key{Package: k.Package, Name: k.Name}]
	if owner == nil && e.target.Load != nil {
		owner = e.target.Load(components.Key{Package: k.Package, Name: k.Name})
	}
	if owner == nil || owner.Artifact == nil {
		return nil
	}
	dir := owner.Artifact.SourcePath
	if !owner.Artifact.IsDirectory {
		dir = filepath.Dir(dir)
	}
	path, err := library.AddressedResource(dir, k.Resource)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	art := *owner.Artifact
	art.SourcePath, art.IsDirectory, art.Body, art.Resources = path, false, string(data), nil
	return &Unit{
		Artifact: &art, Transformer: owner.Transformer, Package: owner.Package, Resource: k.Resource,
		Root: owner.Root, User: owner.User, Known: owner.Known, Variables: owner.Variables, Err: owner.Err,
	}
}

func (e *expansion) check(set *componentSet, docs []components.Document) {
	for _, d := range components.Check(docs, set.decls, set.known) {
		if d.Level != components.LevelError {
			continue
		}
		if u := e.byPath[d.Path]; u != nil {
			u.report(d)
			continue
		}
		if !set.checked {
			e.decl[d.Path] = append(e.decl[d.Path], d)
		}
	}
	set.checked = true
}

func (u *Unit) report(d components.Diagnostic) {
	if !slices.ContainsFunc(u.diags, func(c components.Diagnostic) bool {
		return c.ID == d.ID && c.Path == d.Path && c.Line == d.Line && c.Column == d.Column && c.Message == d.Message
	}) {
		u.diags = append(u.diags, d)
	}
}

func (e *expansion) failures(u *Unit, visiting map[*Unit]bool) Diagnostics {
	if u.done || visiting[u] {
		return u.failed
	}
	visiting[u] = true
	failed := slices.Clone(u.diags)
	if u.err != nil {
		failed = append(failed, components.Diagnostic{Level: components.LevelError, Path: u.path, Message: u.err.Error()})
	}
	for _, path := range u.calls {
		failed = append(failed, e.decl[path]...)
	}
	for _, dep := range u.edges {
		line := dep.call.Line
		if dep.call.File == u.path {
			line += u.offset
		}
		note := fmt.Sprintf("included from %s:%d:%d", dep.call.File, line, dep.call.Column)
		for _, d := range e.failures(dep.to, visiting) {
			if !slices.Contains(d.Notes, note) {
				d.Notes = append(slices.Clone(d.Notes), note)
			}
			failed = append(failed, d)
		}
	}
	slices.SortStableFunc(failed, func(a, b components.Diagnostic) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
	})
	u.failed, u.done = failed, true
	delete(visiting, u)
	return failed
}

func (u *Unit) prepare() {
	if u.result, u.values, u.err = u.Transformer.Prepare(u.Artifact); u.err != nil {
		return
	}
	body := []byte(u.Artifact.Body)
	u.source = body
	if data, err := os.ReadFile(u.path); err == nil && bytes.HasSuffix(data, body) {
		u.source, u.offset = data, bytes.Count(data[:len(data)-len(body)], []byte{'\n'})
	}
	data := map[string]any{}
	for name := range u.Known {
		data[name] = nil
	}
	for name, value := range u.Variables {
		data[name] = value
	}
	maps.Copy(data, u.values)
	if templated, err := components.TemplateBody(u.path, string(body), data); err != nil {
		u.diags = append(u.diags, components.Diagnostic{ID: components.InvalidTemplate, Level: components.LevelError, Path: u.path, Line: u.offset + 1, Column: 1, Message: err.Error()})
	} else {
		body = []byte(templated)
	}
	u.doc, _ = markup.Parse(u.path, body)
}

func (u *Unit) Path() string { return u.path }

func (u *Unit) Offset() int { return u.offset }

func (u *Unit) Document() *markup.Document { return u.doc }

func (u *Unit) Failures() Diagnostics { return u.failed }

func (u *Unit) Render() (*artifact.Artifact, error) {
	switch {
	case u.err != nil:
		return nil, u.err
	case len(u.failed) > 0:
		return nil, u.failed
	}
	body, err := u.Transformer.RenderDocument(u.doc, u.Artifact)
	if err != nil {
		return nil, err
	}
	u.result.Body = body
	return u.result, nil
}

func UnitName(art *artifact.Artifact) string {
	if art.Type == artifact.TypeSkill {
		return art.Name
	}
	return string(art.Type) + "/" + art.Name
}

type Diagnostics []components.Diagnostic

func (ds Diagnostics) Error() string {
	var b strings.Builder
	for i, d := range ds {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.Path)
		if d.Line > 0 {
			fmt.Fprintf(&b, ":%d:%d", d.Line, d.Column)
		}
		fmt.Fprintf(&b, ": %s ", d.Level)
		if d.ID != "" {
			fmt.Fprintf(&b, "[%s] ", d.ID)
		}
		b.WriteString(d.Message)
		for _, note := range d.Notes {
			fmt.Fprintf(&b, "\n  note: %s", note)
		}
	}
	return b.String()
}

func Known(arts []*artifact.Artifact, harnesses map[string]map[string]string) map[string]bool {
	known := map[string]bool{}
	for _, art := range arts {
		if art.Type != artifact.TypeSkill {
			continue
		}
		for name := range art.Frontmatter {
			known[name] = true
		}
		if vars, ok := heniaSettings(art.Frontmatter)["variables"].(map[string]any); ok {
			for name := range vars {
				known[name] = true
			}
		}
	}
	for _, vars := range harnesses {
		for name := range vars {
			known[name] = true
		}
	}
	return known
}

func Targets(harnesses map[string]henia.Harness, dependencies []Dependency) (map[string]map[string]string, error) {
	builtin, err := defaults.DefaultConfig()
	if err != nil {
		return nil, err
	}
	layers := []map[string]henia.Harness{builtin.Harness}
	for _, d := range dependencies {
		layers = append(layers, d.Harnesses)
	}
	targets := map[string]map[string]string{}
	for _, layer := range append(layers, harnesses) {
		for name, h := range layer {
			if targets[name] == nil {
				targets[name] = map[string]string{}
			}
			maps.Copy(targets[name], h.Variables)
		}
	}
	return targets, nil
}

func PackageArtifacts(all []*artifact.Artifact, root, project string, dependencies []Dependency) []*artifact.Artifact {
	var arts []*artifact.Artifact
	for _, art := range all {
		if Root(project, art.SourcePath, dependencies) == root {
			arts = append(arts, art)
		}
	}
	return arts
}

func PackageName(root string, dependencies []Dependency) string {
	for _, d := range dependencies {
		if d.Root == root && d.Name != "" {
			return d.Name
		}
		if d.Root == root {
			return filepath.Base(root)
		}
	}
	return library.Project
}
