package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/deps"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/profile"
)

type Config struct {
	Lint              lint.Options               `toml:"lint,omitempty"`
	Artifacts         []string                   `toml:"artifacts,omitempty"`
	Shared            map[string]any             `toml:"shared,omitempty"`
	Build             BuildOptions               `toml:"build,omitempty"`
	Harness           map[string]henia.Harness   `toml:"harness,omitempty"`
	Preload           preload.Settings           `toml:"preload,omitempty"`
	Resources         ResourceOptions            `toml:"resources,omitempty"`
	Depends           map[string]deps.Dependency `toml:"dependencies,omitempty"`
	Slots             map[string]SlotOptions     `toml:"slots,omitempty"`
	Install           map[string]InstallTarget   `toml:"install,omitempty"`
	InstallHeniaSkill *bool                      `toml:"install_henia_skill,omitempty"`
	Selected          []string                   `toml:"-"`
	Path              string                     `toml:"-"`
}

type InstallTarget struct {
	Path    string `toml:"path,omitempty"`
	Enabled *bool  `toml:"enabled,omitempty"`
}

func (c *Config) HeniaSkill() bool { return c.InstallHeniaSkill == nil || *c.InstallHeniaSkill }

type SlotOptions struct {
	Disable []string `toml:"disable,omitempty"`
	Provide []string `toml:"provide,omitempty"`
}

func (c *Config) DisabledProviders() map[string][]string {
	disabled := map[string][]string{}
	for slot, options := range c.Slots {
		if len(options.Disable) > 0 {
			disabled[slot] = options.Disable
		}
	}
	return disabled
}

func (c *Config) ProvidedSkills() map[string][]string {
	provided := map[string][]string{}
	for slot, options := range c.Slots {
		if len(options.Provide) > 0 {
			provided[slot] = options.Provide
		}
	}
	return provided
}

type ResourceOptions struct {
	Disclosure *bool `toml:"disclosure,omitempty"`
}

type BuildOptions struct {
	Clean  bool   `toml:"clean,omitempty"`
	Output string `toml:"output,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadLayers(path, data)
}

func LoadOptional(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return loadLayers(path, data)
}

func Find(dir string) (string, error) {
	var found []string
	for _, candidate := range []string{filepath.Join(dir, "henia.toml"), filepath.Join(dir, ".henia", "henia.toml")} {
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, candidate)
		}
	}
	switch len(found) {
	case 0:
		return filepath.Join(dir, "henia.toml"), os.ErrNotExist
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("both %s and %s configure the project; keep one", found[0], found[1])
}

// HarnessTables returns the harness tables that the file at path and its
// henia.local.toml declare, without the bundled presets or the user layer.
func HarnessTables(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if data, err = withLocal(path, data); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	tables, _ := raw["harness"].(map[string]any)
	root, err := layerRoot(path)
	if err != nil {
		return nil, err
	}
	if err := resolveHarnessFiles(tables, root); err != nil {
		return nil, err
	}
	return tables, nil
}

// InstallTables returns the install tables that the file at path and its
// henia.local.toml declare, without the user layer.
func InstallTables(path string) (map[string]InstallTarget, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if data, err = withLocal(path, data); err != nil {
		return nil, err
	}
	var raw struct {
		Install map[string]InstallTarget `toml:"install"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return raw.Install, nil
}

