package cli

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
)

func newBuildCommand() *cobra.Command {
	var output string
	var clean bool
	var selected []string
	cmd := &cobra.Command{
		Use:   "build [source-directory]",
		Short: "Compile local canonical artifacts for multiple harnesses",
		Long:  "Compile local canonical artifacts for multiple harnesses.\n\nWithout --config, the source directory's henia.toml is read when present, else ./henia.toml.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := "."
			if len(args) > 0 {
				source = args[0]
			} else if holdsArtifacts(library.ProjectDir) && !holdsArtifacts(".") {
				source = library.ProjectDir
			}
			info, err := os.Stat(source)
			if err != nil {
				return fmt.Errorf("source: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("source must be a directory containing skills, commands or agents")
			}
			project := source
			if filepath.Base(filepath.Clean(source)) == library.ProjectDir {
				project = filepath.Dir(filepath.Clean(source))
			}
			cfg, err := optionalConfig(cmd, project)
			if err != nil {
				return err
			}
			destination := output
			if !cmd.Flags().Changed("output") {
				destination = cfg.Build.Output
			}
			if err := ensurePackages(cmd, project); err != nil {
				return err
			}
			lib := library.Open(project, nil)
			var dependencies []library.Package
			for _, dependency := range lib.Packages {
				if dependency.Tier == library.Dependency {
					dependencies = append(dependencies, dependency)
				}
			}
			resolve := staticFiles(project, lib)
			var built int
			if owners, err := dependencyHarnesses(dependencies, cfg); err != nil {
				return err
			} else if len(owners) > 0 {
				built, err = buildDependencyHarnesses(cmd, source, cfg, owners, selected, destination, clean || cfg.Build.Clean, dependencies, resolve)
				if err != nil {
					return err
				}
			} else {
				result, err := compilePackage(cmd, source, cfg, selected, destination, clean || cfg.Build.Clean, dependencies, resolve)
				if err != nil {
					return err
				}
				built = result.Built
			}
			for dir := filepath.Clean(destination); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
				if filepath.Base(dir) == library.ProjectDir {
					if err := library.EnsureGitignore(dir); err != nil {
						return err
					}
					break
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Built %d artifact(s) in %s\n", built, destination)
			return err
		},
	}
	cmd.Flags().BoolVar(&clean, "clean", false, "Replace the entire output tree after a successful build")
	cmd.Flags().StringVarP(&output, "output", "o", ".henia/build", "Build output directory (overrides [build].output)")
	cmd.Flags().StringSliceVar(&selected, "harness", nil, "Harnesses to build (comma-separated; default all configured)")
	return cmd
}

func optionalConfig(cmd *cobra.Command, source string) (*config.Config, error) {
	if !cmd.Flags().Changed("config") {
		path, err := config.Find(source)
		if err == nil {
			cfg, err := config.Load(path)
			if err != nil {
				return nil, fmt.Errorf("load config: %w", err)
			}
			return cfg, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if path, err := config.Find("."); err == nil && configPath == "henia.toml" {
			configPath = path
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	cfg, err := config.Load(configPath)
	if err == nil {
		return cfg, nil
	}
	if cmd.Flags().Changed("config") || !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return config.LoadOptional(configPath)
}

func compilePackage(cmd *cobra.Command, source string, cfg *config.Config, selected []string, destination string, clean bool, dependencies []library.Package, resolve build.Resolver) (*build.Result, error) {
	harnesses := cfg.Harness
	if len(harnesses) == 0 {
		return nil, fmt.Errorf("no harnesses configured")
	}
	if len(selected) > 0 {
		harnesses = make(map[string]henia.Harness, len(selected))
		for _, name := range selected {
			h, ok := cfg.Harness[name]
			if !ok {
				return nil, fmt.Errorf("unknown harness %q", name)
			}
			harnesses[name] = h
		}
	}
	for name, h := range harnesses {
		if len(h.Artifacts) == 0 {
			h.Artifacts = cfg.Artifacts
		}
		harnesses[name] = h
	}
	sources := []string{source}
	for _, dependency := range dependencies {
		sources = append(sources, dependency.Root)
	}
	layers, err := dependencyLayers(dependencies)
	if err != nil {
		return nil, err
	}
	result, err := build.RunSelected(cmd.Context(), sources, destination, harnesses, cfg.Harness, clean, resolve, layers...)
	if err != nil {
		return nil, err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", warning)
	}
	if err := errors.Join(result.Errors...); err != nil {
		return nil, err
	}
	if result.Built == 0 {
		return nil, fmt.Errorf("no artifacts found for selected harnesses")
	}
	return result, nil
}

func holdsArtifacts(dir string) bool {
	for _, kind := range []string{"skills", "commands", "agents"} {
		if info, err := os.Stat(filepath.Join(dir, kind)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// dependencyHarnesses maps each harness that a dependency configures to the
// configurations of the dependencies that configure it, the project's own
// table of that harness merged over each.
func dependencyHarnesses(dependencies []library.Package, cfg *config.Config) (map[string]map[string]*config.Config, error) {
	project, err := config.HarnessTables(cfg.Path)
	if err != nil {
		return nil, err
	}
	owners := map[string]map[string]*config.Config{}
	for _, dependency := range dependencies {
		if dependency.Config == "" {
			continue
		}
		own, err := config.HarnessTables(dependency.Config)
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", dependency.Name, err)
		}
		if len(own) == 0 {
			continue
		}
		pkgCfg, err := config.LoadPackage(dependency.Config, project)
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", dependency.Name, err)
		}
		for name := range own {
			if owners[name] == nil {
				owners[name] = map[string]*config.Config{}
			}
			owners[name][dependency.Name] = pkgCfg
		}
	}
	return owners, nil
}

// buildDependencyHarnesses builds every harness that the project or a
// dependency configures. A harness a dependency configures renders the way
// henia install renders it, with the project's skills as one more package.
func buildDependencyHarnesses(cmd *cobra.Command, source string, cfg *config.Config, owners map[string]map[string]*config.Config, selected []string, destination string, clean bool, dependencies []library.Package, resolve build.Resolver) (int, error) {
	root, err := filepath.Abs(source)
	if err != nil {
		return 0, err
	}
	project := library.Package{Name: library.Project, Tier: library.Project, Root: root, Config: cfg.Path}
	packages := append(slices.Clone(dependencies), project)
	tables, err := config.HarnessTables(cfg.Path)
	if err != nil {
		return 0, err
	}
	own, err := projectHarnesses(tables)
	if err != nil {
		return 0, err
	}
	names := slices.Collect(maps.Keys(owners))
	for _, name := range cfg.Selected {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(selected) > 0 {
		for _, name := range selected {
			if !slices.Contains(names, name) {
				return 0, fmt.Errorf("unknown harness %q", name)
			}
		}
		names = selected
	}
	arts, err := artifact.Discover(root, []string{"skills", "commands", "agents"})
	if err != nil {
		return 0, err
	}
	staging, err := os.MkdirTemp("", "henia-build-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(staging)
	trees := map[string]*harnessTree{}
	built := 0
	for _, name := range names {
		harnessOwners := map[string]*config.Config{}
		for pkg, pkgCfg := range owners[name] {
			pkgCfg.Harness[name] = projectModes(pkgCfg.Harness[name], own[name], arts)
			harnessOwners[pkg] = pkgCfg
		}
		if len(harnessOwners) == 0 {
			harnessOwners[library.Project] = cfg
		}
		tree, err := renderHarness(cmd, name, packages, harnessOwners, filepath.Join(staging, name), resolve)
		if err != nil {
			return 0, fmt.Errorf("harness %s: %w", name, err)
		}
		trees[name], built = tree, built+tree.built
	}
	write := func(output string) error {
		for _, name := range names {
			for rel, path := range trees[name].files {
				if err := copyFile(path, filepath.Join(output, name, rel)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if clean {
		sources := []string{root}
		for _, pkg := range dependencies {
			sources = append(sources, pkg.Root)
		}
		return built, build.Publish(sources, destination, write)
	}
	return built, write(destination)
}

func projectHarnesses(tables map[string]any) (map[string]henia.Harness, error) {
	data, err := toml.Marshal(map[string]any{"harness": tables})
	if err != nil {
		return nil, err
	}
	return config.Harnesses(data)
}

// projectModes gives the project's own artifacts the modes the project's
// harness table assigns them, static when it assigns none.
func projectModes(h, own henia.Harness, arts []*artifact.Artifact) henia.Harness {
	for _, kind := range []string{"skills", "commands", "agents"} {
		modes := h.Type(kind)
		lists := map[string][]string{henia.Static: modes.Static, henia.Dynamic: modes.Dynamic, henia.Hybrid: modes.Hybrid}
		for _, art := range arts {
			if artifact.TypeDirName(art.Type) != kind {
				continue
			}
			for mode, names := range lists {
				lists[mode] = slices.DeleteFunc(slices.Clone(names), func(name string) bool { return name == art.Name })
			}
			mode := own.Mode(kind, art.Name)
			lists[mode] = append(lists[mode], art.Name)
		}
		modes.Static, modes.Dynamic, modes.Hybrid = lists[henia.Static], lists[henia.Dynamic], lists[henia.Hybrid]
		switch kind {
		case "skills":
			h.Skills = modes
		case "commands":
			h.Commands = modes
		case "agents":
			h.Agents = modes
		}
	}
	return h
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, data, info.Mode().Perm())
}
