// Package build compiles local canonical artifacts into a separate output tree.
package build

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/components"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/profile"
	"github.com/srnnkls/henia/internal/target"
	"github.com/srnnkls/henia/internal/templates"
	"github.com/srnnkls/henia/internal/transform"
)

type writeJob struct {
	target   target.Target
	artifact *artifact.Artifact
	harness  henia.Harness
}

type Result struct {
	Built    int
	Errors   []error
	Warnings []string
	Dynamic  map[string][]Entry
}

// Run compiles local source directories. Each harness writes beneath output/name.
type Dependency struct {
	Root      string
	Harnesses map[string]henia.Harness
}

func Run(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness, dependencies ...Dependency) (*Result, error) {
	return RunSelected(ctx, sources, output, harnesses, harnesses, false, dependencies...)
}

func RunArtifact(ctx context.Context, art *artifact.Artifact, source, output string, harnesses map[string]henia.Harness) (*Result, error) {
	return run(ctx, []string{source}, output, harnesses, harnesses, false, nil, []*artifact.Artifact{art})
}

// RunClean publishes a complete output tree only after every artifact was written successfully.
func RunClean(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness, dependencies ...Dependency) (*Result, error) {
	return RunSelected(ctx, sources, output, harnesses, harnesses, true, dependencies...)
}

// RunSelected writes the harnesses; conditions name every configured harness.
func RunSelected(ctx context.Context, sources []string, output string, harnesses, configured map[string]henia.Harness, clean bool, dependencies ...Dependency) (*Result, error) {
	if clean {
		if err := validateCleanOutput(sources, output); err != nil {
			return nil, err
		}
	}
	return run(ctx, sources, output, harnesses, configured, clean, dependencies, nil)
}

