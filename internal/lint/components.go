package lint

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/components"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pipeline"
	"github.com/srnnkls/henia/internal/slots"
)

const (
	StageSource   = "source"
	StageExpanded = "expanded"
	StageRendered = "rendered"
)

var stages = []string{StageSource, StageExpanded, StageRendered}

var supersededRules = []string{components.InvalidMarkup, components.InvalidTemplate}

var checkIDs = []string{
	components.UnknownComponent, components.MissingAttribute, components.UnknownAttribute, components.InvalidAttribute,
	components.InvalidBody, components.DisallowedChild, components.WrongForm, components.TemplateAction,
	components.InvalidTemplate, components.UnknownField, components.UnusedComponent, components.InvalidMarkup,
	components.MissingInclude, components.InvalidInclude, components.UnknownTarget, components.UnknownVariable,
	components.ComponentCycle, components.IncludeCycle, components.InvalidComponent, components.RemovedComposition,
	"duplicate-attribute", "unclosed-directive",
}

type unit struct {
	path   string
	source []byte
	offset int
	art    *artifact.Artifact
	pkg    *componentPackage
	views  map[string]*view
}

type componentResult struct {
	diagnostics []Diagnostic
	units       map[string]*unit
	harnesses   []string
}

type componentPackage struct {
	root, user string
	decls      map[string]components.Declaration
	problems   []components.Problem
	known      map[string]bool
	arts       []*artifact.Artifact
	units      []*unit
}

func (p *Plan) componentDiagnostics(files []string) (componentResult, error) {
	if p.options.Project == "" {
		return componentResult{}, nil
	}
	targets, err := pipeline.Targets(p.options.Configured, p.options.Packages)
	if err != nil {
		return componentResult{}, err
	}
	built, err := builtHarnesses(p.options.Configured)
	if err != nil {
		return componentResult{}, err
	}
	packages, err := p.packages(files, targets)
	if err != nil {
		return componentResult{}, err
	}
	result := componentResult{units: map[string]*unit{}, harnesses: slices.Sorted(maps.Keys(built))}
	declared := declaredSlots(packages)
	var out []Diagnostic
	for _, pkg := range packages {
		if len(pkg.units) == 0 {
			continue
		}
		var docs []components.Document
		for _, u := range pkg.units {
			result.units[u.path] = u
			docs = append(docs, components.Document{Path: u.path, Source: u.source})
		}
		for _, problem := range pkg.problems {
			out = append(out, Diagnostic{Path: problem.Path, Severity: components.LevelError, Rule: problem.ID, Message: problem.Message, Stage: StageSource})
		}
		for _, d := range components.Check(docs, pkg.decls, pkg.known) {
			if d.ID == components.UnusedComponent && pkg.user != "" && within(pkg.user, d.Path) {
				continue
			}
			out = append(out, diagnostic(d, StageSource))
		}
		for _, u := range pkg.units {
			out = append(out, slotAttributes(u, declared)...)
		}
	}
	seen := map[string]bool{}
	for _, d := range out {
		seen[d.key()] = true
	}
	var expanded perHarness
	for _, name := range result.harnesses {
		var found []Diagnostic
		for _, d := range p.expand(packages, name, built[name], targets) {
			if d := diagnostic(d, StageExpanded); !seen[d.key()] {
				found = append(found, d)
			}
		}
		expanded.add(name, found)
	}
	out = append(out, expanded.diagnostics(len(result.harnesses))...)
	for _, d := range out {
		if p.disabled[d.Rule] {
			continue
		}
		if u := result.units[d.Path]; u != nil && slices.Contains(disabledRules(u.art), d.Rule) {
			continue
		}
		result.diagnostics = append(result.diagnostics, d)
	}
	return result, nil
}

func (r componentResult) checked(path string) bool { return r.units[path] != nil }

func (d Diagnostic) key() string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", d.Rule, d.Path, d.Line, d.Column, d.Message)
}

func diagnostic(d components.Diagnostic, stage string) Diagnostic {
	return Diagnostic{Path: d.Path, Line: d.Line, Column: d.Column, Severity: d.Level, Rule: d.ID, Message: d.Message, Stage: stage, Notes: d.Notes}
}

var artifactDirs = []string{"skills", "commands", "agents"}

