package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	key := library.StoreKey(project, manifest)
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
			report, _, err := syncProject(cmd.Context(), project, cmd.ErrOrStderr(), false)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Synced %d package(s): %s\n", len(report.Packages), strings.Join(report.Packages, ", "))
			return nil
		}
	}
	return nil
}

func syncThenInstall(cmd *cobra.Command, scope deps.Scope, update, global, noInstall bool) error {
	if err := syncPackages(cmd, scope, update, global); err != nil || noInstall {
		return err
	}
	if global {
		return installGlobal(cmd, nil, false, false)
	}
	return installProject(cmd, projectRoot(cmd), nil, false, false)
}

func syncPackages(cmd *cobra.Command, scope deps.Scope, update, global bool) error {
	var report deps.Report
	var err error
	if global {
		report, err = deps.Sync(cmd.Context(), scope, update)
	} else {
		report, scope, err = syncProject(cmd.Context(), projectRoot(cmd), cmd.ErrOrStderr(), update)
	}
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
		Use:   "add <name> (--repo OWNER/REPO | --git URL | --path DIR)",
		Short: "Declare a skill package dependency and fetch it",
		Long: `Declare a skill package dependency in henia.toml and fetch it with phora.
A package is a folder holding skills/, its own henia.toml and, optionally,
lint modules in .henia/lint/. --global records it in the user henia.toml and
fetches it for every project.

--repo takes a forge shorthand as phora add does: owner/repo on GitHub,
<host>:<owner>/<repo>, or <domain>/<owner>/<repo> for a built-in forge such
as gitlab.com. Segments past owner/repo name the package root. It is
recorded as host and repo, never as an expanded URL.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if d.Repo != "" {
				host, repo, root, err := deps.Forge(d.Repo)
				if err != nil {
					return err
				}
				if host != "" && d.Host != "" && host != d.Host {
					return fmt.Errorf("--repo %s names the forge %s, not %s", d.Repo, host, d.Host)
				}
				if root != "" && d.Root != "" {
					return fmt.Errorf("--repo %s names the root %s; drop --root", d.Repo, root)
				}
				d.Host, d.Repo, d.Root = cmp.Or(host, d.Host, "github"), repo, cmp.Or(d.Root, root)
			}
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
			return syncThenInstall(cmd, scope, false, global, false)
		},
	}
	cmd.Flags().StringVar(&d.Repo, "repo", "", "Forge repository of the package: owner/repo, <host>:<owner>/<repo> or <domain>/<owner>/<repo>")
	cmd.Flags().StringVar(&d.Host, "host", "", "Forge of --repo owner/repo (default github)")
	cmd.Flags().StringVar(&d.Git, "git", "", "Git URL of the package")
	cmd.Flags().StringVar(&d.Path, "path", "", "Local directory of the package")
	cmd.Flags().StringVar(&d.Branch, "branch", "", "Branch to follow")
	cmd.Flags().StringVar(&d.Tag, "tag", "", "Tag to pin")
	cmd.Flags().StringVar(&d.Rev, "rev", "", "Commit to pin")
	cmd.Flags().StringVar(&d.Root, "root", "", "Subdirectory of the repository holding the package")
	cmd.Flags().StringSliceVar(&d.Skills, "skill", nil, "Take only these skills (repeatable)")
	cmd.Flags().BoolVar(&d.Transitive, "transitive", false, "Fetch the package with the dependencies its phora.toml declares")
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	cmd.Flags().BoolVar(&offline, "no-sync", false, "Only record the dependency")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	var global, noInstall bool
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
			return syncThenInstall(cmd, scope, false, global, noInstall)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	cmd.Flags().BoolVar(&noInstall, "no-install", false, "Do not install afterwards")
	return cmd
}

func newSyncCommand() *cobra.Command {
	var global, noInstall bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch the declared skill packages at their locked commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := packageScope(cmd, global)
			if err != nil {
				return err
			}
			return syncThenInstall(cmd, scope, false, global, noInstall)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	cmd.Flags().BoolVar(&noInstall, "no-install", false, "Do not install afterwards")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	var global, noInstall bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Move the declared skill packages to their latest commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := packageScope(cmd, global)
			if err != nil {
				return err
			}
			return syncThenInstall(cmd, scope, true, global, noInstall)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Use the user henia.toml and the shared package store")
	cmd.Flags().BoolVar(&noInstall, "no-install", false, "Do not install afterwards")
	return cmd
}

// storeRetention is how long a project store nothing uses survives.
const storeRetention = 30 * 24 * time.Hour

// syncProject syncs a project into the store its lock keys. Checkouts on one
// lock share that store, so an update resolves the new lock in a store of its
// own, discarded afterwards; a sync that moves the lock, as a first sync or an
// update does, syncs once more into the store the moved lock keys.
func syncProject(ctx context.Context, project string, log io.Writer, update bool) (deps.Report, deps.Scope, error) {
	scope, err := projectScope(project)
	if err != nil {
		return deps.Report{}, scope, err
	}
	scope.Log = log
	run := scope
	if update {
		staging := "update-" + library.ProjectKey(project)
		run.State, run.Store = library.StateDir(staging), library.PackagesDir(staging)
		for _, dir := range []string{run.State, run.Store} {
			if err := os.RemoveAll(dir); err != nil {
				return deps.Report{}, scope, err
			}
			defer os.RemoveAll(dir)
		}
	}
	report, err := deps.Sync(ctx, run, update)
	if err != nil {
		return report, scope, err
	}
	if moved, err := projectScope(project); err == nil && (update || moved.Store != scope.Store) {
		moved.Log = log
		scope = moved
		if report, err = deps.Sync(ctx, scope, false); err != nil {
			return report, scope, err
		}
	}
	link, err := config.LinksResources(scope.Manifest)
	if err != nil {
		return report, scope, err
	}
	if err := library.LinkResources(projectSource(project), link); err != nil {
		return report, scope, err
	}
	key := filepath.Base(scope.Store)
	library.Touch(key)
	return report, scope, library.PruneStores(key, storeRetention)
}