func layered(harness henia.Harness, name, path string, dependencies []Dependency) henia.Harness {
	for _, d := range dependencies {
		own, ok := d.Harnesses[name]
		if !ok || !strings.HasPrefix(path, d.Root+string(filepath.Separator)) {
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

func templateRoot(project, path string, dependencies []Dependency) string {
	for _, d := range dependencies {
		if strings.HasPrefix(path, d.Root+string(filepath.Separator)) {
			return d.Root
		}
	}
	return project
}

func run(ctx context.Context, sources []string, output string, harnesses, configured map[string]henia.Harness, clean bool, dependencies []Dependency, extra []*artifact.Artifact) (*Result, error) {
	if output == "" {
		return nil, fmt.Errorf("build output directory is required")
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("at least one source directory is required")
	}
	for name := range harnesses {
		if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." {
			return nil, fmt.Errorf("invalid harness name %q", name)
		}
	}
	result := &Result{Dynamic: map[string][]Entry{}}
	documents := 0
	documentError := func(err error) {
		result.Errors = append(result.Errors, err)
		documents++
	}

	var allArtifacts []*artifact.Artifact
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		arts, err := artifact.Discover(src, []string{"skills", "commands", "agents"})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("discover %s: %w", src, err))
			continue
		}
		allArtifacts = append(allArtifacts, arts...)
	}
	allArtifacts = append(allArtifacts, extra...)

	protected := slices.Clone(allArtifacts)
	for _, harness := range harnesses {
		for _, file := range harness.Files {
			for _, source := range sources {
				protected = append(protected, &artifact.Artifact{SourcePath: filepath.Join(source, file.Source)})
			}
		}
	}
	if clean {
		var inputs []string
		for _, input := range protected {
			inputs = append(inputs, input.SourcePath)
		}
		if err := validateCleanOutput(inputs, output); err != nil {
			return nil, err
		}
	}
	targets, err := Targets(configured, dependencies)
	if err != nil {
		return nil, err
	}
	var jobs []writeJob
	destinations := make(map[string]string)
	for _, harnessName := range slices.Sorted(maps.Keys(harnesses)) {
		harness := harnesses[harnessName]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outputPath := filepath.Join(output, harnessName)
		filtered := filterArtifacts(allArtifacts, harness)
		served := Served(allArtifacts, filtered)
		if entries, err := dynamicEntries(allArtifacts, filtered); err == nil {
			result.Dynamic[harnessName] = entries
		}
		if index, err := catalog(sources[0], allArtifacts, filtered, harness); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("harness %s: %w", harnessName, err))
			continue
		} else if index != nil {
			filtered = append(filtered, index)
		}

		references := &transform.Transformer{Tools: harness.Tools, References: convertReferences(harness.References), Served: served}
		files, err := supportFiles(sources, harness.Files, references)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("harness %s: %w", harnessName, err))
			continue
		}
		if len(files) > 0 {
			filtered = append(filtered, &artifact.Artifact{Name: "support files", SourcePath: sources[0], Files: files})
		}
		type pending struct {
			art, transformed *artifact.Artifact
			effective        henia.Harness
			tgt              target.Target
			hybrid           bool
			unit             *Unit
		}
		var batch []pending
		var units []*Unit
		known := map[string]map[string]bool{}
		for _, art := range allArtifacts {
			if art.Type != artifact.TypeUnknown && !slices.Contains(filtered, art) {
				units = append(units, unitFor(art, harnessName, outputPath, layered(harness, harnessName, art.SourcePath, dependencies), served, allArtifacts, targets, known, dependencies))
			}
		}
		for _, art := range filtered {
			h := layered(harness, harnessName, art.SourcePath, dependencies)
			root := templateRoot(h.ProjectRoot, art.SourcePath, dependencies)
			effective, tr, err := transformerFor(harnessName, outputPath, h, art.Type)
			if err == nil {
				tr.Served = served
				if tr.Templates, err = templates.Load(root, effective.UserRoot); err != nil {
					err = fmt.Errorf("%s: templates for %s: %w", art.SourcePath, harnessName, err)
				}
			}
			if err != nil {
				documentError(err)
				units = append(units, &Unit{Artifact: art, Package: packageName(root, dependencies), Err: err})
				continue
			}
			hybrid := art.Type == artifact.TypeSkill && art.Name != CatalogName && harness.Mode("skills", art.Name) == henia.Hybrid
			tr.Head, tr.LibraryLinks = hybrid, hybrid
			p := pending{art: art, transformed: art, effective: effective, tgt: target.NewFromConfig(harnessName, outputPath, effective), hybrid: hybrid}
			switch {
			case art.Type == artifact.TypeUnknown:
			case Legacy(art, tr.Templates):
				if p.transformed, err = tr.Transform(art); err != nil {
					documentError(fmt.Errorf("%s: transform %s for %s: %w", art.SourcePath, art.Name, harnessName, err))
					continue
				}
			default:
				p.unit = pipelineUnit(art, tr, effective, root, allArtifacts, targets, known, dependencies)
				units = append(units, p.unit)
			}
			batch = append(batch, p)
		}
		Expand(units, Target{Name: harnessName, Harnesses: targets})
		for _, p := range batch {
			art, effective, tgt, hybrid, transformed := p.art, p.effective, p.tgt, p.hybrid, p.transformed
			if p.unit != nil {
				var err error
				if transformed, err = p.unit.Render(); err != nil {
					documentError(fmt.Errorf("%s: transform %s for %s: %w", art.SourcePath, art.Name, harnessName, err))
					continue
				}
			}
			if hybrid {
				hybridHead(transformed, art, nativePreloads(effective))
			}
			if art.Type == artifact.TypeSkill || art.Type == artifact.TypeCommand {
				projectPreloads(transformed, effective)
			}
			for _, warning := range transformed.Warnings {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s (%s): %s", art.SourcePath, harnessName, warning))
			}
			destination, err := filepath.Abs(tgt.TargetPath(transformed))
			if err != nil {
				return nil, err
			}
			source := art.SourcePath
			if art.IsDirectory {
				source = filepath.Join(source, artifact.MainFileName(art.Type))
			}
			source, err = filepath.Abs(source)
			if err != nil {
				return nil, err
			}
			if destination == source {
				result.Errors = append(result.Errors, fmt.Errorf("output would overwrite canonical source %s", source))
				continue
			}
			if art.IsDirectory {
				relative, err := filepath.Rel(filepath.Dir(source), destination)
				if err != nil {
					return nil, err
				}
				if filepath.IsLocal(relative) {
					result.Errors = append(result.Errors, fmt.Errorf("output %s is inside canonical skill directory %s", destination, art.SourcePath))
					continue
				}
			}
			paths, err := target.OutputPaths(tgt, transformed)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}
			for _, path := range paths {
				absolute, err := resolvedPath(path)
				if err != nil {
					return nil, err
				}
				if first, ok := destinations[absolute]; ok {
					result.Errors = append(result.Errors, fmt.Errorf("output collision at %s between %s and %s", absolute, first, source))
				}
				destinations[absolute] = source
				boundary, err := resolvedPath(tgt.Path())
				if err != nil {
					return nil, err
				}
				relative, err := filepath.Rel(boundary, absolute)
				if err != nil {
					return nil, err
				}
				if !filepath.IsLocal(relative) {
					result.Errors = append(result.Errors, fmt.Errorf("output %s escapes build output %s", path, tgt.Path()))
				}
				for _, canonical := range protected {
					protected, err := resolvedPath(canonical.SourcePath)
					if err != nil {
						return nil, err
					}
					relative, err := filepath.Rel(protected, absolute)
					if err != nil {
						return nil, err
					}
					if absolute == protected || canonical.IsDirectory && filepath.IsLocal(relative) {
						result.Errors = append(result.Errors, fmt.Errorf("output %s would overwrite canonical source %s", absolute, protected))
					}
				}
			}
			jobs = append(jobs, writeJob{tgt, transformed, effective})
		}
	}
	// File/directory conflicts also fail before writing any artifact.
	for path := range destinations {
		for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if _, exists := destinations[parent]; exists {
				result.Errors = append(result.Errors, fmt.Errorf("output file/directory collision between %s and %s", parent, path))
			}
		}
	}
	// A failed document stops only itself; output-safety errors, and any error of a clean build, stop every write.
	if len(result.Errors) > documents || clean && len(result.Errors) > 0 {
		return result, nil
	}
	if clean {
		return publish(ctx, output, jobs, result)
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := job.target.Write(job.artifact); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write %s for %s: %w", job.artifact.Name, job.target.Name(), err))
			continue
		}
		if job.artifact.Type != artifact.TypeUnknown {
			result.Built++
		}
	}
	return result, nil
}