func (p *Plan) packages(files []string, targets map[string]map[string]string) ([]*componentPackage, error) {
	roots := []string{p.options.Project}
	for _, d := range p.options.Packages {
		roots = append(roots, d.Root)
	}
	canonical := make([]string, len(roots))
	for i, root := range roots {
		var err error
		if canonical[i], err = canonicalPath(root); err != nil {
			return nil, err
		}
	}
	packages := make([]*componentPackage, len(roots))
	discovered := map[string]*artifact.Artifact{}
	for i, root := range roots {
		pkg := &componentPackage{root: root}
		if i == 0 {
			pkg.user = p.options.User
		}
		pkg.decls, pkg.problems = components.Load(root, pkg.user)
		pkg.arts, _ = artifact.Discover(root, artifactDirs)
		for _, art := range pkg.arts {
			if path, err := canonicalPath(mainPath(art)); err == nil {
				discovered[path] = art
			}
		}
		packages[i] = pkg
	}
	owner := func(path string) *componentPackage {
		best := -1
		for i, root := range canonical {
			if within(root, path) && (best < 0 || len(root) > len(canonical[best])) {
				best = i
			}
		}
		if best < 0 {
			return nil
		}
		return packages[best]
	}
	for _, path := range files {
		kind := artifactKind(path)
		if kind == artifact.TypeUnknown {
			continue
		}
		absolute, err := canonicalPath(path)
		if err != nil {
			return nil, err
		}
		pkg := owner(absolute)
		if pkg == nil {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		parsed, err := artifact.Parse(source)
		if err != nil {
			continue
		}
		art := discovered[absolute]
		if art == nil {
			art = parsed
			art.Type, art.IsDirectory = kind, filepath.Base(path) == artifact.MainFileName(kind)
			art.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if art.IsDirectory {
				art.Name = filepath.Base(filepath.Dir(path))
			}
			pkg.arts = append(pkg.arts, art)
		}
		art.SourcePath = path
		if art.IsDirectory {
			art.SourcePath = filepath.Dir(path)
		}
		u := &unit{path: path, source: source, art: parsed, pkg: pkg, views: map[string]*view{}}
		u.offset = bytes.Count(source[:len(source)-len(parsed.Body)], []byte{'\n'})
		pkg.units = append(pkg.units, u)
	}
	for _, pkg := range packages {
		pkg.known = pipeline.Known(pkg.arts, targets)
	}
	return packages, nil
}

func mainPath(art *artifact.Artifact) string {
	if art.IsDirectory {
		return filepath.Join(art.SourcePath, artifact.MainFileName(art.Type))
	}
	return art.SourcePath
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved, nil
	}
	return absolute, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func disabledRules(art *artifact.Artifact) []string {
	h, _ := art.Frontmatter["henia"].(map[string]any)
	settings, _ := h["lint"].(map[string]any)
	var rules []string
	switch disable := settings["disable"].(type) {
	case string:
		rules = append(rules, disable)
	case []any:
		for _, rule := range disable {
			if id, ok := rule.(string); ok {
				rules = append(rules, id)
			}
		}
	}
	return rules
}

func (p *Plan) expand(packages []*componentPackage, target string, configured henia.Harness, targets map[string]map[string]string) []components.Diagnostic {
	var all []*pipeline.Unit
	var arts []*artifact.Artifact
	for _, pkg := range packages {
		arts = append(arts, pkg.arts...)
	}
	served := pipeline.Served(arts, pipeline.Projected(arts, configured))
	scanned := map[*pipeline.Unit]*unit{}
	for _, pkg := range packages {
		byPath := map[string]*unit{}
		for _, u := range pkg.units {
			byPath[u.path] = u
		}
		for _, art := range pkg.arts {
			absolute, _ := filepath.Abs(art.SourcePath)
			h := pipeline.Layered(configured, target, absolute, p.options.Packages)
			h.ProjectRoot, h.UserRoot = pkg.root, p.options.User
			pu := &pipeline.Unit{
				Artifact: art, Package: pipeline.PackageName(pkg.root, p.options.Packages), Root: pkg.root, User: pkg.user,
				Known: pkg.known, Variables: h.Variables,
			}
			pu.Transformer, pu.Err = pipeline.Renderer(art, target, h, served, false)
			if pu.Err == nil {
				pu.Transformer.AuthoredLinks = true
			}
			if u := byPath[mainPath(art)]; u != nil {
				scanned[pu] = u
				u.view(target).syntax = pipeline.ReferenceSyntax(h.References["skill"])
			}
			all = append(all, pu)
		}
	}
	pipeline.Expand(all, pipeline.Target{
		Name: target, Harnesses: targets,
		Expanded: func(pu *pipeline.Unit, doc *markup.Document) {
			if u := scanned[pu]; u != nil {
				u.view(target).observeSlots(markup.Write(doc))
			}
		},
	})
	var out []components.Diagnostic
	for _, pu := range all {
		u := scanned[pu]
		if u == nil {
			continue
		}
		for _, d := range pu.Failures() {
			if d.ID != "" {
				out = append(out, d)
			}
		}
		if doc := pu.Document(); doc != nil {
			v := u.view(target)
			v.expanded = expandedTree(doc, markup.Write(doc), pu.Path(), pu.Offset())
			v.render(u, pu, configured.Mode("skills", pu.Artifact.Name) == henia.Hybrid && pu.Artifact.Type == artifact.TypeSkill)
		}
	}
	return out
}

type origin struct {
	written, line, column int
	generated             bool
}

func expandedTree(doc *markup.Document, written []byte, path string, offset int) *markup.Element {
	tree, _ := markup.Tree(written)
	if tree == nil {
		return nil
	}
	relocate(tree, origins(doc, written, path, offset), offset)
	return tree
}

func origins(doc *markup.Document, written []byte, path string, offset int) []origin {
	reparsed, _ := markup.Parse(path, written)
	if reparsed == nil {
		return nil
	}
	items := flatten(doc.Children)
	var out []origin
	next := 0
	var visit func(written []*markup.Item)
	visit = func(written []*markup.Item) {
		for _, it := range written {
			i := counterpart(items[next:], it)
			if i < 0 {
				visit(it.Children)
				continue
			}
			source := items[next+i]
			next += i + 1
			at, generated := source.Origin, len(source.Backtrace) > 0
			if generated {
				at = source.Backtrace[0].Directive
			}
			if at.File == path {
				out = append(out, origin{it.Origin.Line, offset + at.Line, at.Column, generated})
			}
			if source.Kind != "text" {
				visit(it.Children)
			}
		}
	}
	visit(reparsed.Children)
	slices.SortStableFunc(out, func(a, b origin) int { return a.written - b.written })
	return out
}

func counterpart(items []*markup.Item, written *markup.Item) int {
	return slices.IndexFunc(items, func(it *markup.Item) bool {
		return it.Kind == written.Kind && it.Text == written.Text || it.Kind == "text" && strings.Contains(it.Text, written.Text)
	})
}

func relocate(e *markup.Element, origins []origin, offset int) {
	e.Walk(func(e *markup.Element) bool {
		i, _ := slices.BinarySearchFunc(origins, e.Line+1, func(o origin, line int) int { return o.written - line })
		if i == 0 {
			e.Line, e.EndLine = e.Line+offset, e.EndLine+offset
			return true
		}
		o := origins[i-1]
		if o.generated {
			e.Line, e.EndLine, e.Column = o.line, o.line, o.column
			return true
		}
		e.Line, e.EndLine = o.line+e.Line-o.written, o.line+e.EndLine-o.written
		return true
	})
}

func flatten(items []*markup.Item) []*markup.Item {
	var out []*markup.Item
	for _, it := range items {
		out = append(out, it)
		out = append(out, flatten(it.Children)...)
	}
	return out
}

func declaredSlots(packages []*componentPackage) *slots.Resolution {
	var skills []slots.Skill
	for _, pkg := range packages {
		for _, art := range pkg.arts {
			skills = append(skills, slots.Skill{Path: mainPath(art), Metadata: slots.Read(art.Frontmatter, art.Body, slots.Normal)})
		}
	}
	return slots.Evaluate(skills, nil)
}

func declares(r *slots.Resolution, slot string) bool {
	if len(r.Owners) == 0 {
		return false
	}
	owner, ok := r.Owner(slot)
	return ok && (owner.Slot == slot || owner.Type.Keyed)
}

func slotAttributes(u *unit, declared *slots.Resolution) []Diagnostic {
	tree, _ := markup.Tree([]byte(u.art.Body))
	if tree == nil {
		return nil
	}
	var out []Diagnostic
	tree.Walk(func(e *markup.Element) bool {
		if e.Type != "directive" {
			return true
		}
		decl, ok := u.pkg.decls[e.Attrs["name"]]
		if !ok {
			return true
		}
		callee := decl
		if decl.Name == "include" {
			callee, ok = u.pkg.decls[e.Attrs["template"]]
		}
		for _, name := range slices.Sorted(maps.Keys(e.Attrs)) {
			attr, typed := callee.Attributes[name]
			if !ok || !typed || name == "name" || (attr.Type != "slot" && (attr.Type != "list" || attr.Of != "slot")) {
				continue
			}
			for _, slot := range strings.Fields(e.Attrs[name]) {
				if !declares(declared, slot) {
					out = append(out, Diagnostic{
						Path: u.path, Line: u.offset + e.Line, Column: e.Column, Severity: components.LevelError,
						Rule: components.InvalidAttribute, Stage: StageSource,
						Message: fmt.Sprintf("%s attribute %s: slot %s is not declared by any package skill", callee.Name, name, slot),
					})
				}
			}
		}
		return decl.Body.Kind != "text"
	})
	return out
}

func validStage(stage string) bool { return slices.Contains(stages, stage) }
