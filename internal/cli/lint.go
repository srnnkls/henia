package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/profile"
)

func newLintCommand() *cobra.Command {
	var format string
	var strict bool
	var disabled []string
	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Check skill metadata, references, duplication and freshness",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "json" {
				return fmt.Errorf("unknown lint format %q (use text or json)", format)
			}
			options, cfg, err := lintOptions(cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				if args = packageContents(projectRoot(cmd), cfg); len(args) == 0 {
					return fmt.Errorf("no package contents to lint: no skills, commands or agents under the project or its .henia; pass the paths to lint")
				}
			}
			options.Disable = append(options.Disable, disabled...)
			plan, err := lint.Compile(options)
			if err != nil {
				return err
			}
			diagnostics, err := plan.Run(cmd.Context(), args)
			if err != nil {
				return err
			}
			for i := range diagnostics {
				projectRelative(&diagnostics[i], options.Project)
			}
			if format == "json" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(diagnostics); err != nil {
					return err
				}
			} else {
				for _, d := range diagnostics {
					location := d.Path
					if d.Line > 0 {
						location = fmt.Sprintf("%s:%d:%d", d.Path, d.Line, d.Column)
					}
					message := d.Message
					if d.Harness != "" {
						message = d.Harness + ": " + message
					}
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s [%s] %s\n", location, d.Severity, d.Rule, message); err != nil {
						return err
					}
					for _, note := range d.Notes {
						if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  note: %s\n", note); err != nil {
							return err
						}
					}
				}
			}
			for _, d := range diagnostics {
				if d.Severity == "error" || strict {
					return fmt.Errorf("lint found %d diagnostic(s)", len(diagnostics))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Diagnostic output: text or json")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit unsuccessfully on warnings as well as errors")
	cmd.Flags().StringSliceVar(&disabled, "disable", nil, "Disable lint rules (comma-separated)")
	cmd.AddCommand(newLintTestCommand())
	return cmd
}

func newLintTestCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Run the Matches and Passes examples of every lint module",
		Long: `Run the Matches and Passes examples of every lint module: the standard
library, skill packages' .henia/lint/ directories, the user's and the project's.
A Matches example must make its rule report; a Passes example must not.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, _, err := lintOptions(cmd)
			if err != nil {
				return err
			}
			plan, err := lint.Compile(options)
			if err != nil {
				return err
			}
			failures, count, err := plan.Test()
			if err != nil {
				return err
			}
			for _, f := range failures {
				fmt.Fprintln(cmd.OutOrStdout(), f.Error())
			}
			if len(failures) > 0 {
				return fmt.Errorf("%d of %d lint examples failed", len(failures), count)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d lint examples passed\n", count)
			return nil
		},
	}
}

func lintOptions(cmd *cobra.Command) (lint.Options, *config.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil && (cmd.Flags().Changed("config") || !errors.Is(err, os.ErrNotExist)) {
		return lint.Options{}, nil, fmt.Errorf("load config: %w", err)
	}
	if err != nil {
		if cfg, err = config.LoadOptional(configPath); err != nil {
			return lint.Options{}, nil, err
		}
	}
	options := cfg.Lint
	project := projectRoot(cmd)
	if err := ensurePackages(cmd, project); err != nil {
		return lint.Options{}, nil, err
	}
	lib := library.Open(project, nil)
	for _, pkg := range lib.Packages {
		if pkg.Tier == library.Dependency {
			options.Modules = append(options.Modules, lint.ModuleDir{Dir: filepath.Join(pkg.Root, library.ProjectDir, "lint"), Prefix: pkg.Name})
		}
	}
	for _, entry := range lib.Entries {
		if entry.Tier == library.Dependency {
			options.Dependencies = append(options.Dependencies, entry.Path)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Harness)) {
		h := cfg.Harness[name]
		if h.Profile == "" {
			continue
		}
		profile, err := profile.Load(h.Profile, project, library.ConfigDir())
		if err != nil {
			return lint.Options{}, nil, fmt.Errorf("harness %s: %w", name, err)
		}
		for _, command := range profile.Commands {
			options.Builtin = append(options.Builtin, "command:"+command)
		}
		for _, agent := range profile.Agents {
			options.Builtin = append(options.Builtin, "agent:"+agent)
		}
	}
	options.Project, options.User, options.Configured = project, library.ConfigDir(), cfg.Harness
	var dependencies []library.Package
	for _, pkg := range lib.Packages {
		if pkg.Tier == library.Dependency {
			dependencies = append(dependencies, pkg)
		}
	}
	if options.Packages, err = dependencyLayers(dependencies); err != nil {
		return lint.Options{}, nil, err
	}
	options.Modules = append(options.Modules, lint.ModuleDir{Dir: filepath.Join(library.ConfigDir(), "lint")}, lint.ModuleDir{Dir: filepath.Join(project, library.ProjectDir, "lint")})
	return options, cfg, nil
}

func projectRelative(d *lint.Diagnostic, project string) {
	relative := func(path string) string {
		if rel, err := filepath.Rel(project, path); err == nil && filepath.IsAbs(path) && filepath.IsLocal(rel) {
			return rel
		}
		return path
	}
	d.Path = relative(d.Path)
	for i := range d.Related {
		d.Related[i].Path = relative(d.Related[i].Path)
	}
	for i, note := range d.Notes {
		d.Notes[i] = strings.ReplaceAll(note, project+string(filepath.Separator), "")
	}
}

func dependencyLayers(dependencies []library.Package) ([]build.Dependency, error) {
	var layers []build.Dependency
	for _, dependency := range dependencies {
		own := map[string]henia.Harness{}
		if data, err := os.ReadFile(dependency.Config); err == nil {
			if own, err = config.Harnesses(data); err != nil {
				return nil, fmt.Errorf("package %s: %w", dependency.Name, err)
			}
		}
		layers = append(layers, build.Dependency{Root: dependency.Root, Harnesses: own})
	}
	return layers, nil
}

func packageContents(project string, cfg *config.Config) []string {
	types := slices.Clone(cfg.Artifacts)
	for _, h := range cfg.Harness {
		types = append(types, h.Artifacts...)
	}
	if len(types) == 0 {
		types = []string{"skills", "commands", "agents"}
	}
	var paths []string
	add := func(path string) {
		if _, err := os.Stat(path); err == nil && !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	for _, root := range []string{project, filepath.Join(project, library.ProjectDir)} {
		for _, kind := range types {
			add(filepath.Join(root, kind))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Harness)) {
		for _, file := range cfg.Harness[name].Files {
			add(filepath.Join(project, file.Source))
		}
	}
	return paths
}
