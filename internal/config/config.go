package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

// LoadPackage loads a skill package's henia.toml with the given harness tables
// merged over the harnesses the package configures itself.
func LoadPackage(path string, harnesses map[string]any) (*Config, error) {
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
	if data, err = toml.Marshal(mergeLayer(raw, map[string]any{"harness": overrides})); err != nil {
		return nil, err
	}
	return decodeFile(path, data)
}

func loadLayers(path string, projectData []byte) (*Config, error) {
	projectData, err := withLocal(path, projectData)
	if err != nil {
		return nil, err
	}
	return decodeFile(path, projectData)
}

func decodeFile(path string, projectData []byte) (*Config, error) {
	projectRoot, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if filepath.Base(projectRoot) == ".henia" {
		projectRoot = filepath.Dir(projectRoot)
	}
	userRoot := library.ConfigDir()
	userData, err := os.ReadFile(filepath.Join(userRoot, "henia.toml"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	cfg, err := decodeLayers(projectRoot, userRoot, userData, projectData)
	if err != nil {
		return nil, err
	}
	cfg.Path = path
	return cfg, nil
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
	return toml.Marshal(mergeLayer(project, local))
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

// mergeLayer merges a later henia.toml layer. Arrays under artifacts,
// [shared] and harness config replace earlier ones; a harness status line
// replaces the earlier one whole.
func mergeLayer(base, override map[string]any) map[string]any {
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
			target["config"] = replaced
		}
	}
	return merged
}

func decodeLayers(projectRoot, userRoot string, userData, projectData []byte) (*Config, error) {
	var base, user, project map[string]any
	for _, layer := range []struct {
		data []byte
		out  *map[string]any
	}{
		{[]byte(defaults.ConfigTOML), &base}, {userData, &user}, {projectData, &project},
	} {
		if err := toml.Unmarshal(layer.data, layer.out); err != nil {
			return nil, err
		}
	}
	// Explicit configurations select their harnesses. Merge each selected harness
	// with its preset when harnesses are explicitly selected.
	userHarnesses, _ := user["harness"].(map[string]any)
	projectHarnesses, _ := project["harness"].(map[string]any)
	var explicit []string
	for name := range userHarnesses {
		explicit = append(explicit, name)
	}
	for name := range projectHarnesses {
		if _, ok := userHarnesses[name]; !ok {
			explicit = append(explicit, name)
		}
	}
	slices.Sort(explicit)
	if len(userHarnesses) > 0 || len(projectHarnesses) > 0 {
		selected := map[string]any{}
		for _, layer := range []map[string]any{user, project} {
			if harnesses, ok := layer["harness"].(map[string]any); ok {
				for name := range harnesses {
					if value, ok := base["harness"].(map[string]any)[name]; ok {
						selected[name] = value
					}
				}
			}
		}
		base["harness"] = selected
		for name, value := range selected {
			selected[name] = maps.Clone(value.(map[string]any))
		}
	}
	// Resolve filesystem settings in the layer that declares them, so defaults do
	// not change meaning with the project or invocation working directory.
	for _, layer := range []struct {
		values map[string]any
		root   string
	}{{user, userRoot}, {project, projectRoot}} {
		lintConfig, _ := layer.values["lint"].(map[string]any)
		buildConfig, _ := layer.values["build"].(map[string]any)
		if path, ok := buildConfig["output"].(string); ok && path != "" && !filepath.IsAbs(path) {
			buildConfig["output"] = filepath.Join(layer.root, path)
		}
		semantic, _ := lintConfig["semantic"].(map[string]any)
		if path, ok := semantic["model_path"].(string); ok && path != "" && !filepath.IsAbs(path) {
			semantic["model_path"] = filepath.Join(layer.root, path)
		}
		harnesses, _ := layer.values["harness"].(map[string]any)
		for name, value := range harnesses {
			harness, _ := value.(map[string]any)
			config, _ := harness["config"].(map[string]any)
			statusline, _ := config["statusline"].(map[string]any)
			if path, ok := statusline["file"].(string); ok {
				resolved, err := layerFile(layer.root, path)
				if err != nil {
					return nil, fmt.Errorf("harness.%s.config.statusline.file: %w", name, err)
				}
				statusline["file"] = resolved
			}
		}
	}
	merged := mergeLayer(mergeLayer(base, user), project)
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