// LoadPackage loads a skill package's henia.toml with the user layer merged
// over it, then the given harness tables over the harnesses the package
// configures itself. home keeps the supporting files only the user layer
// lists, for an install into the user's harness homes.
func LoadPackage(path string, harnesses map[string]any, home bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if data, err = withLocal(path, data); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	own, _ := raw["harness"].(map[string]any)
	overrides := map[string]any{}
	for name, table := range harnesses {
		if _, ok := own[name]; ok {
			overrides[name] = table
		}
	}
	overrideData, err := toml.Marshal(map[string]any{"harness": overrides})
	if err != nil {
		return nil, err
	}
	root, err := layerRoot(path)
	if err != nil {
		return nil, err
	}
	userRoot := library.ConfigDir()
	userData, err := userLayer(userRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := decode(root, userRoot, []layer{{data, root, false}, {userData, userRoot, true}, {overrideData, root, false}}, home)
	if err != nil {
		return nil, err
	}
	cfg.Path = path
	return cfg, nil
}

func loadLayers(path string, projectData []byte) (*Config, error) {
	projectData, err := withLocal(path, projectData)
	if err != nil {
		return nil, err
	}
	return decodeFile(path, projectData)
}

func decodeFile(path string, projectData []byte) (*Config, error) {
	projectRoot, err := layerRoot(path)
	if err != nil {
		return nil, err
	}
	userRoot := library.ConfigDir()
	userData, err := userLayer(userRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := decodeLayers(projectRoot, userRoot, userData, projectData)
	if err != nil {
		return nil, err
	}
	cfg.Path = path
	return cfg, nil
}

// userLayer is the user henia.toml with its henia.local.toml layered over it.
func userLayer(userRoot string) ([]byte, error) {
	path := filepath.Join(userRoot, "henia.toml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return withLocal(path, data)
}

func withLocal(path string, projectData []byte) ([]byte, error) {
	localData, err := os.ReadFile(deps.Local(path))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(localData) == 0 {
		return projectData, nil
	}
	var project, local map[string]any
	if err := toml.Unmarshal(projectData, &project); err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(localData, &local); err != nil {
		return nil, fmt.Errorf("%s: %w", deps.Local(path), err)
	}
	root, err := layerRoot(path)
	if err != nil {
		return nil, err
	}
	return toml.Marshal(mergeLayer(project, local, layerExtensions(root, library.ConfigDir())))
}

// layerRoot is the directory a henia.toml's relative paths resolve against:
// its own, or the project's for one inside .henia.
func layerRoot(path string) (string, error) {
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	if filepath.Base(root) == ".henia" {
		root = filepath.Dir(root)
	}
	return root, nil
}

// resolveHarnessFiles resolves the files the harness tables of one layer point
// to against that layer's root.
func resolveHarnessFiles(harnesses map[string]any, root string) error {
	for name, value := range harnesses {
		harness, _ := value.(map[string]any)
		config, _ := harness["config"].(map[string]any)
		statusline, _ := config["statusline"].(map[string]any)
		if path, ok := statusline["file"].(string); ok {
			resolved, err := layerFile(root, path)
			if err != nil {
				return fmt.Errorf("harness.%s.config.statusline.file: %w", name, err)
			}
			statusline["file"] = resolved
		}
	}
	return nil
}

// userFileRoots resolves the supporting files the user layer declares, and no
// later layer redeclares, in the user layer's directory for a harness home
// install, and drops them from any other build.
func userFileRoots(cfg *Config, user, project map[string]any, root string, home bool) {
	for name, value := range user {
		harness, _ := value.(map[string]any)
		files, _ := harness["files"].(map[string]any)
		projectHarness, _ := project[name].(map[string]any)
		projectFiles, _ := projectHarness["files"].(map[string]any)
		for path := range files {
			if _, ok := projectFiles[path]; ok {
				continue
			}
			file, ok := cfg.Harness[name].Files[path]
			if !ok {
				continue
			}
			if !home {
				delete(cfg.Harness[name].Files, path)
				continue
			}
			file.Root = root
			cfg.Harness[name].Files[path] = file
		}
	}
}

// layerFile resolves a file a layer points to against that layer's root.
func layerFile(root, path string) (string, error) {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, rest), nil
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(root, path), nil
}

// extender returns the native paths of a harness whose arrays later layers
// extend, given the harness's merged table.
type extender func(harness string, table map[string]any) []string

// layerExtensions resolves each harness's profile, the preset's when the
// layers name none, for the native paths its config files extend.
func layerExtensions(projectRoot, userRoot string) extender {
	return func(harness string, table map[string]any) []string {
		name, _ := table["profile"].(string)
		if name == "" {
			name = presetProfiles()[harness]
		}
		if name == "" {
			return nil
		}
		loaded, err := profile.Load(name, projectRoot, userRoot)
		if err != nil {
			return nil
		}
		return loaded.Config.Extensions()
	}
}

var presetProfiles = sync.OnceValue(func() map[string]string {
	var presets struct {
		Harness map[string]struct {
			Profile string `toml:"profile"`
		} `toml:"harness"`
	}
	_ = toml.Unmarshal([]byte(defaults.ConfigTOML), &presets)
	profiles := map[string]string{}
	for name, h := range presets.Harness {
		profiles[name] = h.Profile
	}
	return profiles
})

// mergeLayer merges a later henia.toml layer. Arrays under artifacts,
// [shared] and harness config replace earlier ones, except the native arrays
// the harness's profile extends, which keep the earlier items and add the later
// ones not already present; a harness status line replaces the earlier one
// whole.
func mergeLayer(base, override map[string]any, extend extender) map[string]any {
	merged := profile.Merge(base, override)
	if artifacts, ok := override["artifacts"]; ok {
		merged["artifacts"] = artifacts
	}
	if shared, ok := override["shared"].(map[string]any); ok {
		left, _ := base["shared"].(map[string]any)
		merged["shared"] = profile.Replace(left, shared)
	}
	harnesses, _ := override["harness"].(map[string]any)
	if len(harnesses) == 0 {
		return merged
	}
	all, _ := merged["harness"].(map[string]any)
	all = maps.Clone(all)
	merged["harness"] = all
	for name, value := range harnesses {
		right, _ := value.(map[string]any)
		before, _ := base["harness"].(map[string]any)
		left, _ := before[name].(map[string]any)
		target, _ := all[name].(map[string]any)
		if right == nil || target == nil {
			continue
		}
		target = maps.Clone(target)
		all[name] = target
		if artifacts, ok := right["artifacts"]; ok {
			target["artifacts"] = artifacts
		}
		if config, ok := right["config"].(map[string]any); ok {
			before, _ := left["config"].(map[string]any)
			replaced := profile.Replace(before, config)
			if statusline, ok := config["statusline"]; ok {
				replaced["statusline"] = statusline
			}
			if extend != nil {
				left, _ := before["native"].(map[string]any)
				right, _ := config["native"].(map[string]any)
				merged, _ := replaced["native"].(map[string]any)
				for _, path := range extend(name, target) {
					extendArrays(merged, left, right, strings.Split(path, "."))
				}
			}
			target["config"] = replaced
		}
	}
	return merged
}

// extendArrays sets the array at path in merged, or each array of the table
// at path, to the left items followed by the right items not among them.
// merged holds fresh tables wherever both sides do.
func extendArrays(merged, left, right map[string]any, path []string) {
	for _, key := range path[:len(path)-1] {
		merged, _ = merged[key].(map[string]any)
		left, _ = left[key].(map[string]any)
		right, _ = right[key].(map[string]any)
	}
	key := path[len(path)-1]
	if merged == nil {
		return
	}
	if l, ok := left[key].(map[string]any); ok {
		if r, ok := right[key].(map[string]any); ok {
			table, _ := merged[key].(map[string]any)
			for name := range r {
				extendArrays(table, l, r, []string{name})
			}
		}
		return
	}
	l, lok := left[key].([]any)
	r, rok := right[key].([]any)
	if !lok || !rok {
		return
	}
	items := slices.Clone(l)
	for _, item := range r {
		if !slices.ContainsFunc(items, func(have any) bool { return reflect.DeepEqual(have, item) }) {
			items = append(items, item)
		}
	}
	merged[key] = items
}

// layer is one henia.toml above the bundled defaults, with the directory its
// relative paths resolve against.
type layer struct {
	data []byte
	root string
	user bool
}

func decodeLayers(projectRoot, userRoot string, userData, projectData []byte) (*Config, error) {
	return decode(projectRoot, userRoot, []layer{{userData, userRoot, true}, {projectData, projectRoot, false}}, false)
}

// decode merges the layers in order over the bundled defaults.
func decode(projectRoot, userRoot string, layers []layer, home bool) (*Config, error) {
	var base map[string]any
	if err := toml.Unmarshal([]byte(defaults.ConfigTOML), &base); err != nil {
		return nil, err
	}
	values := make([]map[string]any, len(layers))
	for i, l := range layers {
		if err := toml.Unmarshal(l.data, &values[i]); err != nil {
			return nil, err
		}
	}
	// Explicit configurations select their harnesses. Merge each selected harness
	// with its preset when harnesses are explicitly selected.
	var explicit []string
	var userHarnesses map[string]any
	later := map[string]any{}
	for i, l := range layers {
		harnesses, _ := values[i]["harness"].(map[string]any)
		for name := range harnesses {
			if !slices.Contains(explicit, name) {
				explicit = append(explicit, name)
			}
		}
		if l.user {
			userHarnesses, later = harnesses, map[string]any{}
		} else if userHarnesses != nil {
			later = profile.Merge(later, harnesses)
		}
	}
	slices.Sort(explicit)
	if len(explicit) > 0 {
		selected := map[string]any{}
		for _, name := range explicit {
			if value, ok := base["harness"].(map[string]any)[name]; ok {
				selected[name] = maps.Clone(value.(map[string]any))
			}
		}
		base["harness"] = selected
	}
	// Resolve filesystem settings in the layer that declares them, so defaults do
	// not change meaning with the project or invocation working directory.
	for i, l := range layers {
		lintConfig, _ := values[i]["lint"].(map[string]any)
		buildConfig, _ := values[i]["build"].(map[string]any)
		if path, ok := buildConfig["output"].(string); ok && path != "" && !filepath.IsAbs(path) {
			buildConfig["output"] = filepath.Join(l.root, path)
		}
		semantic, _ := lintConfig["semantic"].(map[string]any)
		if path, ok := semantic["model_path"].(string); ok && path != "" && !filepath.IsAbs(path) {
			semantic["model_path"] = filepath.Join(l.root, path)
		}
		harnesses, _ := values[i]["harness"].(map[string]any)
		if err := resolveHarnessFiles(harnesses, l.root); err != nil {
			return nil, err
		}
	}
	extend := layerExtensions(projectRoot, userRoot)
	merged := base
	for _, value := range values {
		merged = mergeLayer(merged, value, extend)
	}
	data, err := toml.Marshal(merged)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&cfg); err != nil {
		var unknown *toml.StrictMissingError
		if errors.As(err, &unknown) {
			var keys []string
			for _, e := range unknown.Errors {
				keys = append(keys, strings.Join(e.Key(), "."))
			}
			return nil, fmt.Errorf("unknown henia.toml keys: %s", strings.Join(keys, ", "))
		}
		return nil, err
	}
	cfg.Selected = explicit
	userFileRoots(&cfg, userHarnesses, later, userRoot, home)
	if cfg.Build.Output == "" {
		cfg.Build.Output = ".henia/build"
	}
	if cfg.Harness == nil {
		cfg.Harness = make(map[string]henia.Harness)
	}
	for name, h := range cfg.Harness {
		if err := validateHarness(name, h); err != nil {
			return nil, err
		}
		h.ProjectRoot, h.UserRoot, h.Shared = projectRoot, userRoot, cfg.Shared
		h, err := resolveProfiles(name, h)
		if err != nil {
			return nil, err
		}
		cfg.Harness[name] = h
	}
	if err := cfg.Lint.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
