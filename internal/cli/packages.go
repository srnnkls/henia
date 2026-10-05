package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/deps"
	"github.com/srnnkls/henia/internal/library"
)

func packageScope(cmd *cobra.Command, global bool) (deps.Scope, error) {
	if global {
		manifest := filepath.Join(library.ConfigDir(), library.ConfigFile)
		return deps.Scope{Manifest: manifest, Lock: filepath.Join(library.ConfigDir(), library.LockFile), State: library.StateDir("global"), Store: library.PackagesDir("global"), Tools: filepath.Join(library.CacheDir(), "tools"), Log: cmd.ErrOrStderr()}, nil
	}
	scope, err := projectScope(projectRoot(cmd))
	scope.Log = cmd.ErrOrStderr()
	return scope, err
}

func projectScope(project string) (deps.Scope, error) {
	manifest, err := config.Find(project)
	if errors.Is(err, os.ErrNotExist) {
		manifest, err = filepath.Join(project, library.ConfigFile), nil
	}
	if err != nil {
		return deps.Scope{}, err
	}
	key := library.ProjectKey(project)
	return deps.Scope{Manifest: manifest, Lock: filepath.Join(filepath.Dir(manifest), library.LockFile), State: library.StateDir(key), Store: library.PackagesDir(key), Tools: filepath.Join(library.CacheDir(), "tools")}, nil
}

func ensurePackages(cmd *cobra.Command, project string) error {
	scope, err := projectScope(project)
	if err != nil {
		return err
	}
	scope.Log = cmd.ErrOrStderr()
	declared, err := deps.Read(scope.Manifest)
	if err != nil {
		return err
	}
	for name := range declared {
		if _, err := os.Stat(filepath.Join(scope.Store, name)); err != nil {
			report, err := deps.Sync(cmd.Context(), scope, false)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Synced %d package(s): %s\n", len(report.Packages), strings.Join(report.Packages, ", "))
			return nil
		}
	}
	return nil
}

func syncPackages(cmd *cobra.Command, scope deps.Scope, update bool) error {
	report, err := deps.Sync(cmd.Context(), scope, update)
	if err != nil {
		return err
	}
	verb := "Synced"
	if update {
		verb = "Updated"
	}
	if len(report.Packages) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: no packages declared in %s\n", verb, scope.Manifest)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %d package(s) into %s: %s\n", verb, len(report.Packages), scope.Store, strings.Join(report.Packages, ", "))
	return nil
}

func newAddCommand() *cobra.Command {
	var d deps.Dependency
	var global, offline bool
	cmd := &cobra.Command{
		Use:   "add <name> (--git URL | --path DIR)",
		Short: "Declare a skill package dependency and fetch it",
		Long: `Declare a skill package dependency in henia.toml and fetch it with phora.
A package is a folder holding skills/, its own henia.toml and, optionally,
lint modules in .henia/lint/. --global records it in the user henia.toml and
fetches it for every project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := packageScope(cmd, global)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(scope.Manifest), 0o755); err != nil {
				return err
			}
			if err := deps.Add(scope.Manifest, args[0], d); err != nil {
				return err
			}
			if offline {
				return nil
			}
			return syncPackages(cmd, scope, false)
		},
	}
	cmd.Flags().StringVar(&d.Git, "git", "", "Git URL of the package")
	cmd.Flags().StringVar(&d.Path, "path", "", "Local directory of the package")
	cmd.Flags().StringVar(&d.Branch, "branch", "", "Branch to follow")
	cmd.Flags().StringVar(&d.Tag, "tag", "", "Tag to pin")
	cmd.Flags().StringVar(&d.Rev, "rev", "", "Commit to pin")
	cmd.Flags().StringVar(&d.Root, "root", "", "Subdirectory of the repository holding the package")
	cmd.Flags().StringSliceVar(&d.Skills, "skill", nil, "Take only these skills (repeatable)")
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	cmd.Flags().BoolVar(&offline, "no-sync", false, "Only record the dependency")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a skill package dependency and its files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := packageScope(cmd, global)
			if err != nil {
				return err
			}
			if err := deps.Remove(scope.Manifest, args[0]); err != nil {
				return err
			}
			return syncPackages(cmd, scope, false)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	return cmd
}

func newSyncCommand() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch the declared skill packages at their locked commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := packageScope(cmd, global)
			if err != nil {
				return err
			}
			return syncPackages(cmd, scope, false)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Move the declared skill packages to their latest commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := packageScope(cmd, global)
			if err != nil {
				return err
			}
			return syncPackages(cmd, scope, true)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	return cmd
}