// Resolve existing ancestors too, so source and destination aliases participate
// in the same collision checks even when the final output files do not exist yet.
func resolvedPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ancestor := absolute
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			relative, err := filepath.Rel(ancestor, absolute)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", err
		}
		ancestor = parent
	}
}

func unitFor(art *artifact.Artifact, name, output string, h henia.Harness, served map[string]bool, all []*artifact.Artifact, targets map[string]map[string]string, known map[string]map[string]bool, dependencies []Dependency) *Unit {
	root := templateRoot(h.ProjectRoot, art.SourcePath, dependencies)
	failed := &Unit{Artifact: art, Package: packageName(root, dependencies)}
	effective, tr, err := transformerFor(name, output, h, art.Type)
	if err != nil {
		failed.Err = err
		return failed
	}
	tr.Served = served
	if tr.Templates, failed.Err = templates.Load(root, effective.UserRoot); failed.Err != nil {
		return failed
	}
	if Legacy(art, tr.Templates) {
		failed.Err = fmt.Errorf("%s composes with legacy templates", art.Name)
		return failed
	}
	return pipelineUnit(art, tr, effective, root, all, targets, known, dependencies)
}

func pipelineUnit(art *artifact.Artifact, tr *transform.Transformer, effective henia.Harness, root string, all []*artifact.Artifact, targets map[string]map[string]string, known map[string]map[string]bool, dependencies []Dependency) *Unit {
	if known[root] == nil {
		known[root] = Known(packageArtifacts(all, root, effective.ProjectRoot, dependencies), targets)
	}
	u := &Unit{Artifact: art, Transformer: tr, Package: packageName(root, dependencies), Root: root, Known: known[root], Variables: effective.Variables}
	if root == effective.ProjectRoot {
		u.User = effective.UserRoot
	}
	return u
}

func nativePreloads(h henia.Harness) bool {
	if h.Profile == "" {
		return false
	}
	profile, err := profile.Load(h.Profile, h.ProjectRoot, h.UserRoot)
	return err == nil && profile.Preloads
}

