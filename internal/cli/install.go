package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/library"
)

func newInstallCommand() *cobra.Command {
	var selected []string
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write the global skill packages into harness directories",
		Long: `Write the global skill packages into harness directories: static skills, hybrid
heads, the catalog skill and harness files, rendered by each package that
configures the harness. [install.<harness>] path in the user henia.toml names
each directory. A file that henia did not install is never overwritten without
--force, and files a previous install wrote but this one does not are removed
unless they were edited since. henia sync --global installs afterwards.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return install(cmd, selected, force, true)
		},
	}
	cmd.Flags().StringSliceVar(&selected, "harness", nil, "Harnesses to install (comma-separated; default all in [install])")
	cmd.Flags().BoolVar(&force, "force", false, "Replace files henia did not install")
	return cmd
}

func install(cmd *cobra.Command, selected []string, force, explicit bool) error {
	manifest := filepath.Join(library.ConfigDir(), library.ConfigFile)
	cfg, err := config.LoadOptional(manifest)
	if err != nil {
		return err
	}
	if len(cfg.Install) == 0 {
		if explicit {
			fmt.Fprintf(cmd.OutOrStdout(), "Nothing to install: %s has no [install.<harness>] path\n", manifest)
		}
		return nil
	}
	var globals []library.Package
	for _, pkg := range library.Open("", nil).Packages {
		if pkg.Tier == library.Global {
			globals = append(globals, pkg)
		}
	}
	for _, harness := range slices.Sorted(maps.Keys(cfg.Install)) {
		if len(selected) > 0 && !slices.Contains(selected, harness) {
			continue
		}
		if err := installHarness(cmd, cfg, harness, expandHome(cfg.Install[harness].Path), globals, force); err != nil {
			return fmt.Errorf("install %s: %w", harness, err)
		}
	}
	return nil
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}

func installHarness(cmd *cobra.Command, cfg *config.Config, harness, target string, globals []library.Package, force bool) error {
	staging, err := os.MkdirTemp("", "henia-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	owners := map[string]*config.Config{}
	for _, pkg := range globals {
		if pkg.Config == "" {
			continue
		}
		pkgCfg, err := config.LoadOptional(pkg.Config)
		if err != nil {
			return fmt.Errorf("package %s: %w", pkg.Name, err)
		}
		if h, ok := pkgCfg.Harness[harness]; ok {
			if cfg.HeniaSkill() {
				disabled := false
				h.Skills.Catalog = &disabled
				pkgCfg.Harness[harness] = h
			}
			owners[pkg.Name] = pkgCfg
		}
	}
	files := map[string]string{}
	from := map[string]string{}
	var dynamic []build.Entry
	collect := func(owner, root string) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if previous, ok := files[rel]; ok && !sameFile(previous, path) {
				return fmt.Errorf("%s is written by both %s and %s", rel, from[rel], owner)
			}
			files[rel], from[rel] = path, owner
			return nil
		})
	}
	for _, pkg := range globals {
		pkgCfg, ok := owners[pkg.Name]
		if !ok {
			continue
		}
		var dependencies []library.Package
		for _, other := range globals {
			if _, owner := owners[other.Name]; !owner {
				dependencies = append(dependencies, other)
			}
		}
		out := filepath.Join(staging, pkg.Name)
		result, err := compilePackage(cmd, pkg.Root, pkgCfg, []string{harness}, out, false, dependencies)
		if err != nil {
			return fmt.Errorf("package %s: %w", pkg.Name, err)
		}
		for _, e := range result.Dynamic[harness] {
			if !slices.ContainsFunc(dynamic, func(d build.Entry) bool { return d.Name == e.Name }) {
				dynamic = append(dynamic, e)
			}
		}
		if err := collect(pkg.Name, filepath.Join(out, harness)); err != nil {
			return err
		}
	}
	if cfg.HeniaSkill() {
		if len(owners) == 0 {
			for _, e := range library.Open("", nil).Entries {
				if e.Tier == library.Global {
					dynamic = append(dynamic, build.Entry{Name: e.Name, Description: strings.Join(strings.Fields(e.Description), " ")})
				}
			}
		}
		profile, ok := cfg.Harness[harness]
		for _, name := range slices.Sorted(maps.Keys(owners)) {
			profile, ok = owners[name].Harness[harness], true
			break
		}
		if err := heniaSkill(cmd, harness, profile, ok, staging, dynamic, collect); err != nil {
			return err
		}
	}
	written, removed, err := apply(target, files, library.InstallManifest(harness), force)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Installed %d file(s) for %s into %s", written, harness, target)
	if removed > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), ", removed %d", removed)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

func heniaSkill(cmd *cobra.Command, harness string, h henia.Harness, ok bool, staging string, dynamic []build.Entry, collect func(owner, root string) error) error {
	if !ok {
		defaults, err := defaults.DefaultConfig()
		if err != nil {
			return err
		}
		if h, ok = defaults.Harness[harness]; !ok {
			return fmt.Errorf("harness %s has no profile for the henia skill; configure [harness.%s] in the user henia.toml or set install_henia_skill = false", harness, harness)
		}
	}
	if h.ProjectRoot == "" {
		h.ProjectRoot, h.UserRoot = library.ConfigDir(), library.ConfigDir()
	}
	disabled := false
	h.Files, h.Artifacts, h.Skills = nil, []string{"skills"}, henia.Artifacts{Catalog: &disabled}
	slices.SortFunc(dynamic, func(a, b build.Entry) int { return strings.Compare(a.Name, b.Name) })
	source := filepath.Join(staging, "henia-source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		return err
	}
	art, err := build.HeniaArtifact(source, dynamic, h)
	if err != nil {
		return err
	}
	out := filepath.Join(staging, "henia")
	result, err := build.RunArtifact(cmd.Context(), art, source, out, map[string]henia.Harness{harness: h})
	if err != nil {
		return err
	}
	if err := errors.Join(result.Errors...); err != nil {
		return err
	}
	return collect("henia", filepath.Join(out, harness))
}

func sameFile(a, b string) bool {
	x, errA := os.ReadFile(a)
	y, errB := os.ReadFile(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

type installRecord struct {
	Target string            `json:"target"`
	Files  map[string]string `json:"files"`
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func apply(target string, files map[string]string, manifest string, force bool) (int, int, error) {
	var previous installRecord
	if data, err := os.ReadFile(manifest); err == nil {
		if err := json.Unmarshal(data, &previous); err != nil {
			return 0, 0, fmt.Errorf("%s: %w", manifest, err)
		}
	}
	if previous.Target != "" && previous.Target != target {
		previous = installRecord{}
	}
	var conflicts []string
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		dest := filepath.Join(target, rel)
		info, err := os.Lstat(dest)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, 0, err
		}
		current, _ := os.ReadFile(dest)
		recorded, ours := previous.Files[rel]
		staged, _ := os.ReadFile(files[rel])
		switch {
		case info.Mode()&os.ModeSymlink != 0 || info.IsDir():
			conflicts = append(conflicts, rel)
		case ours && recorded == hash(current):
		case bytes.Equal(current, staged):
		default:
			conflicts = append(conflicts, rel)
		}
	}
	if len(conflicts) > 0 && !force {
		shown := conflicts[:min(len(conflicts), 5)]
		return 0, 0, fmt.Errorf("%d file(s) in %s were not installed by henia or were edited since, among them %s; move them away or pass --force", len(conflicts), target, strings.Join(shown, ", "))
	}
	record := installRecord{Target: target, Files: map[string]string{}}
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		data, err := os.ReadFile(files[rel])
		if err != nil {
			return 0, 0, err
		}
		info, err := os.Stat(files[rel])
		if err != nil {
			return 0, 0, err
		}
		dest := filepath.Join(target, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return 0, 0, err
		}
		temporary, err := os.CreateTemp(filepath.Dir(dest), ".henia-install-*")
		if err != nil {
			return 0, 0, err
		}
		_, err = temporary.Write(data)
		if closeErr := temporary.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Chmod(temporary.Name(), info.Mode().Perm())
		}
		if err == nil {
			err = os.Rename(temporary.Name(), dest)
		}
		if err != nil {
			os.Remove(temporary.Name())
			return 0, 0, err
		}
		record.Files[rel] = hash(data)
	}
	removed := 0
	for _, rel := range slices.Sorted(maps.Keys(previous.Files)) {
		if _, kept := files[rel]; kept {
			continue
		}
		dest := filepath.Join(target, rel)
		current, err := os.ReadFile(dest)
		if err != nil || hash(current) != previous.Files[rel] {
			continue
		}
		if err := os.Remove(dest); err != nil {
			return 0, 0, err
		}
		removed++
		for dir := filepath.Dir(dest); dir != target && strings.HasPrefix(dir, target); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		return 0, 0, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return 0, 0, err
	}
	return len(files), removed, os.WriteFile(manifest, data, 0o644)
}
