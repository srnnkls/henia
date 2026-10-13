// Package build compiles local canonical artifacts into a separate output tree.
package build

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/pipeline"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/profile"
	"github.com/srnnkls/henia/internal/target"
	"github.com/srnnkls/henia/internal/transform"
)

type (
	Dependency  = pipeline.Dependency
	Unit        = pipeline.Unit
	Target      = pipeline.Target
	Diagnostics = pipeline.Diagnostics
)

var (
	Expand    = pipeline.Expand
	Known     = pipeline.Known
	Targets   = pipeline.Targets
	Served    = pipeline.Served
	Invocable = pipeline.Invocable
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

func Run(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness, dependencies ...Dependency) (*Result, error) {
	return RunSelected(ctx, sources, output, harnesses, harnesses, false, nil, dependencies...)
}

func RunArtifact(ctx context.Context, art *artifact.Artifact, source, output string, harnesses map[string]henia.Harness) (*Result, error) {
	return run(ctx, []string{source}, output, harnesses, harnesses, false, nil, nil, []*artifact.Artifact{art})
}

// RunClean publishes a complete output tree only after every artifact was written successfully.
func RunClean(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness, dependencies ...Dependency) (*Result, error) {
	return RunSelected(ctx, sources, output, harnesses, harnesses, true, nil, dependencies...)
}

// Resolver resolves the henia preloads in a Markdown harness file rendered for
// the named harness. A nil Resolver leaves the file's directives in place.
type Resolver func(harness, text string) string

// RunSelected writes the harnesses; conditions name every configured harness.
func RunSelected(ctx context.Context, sources []string, output string, harnesses, configured map[string]henia.Harness, clean bool, resolve Resolver, dependencies ...Dependency) (*Result, error) {
	if clean {
		if err := validateCleanOutput(sources, output); err != nil {
			return nil, err
		}
	}
	return run(ctx, sources, output, harnesses, configured, clean, resolve, dependencies, nil)
}

func run(ctx context.Context, sources []string, output string, harnesses, configured map[string]henia.Harness, clean bool, resolve Resolver, dependencies []Dependency, extra []*artifact.Artifact) (*Result, error) {
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
	targets, err := pipeline.Targets(configured, dependencies)
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
		filtered := pipeline.Projected(allArtifacts, harness)
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

		references := &transform.Transformer{Tools: harness.Tools, References: pipeline.References(harness.References), Served: served}
		files, err := supportFiles(sources, harnessName, harness.Files, references, resolve)
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
				units = append(units, unitFor(art, harnessName, outputPath, pipeline.Layered(harness, harnessName, art.SourcePath, dependencies), served, allArtifacts, targets, known, dependencies))
			}
		}
		for _, art := range filtered {
			h := pipeline.Layered(harness, harnessName, art.SourcePath, dependencies)
			root := pipeline.Root(h.ProjectRoot, art.SourcePath, dependencies)
			effective, tr, err := pipeline.Transformer(harnessName, outputPath, h, art.Type)
			if err != nil {
				documentError(err)
				units = append(units, &Unit{Artifact: art, Package: pipeline.PackageName(root, dependencies), Err: err})
				continue
			}
			tr.Served = served
			hybrid := art.Type == artifact.TypeSkill && art.Name != CatalogName && harness.Mode("skills", art.Name) == henia.Hybrid
			tr.Head, tr.LibraryLinks = hybrid, hybrid
			p := pending{art: art, transformed: art, effective: effective, tgt: target.NewFromConfig(harnessName, outputPath, effective), hybrid: hybrid}
			if art.Type != artifact.TypeUnknown {
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
	root := pipeline.Root(h.ProjectRoot, art.SourcePath, dependencies)
	failed := &Unit{Artifact: art, Package: pipeline.PackageName(root, dependencies)}
	effective, tr, err := pipeline.Transformer(name, output, h, art.Type)
	if err != nil {
		failed.Err = err
		return failed
	}
	tr.Served = served
	return pipelineUnit(art, tr, effective, root, all, targets, known, dependencies)
}

func pipelineUnit(art *artifact.Artifact, tr *transform.Transformer, effective henia.Harness, root string, all []*artifact.Artifact, targets map[string]map[string]string, known map[string]map[string]bool, dependencies []Dependency) *Unit {
	if known[root] == nil {
		known[root] = pipeline.Known(pipeline.PackageArtifacts(all, root, effective.ProjectRoot, dependencies), targets)
	}
	u := &Unit{Artifact: art, Transformer: tr, Package: pipeline.PackageName(root, dependencies), Root: root, Known: known[root], Variables: effective.Variables}
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

func Renderer(art *artifact.Artifact, name string, h henia.Harness, served map[string]bool, head bool) (*transform.Transformer, error) {
	return pipeline.Renderer(art, name, h, served, head)
}

func Projects(harness henia.Harness, name string) bool { return harness.Builds("skills", name) }

func supportFiles(sources []string, harness string, files map[string]henia.File, references *transform.Transformer, resolve Resolver) (map[string][]byte, error) {
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
				if resolve != nil {
					text = resolve(harness, transform.ExpandFile(text))
				}
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
