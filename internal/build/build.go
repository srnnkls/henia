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

type Result struct {
	Built    int
	Errors   []error
	Warnings []string
}

// Run compiles local source directories. Each harness writes beneath output/name.
func Run(ctx context.Context, sources []string, output string, harnesses map[string]henia.Harness) (*Result, error) {
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

	type writeJob struct {
		target   target.Target
		artifact *artifact.Artifact
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

		tgt := target.NewFromConfig(harnessName, outputPath, harness)
		tr := &transform.Transformer{
			Profile:      harness.Profile,
			Strict:       harness.Strict,
			Variables:    harness.Variables,
			OutputFormat: harness.Format,
			Keys:         harness.Keys,
			Values:       harness.Values,
			Tools:        harness.Tools,
			References:   convertReferences(harness.References),
		}
		if harness.Profile != "" {
			profile, err := vendor.Load(harness.Profile, harness.ProjectRoot, harness.UserRoot)
			if err == nil {
				tr.Compiler, err = vendor.NewCompiler(profile)
			}
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("harness %s: %w", harnessName, err))
				continue
			}
			tr.Context = vendor.Context{Name: harnessName, Profile: harness.Profile, Path: outputPath, Variables: harness.Variables, Tools: harness.Tools, Keys: harness.Keys}
		}

		for _, art := range filtered {
			transformed, err := tr.Transform(art)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("%s: transform %s for %s: %w", art.SourcePath, art.Name, harnessName, err))
				continue
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
				for _, canonical := range allArtifacts {
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
			jobs = append(jobs, writeJob{tgt, transformed})
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
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := job.target.Write(job.artifact); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("write %s for %s: %w", job.artifact.Name, job.target.Name(), err))
			continue
		}
		result.Built++
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
