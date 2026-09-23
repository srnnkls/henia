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
	"github.com/srnnkls/henia/internal/target"
	"github.com/srnnkls/henia/internal/transform"
	"github.com/srnnkls/henia/internal/vendor"
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
}

// Run compiles local source directories. Each harness writes beneath output/name.
func Run(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness) (*Result, error) {
	return run(ctx, sources, output, harnesses, false)
}

// RunClean publishes a complete output tree only after every artifact was written successfully.
func RunClean(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness) (*Result, error) {
	if err := validateCleanOutput(sources, output); err != nil {
		return nil, err
	}
	return run(ctx, sources, output, harnesses, true)
}

func run(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness, clean bool) (*Result, error) {
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
	result := &Result{}

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
	var jobs []writeJob
	destinations := make(map[string]string)
	for _, harnessName := range slices.Sorted(maps.Keys(harnesses)) {
		harness := harnesses[harnessName]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outputPath := filepath.Join(output, harnessName)
		filtered := filterArtifacts(allArtifacts, harness)

		files, err := supportFiles(sources, harness.Files)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("harness %s: %w", harnessName, err))
			continue
		}
		if len(files) > 0 {
			filtered = append(filtered, &artifact.Artifact{Name: "support files", SourcePath: sources[0], Files: files})
		}
		for _, art := range filtered {
			effective, tr, err := transformerFor(harnessName, outputPath, harness, art.Type)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}
			tgt := target.NewFromConfig(harnessName, outputPath, effective)
			transformed := art
			if art.Type != artifact.TypeUnknown {
				transformed, err = tr.Transform(art)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("%s: transform %s for %s: %w", art.SourcePath, art.Name, harnessName, err))
					continue
				}
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
	// Validate every transformation before touching the output tree.
	if len(result.Errors) > 0 {
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
		if !shouldBuild(art.Name, harness) {
			continue
		}
		filtered = append(filtered, art)
	}

	return filtered
}

func shouldBuild(name string, harness henia.Harness) bool {
	if len(harness.Include) > 0 {
		found := slices.Contains(harness.Include, name)
		if !found {
			return false
		}
	}

	return !slices.Contains(harness.Exclude, name)
}

func convertReferences(refs map[string]henia.ReferenceConfig) map[string]transform.ReferenceConfig {
	if refs == nil {
		return nil
	}
	result := make(map[string]transform.ReferenceConfig)
	for k, v := range refs {
		result[k] = transform.ReferenceConfig{Output: v.Output}
	}
	return result
}

func transformerFor(name, output string, h henia.Harness, kind artifact.Type) (henia.Harness, *transform.Transformer, error) {
	mapping := h.ArtifactMappings[artifact.TypeDirName(kind)]
	if kind != artifact.TypeSkill {
		h.Profile = ""
	}
	if mapping.Profile != "" {
		h.Profile = mapping.Profile
	}
	if mapping.Structure != "" {
		h.Structure = mapping.Structure
	}
	h.Keys = mergeMap(h.Keys, mapping.Keys)
	h.Values = mergeMap(h.Values, mapping.Values)
	tr := &transform.Transformer{
		Profile: h.Profile, Strict: h.Strict, Variables: h.Variables,
		OutputFormat: h.Format, Keys: h.Keys, Values: h.Values,
		Tools: h.Tools, References: convertReferences(h.References),
	}
	if h.Profile != "" {
		profile, err := vendor.Load(h.Profile, h.ProjectRoot, h.UserRoot)
		if err == nil {
			tr.Compiler, err = vendor.NewCompiler(profile)
		}
		if err != nil {
			return h, nil, fmt.Errorf("harness %s: %w", name, err)
		}
		tr.Context = vendor.Context{Name: name, Profile: h.Profile, Path: output, Variables: h.Variables, Tools: h.Tools, Keys: h.Keys}
	}
	return h, tr, nil
}

func mergeMap[V any](base, override map[string]V) map[string]V {
	result := make(map[string]V, len(base)+len(override))
	maps.Copy(result, base)
	maps.Copy(result, override)
	return result
}

func supportFiles(sources []string, files map[string]henia.File) (map[string][]byte, error) {
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
			replacements := make([]string, 0, 2*len(file.Replace))
			for _, old := range slices.Sorted(maps.Keys(file.Replace)) {
				if old == "" {
					return nil, fmt.Errorf("empty replacement in supporting file %s", file.Source)
				}
				replacements = append(replacements, old, file.Replace[old])
			}
			result[path] = []byte(strings.NewReplacer(replacements...).Replace(string(data)))
		}
		if _, exists := result[path]; !exists {
			return nil, fmt.Errorf("supporting file %s not found", file.Source)
		}
	}
	return result, nil
}