func projectPreloads(art *artifact.Artifact, h henia.Harness) {
	native := nativePreloads(h)
	needed := preload.Tools(art.Body)
	art.Body = preload.Project(art.Body, art.Name, native)
	if !native {
		return
	}
	for _, tool := range needed {
		switch tools := art.Frontmatter["allowed-tools"].(type) {
		case nil:
			if profile, err := profile.Load(h.Profile, h.ProjectRoot, h.UserRoot); err == nil && slices.Contains(profile.Fields, "allowed-tools") {
				art.Frontmatter["allowed-tools"] = tool
			}
		case string:
			if tools != "" && !strings.Contains(tools, tool) {
				art.Frontmatter["allowed-tools"] = tools + ", " + tool
			}
		case []any:
			if !slices.Contains(tools, any(tool)) {
				art.Frontmatter["allowed-tools"] = append(tools, tool)
			}
		}
	}
}

func Render(art *artifact.Artifact, name string, h henia.Harness, served map[string]bool, head bool) (*artifact.Artifact, error) {
	tr, err := Renderer(art, name, h, served, head)
	if err != nil {
		return nil, err
	}
	return tr.Transform(art)
}

func Renderer(art *artifact.Artifact, name string, h henia.Harness, served map[string]bool, head bool) (*transform.Transformer, error) {
	_, tr, err := transformerFor(name, "", h, art.Type)
	if err != nil {
		return nil, err
	}
	tr.Served, tr.Head, tr.LibraryLinks = served, head, true
	if tr.Templates, err = templates.Load(h.ProjectRoot, h.UserRoot); err != nil {
		return nil, err
	}
	return tr, nil
}

func Legacy(art *artifact.Artifact, shared map[string]string) bool {
	layout, _ := heniaSettings(art.Frontmatter)["layout"].(string)
	return len(shared) > 0 || layout != ""
}

func heniaSettings(frontmatter map[string]any) map[string]any {
	settings, _ := frontmatter["henia"].(map[string]any)
	return settings
}

type Unit struct {
	Artifact    *artifact.Artifact
	Transformer *transform.Transformer
	Package     string
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
}

func Expand(units []*Unit, t Target) {
	e := &expansion{sets: map[[2]string]*componentSet{}, byKey: map[components.Key]*Unit{}, byPath: map[string]*Unit{}, decl: map[string][]components.Diagnostic{}}
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
			if t.Load == nil {
				return components.Unit{}, false
			}
			u := t.Load(k)
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
	key := components.Key{Package: u.Package, Name: unitName(art)}
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
		Name: key.Name, Package: key.Package, Document: u.doc, Components: set.decls, Values: values, Variables: u.Variables,
		Finish: func(doc *markup.Document) []components.Diagnostic {
			transform.Listings(doc, art.Name, art.Type == artifact.TypeSkill)
			return nil
		},
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
		return c.ID == d.ID && c.Path == d.Path && c.Line == d.Line && c.Column == d.Column
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
			d.Notes = append(slices.Clone(d.Notes), note)
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
	u.doc, _ = markup.Parse(u.path, body)
}

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

