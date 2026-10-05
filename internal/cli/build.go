package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia"
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
			}
			info, err := os.Stat(source)
			if err != nil {
				return fmt.Errorf("source: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("source must be a directory containing skills, commands or agents")
			}
			cfg, err := optionalConfig(cmd, source)
			if err != nil {
				return err
			}
			destination := output
			if !cmd.Flags().Changed("output") {
				destination = cfg.Build.Output
			}
			if err := ensurePackages(cmd, source); err != nil {
				return err
			}
			var dependencies []library.Package
			for _, dependency := range library.Open(source, nil).Packages {
				if dependency.Tier == library.Dependency {
					dependencies = append(dependencies, dependency)
				}
			}
			result, err := compilePackage(cmd, source, cfg, selected, destination, clean || cfg.Build.Clean, dependencies)
			if err != nil {
				return err
			}
			for dir := filepath.Clean(destination); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
				if filepath.Base(dir) == library.ProjectDir {
					if err := library.EnsureGitignore(dir); err != nil {
						return err
					}
					break
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Built %d artifact(s) in %s\n", result.Built, destination)
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

func compilePackage(cmd *cobra.Command, source string, cfg *config.Config, selected []string, destination string, clean bool, dependencies []library.Package) (*build.Result, error) {
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
	compile := build.Run
	if clean {
		compile = build.RunClean
	}
	sources := []string{source}
	var layers []build.Dependency
	for _, dependency := range dependencies {
		sources = append(sources, dependency.Root)
		own := map[string]henia.Harness{}
		if data, err := os.ReadFile(dependency.Config); err == nil {
			if own, err = config.Harnesses(data); err != nil {
				return nil, fmt.Errorf("package %s: %w", dependency.Name, err)
			}
		}
		layers = append(layers, build.Dependency{Root: dependency.Root, Harnesses: own})
	}
	result, err := compile(cmd.Context(), sources, destination, harnesses, layers...)
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