func unitName(art *artifact.Artifact) string {
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
	builtin, err := config.Harnesses([]byte(defaults.ConfigTOML))
	if err != nil {
		return nil, err
	}
	layers := []map[string]henia.Harness{builtin}
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

func packageArtifacts(all []*artifact.Artifact, root, project string, dependencies []Dependency) []*artifact.Artifact {
	var arts []*artifact.Artifact
	for _, art := range all {
		if templateRoot(project, art.SourcePath, dependencies) == root {
			arts = append(arts, art)
		}
	}
	return arts
}

func packageName(root string, dependencies []Dependency) string {
	for _, d := range dependencies {
		if d.Root == root {
			return filepath.Base(root)
		}
	}
	return library.Project
}

func Served(all, projected []*artifact.Artifact) map[string]bool {
	served := make(map[string]bool)
	for _, art := range all {
		if art.Type == artifact.TypeSkill && (!slices.Contains(projected, art) || !Invocable(art.Frontmatter)) {
			served[art.FullName()] = true
		}
	}
	return served
}

func Invocable(frontmatter map[string]any) bool {
	settings, _ := frontmatter["henia"].(map[string]any)
	auto, set := settings["auto_invoke"].(bool)
	return !set || auto
}

func filterArtifacts(arts []*artifact.Artifact, harness henia.Harness) []*artifact.Artifact {
	var filtered []*artifact.Artifact

	allowedTypes := harness.Artifacts
	if len(allowedTypes) == 0 {
		allowedTypes = []string{"skills", "commands", "agents"}
	}

	typeSet := make(map[string]bool)
	for _, t := range allowedTypes {
		normalized := t
		if before, ok := strings.CutSuffix(t, "s"); ok {
			normalized = before
		}
		typeSet[normalized] = true
	}

	for _, art := range arts {
		if !typeSet[string(art.Type)] {
			continue
		}
		if !harness.Builds(artifact.TypeDirName(art.Type), art.Name) {
			continue
		}
		filtered = append(filtered, art)
	}

	return filtered
}

func Projects(harness henia.Harness, name string) bool { return harness.Builds("skills", name) }

func convertReferences(refs map[string]string) map[string]transform.ReferenceConfig {
	if refs == nil {
		return nil
	}
	result := make(map[string]transform.ReferenceConfig)
	for k, v := range refs {
		result[k] = transform.ReferenceConfig{Output: v}
	}
	return result
}

func markupFormat(directives string) string {
	switch directives {
	case "keep":
		return "directives"
	case "md":
		return "markdown"
	}
	return directives
}

func transformerFor(name, output string, h henia.Harness, kind artifact.Type) (henia.Harness, *transform.Transformer, error) {
	mapping := h.Type(artifact.TypeDirName(kind))
	if kind != artifact.TypeSkill {
		h.Profile = ""
	}
	if mapping.Profile != "" {
		h.Profile = mapping.Profile
	}
	if mapping.Layout != "" {
		h.Layout = mapping.Layout
	}
	h.Frontmatter.Rename = mergeMap(h.Frontmatter.Rename, mapping.Frontmatter.Rename)
	h.Frontmatter.Values = mergeMap(h.Frontmatter.Values, mapping.Frontmatter.Values)
	tr := &transform.Transformer{
		Profile: h.Profile, Strict: h.Strict, Variables: h.Variables,
		OutputFormat: markupFormat(h.Directives), Keys: h.Frontmatter.Rename, Values: h.Frontmatter.Values,
		Tools: h.Tools, References: convertReferences(h.References),
	}
	if h.Profile != "" {
		loaded, err := profile.Load(h.Profile, h.ProjectRoot, h.UserRoot)
		if err == nil {
			tr.Compiler, err = profile.NewCompiler(loaded)
		}
		if err != nil {
			return h, nil, fmt.Errorf("harness %s: %w", name, err)
		}
		tr.Context = profile.Context{Name: name, Profile: h.Profile, Path: output, Variables: h.Variables, Tools: h.Tools, Keys: h.Frontmatter.Rename}
	}
	return h, tr, nil
}

func mergeMap[V any](base, override map[string]V) map[string]V {
	result := make(map[string]V, len(base)+len(override))
	maps.Copy(result, base)
	maps.Copy(result, override)
	return result
}

func supportFiles(sources []string, files map[string]henia.File, references *transform.Transformer) (map[string][]byte, error) {
	result := make(map[string][]byte, len(files))
	for _, path := range slices.Sorted(maps.Keys(files)) {
		file := files[path]
		if !filepath.IsLocal(path) || path == "." || filepath.Clean(path) != path || strings.Contains(path, "\\") {
			return nil, fmt.Errorf("invalid supporting file output %q", path)
		}
		if !filepath.IsLocal(file.Source) {
			return nil, fmt.Errorf("invalid supporting file source %q", file.Source)
		}
		for _, source := range sources {
			data, err := os.ReadFile(filepath.Join(source, file.Source))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if _, exists := result[path]; exists {
				return nil, fmt.Errorf("multiple sources provide supporting file %s", file.Source)
			}
			text := string(data)
			if strings.EqualFold(filepath.Ext(file.Source), ".md") {
				text = references.RenderReferences(text)
			}
			replacements := make([]string, 0, 2*len(file.Replace))
			for _, old := range slices.Sorted(maps.Keys(file.Replace)) {
				if old == "" {
					return nil, fmt.Errorf("empty replacement in supporting file %s", file.Source)
				}
				replacements = append(replacements, old, file.Replace[old])
			}
			result[path] = []byte(strings.NewReplacer(replacements...).Replace(text))
		}
		if _, exists := result[path]; !exists {
			return nil, fmt.Errorf("supporting file %s not found", file.Source)
		}
	}
	return result, nil
}
