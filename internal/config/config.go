package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/deps"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/vendor"
)

type Config struct {
	Lint      lint.Options               `toml:"lint,omitempty"`
	Artifacts []string                   `toml:"artifacts,omitempty"`
	Build     BuildOptions               `toml:"build,omitempty"`
	Harness   map[string]henia.Harness   `toml:"harness,omitempty"`
	Preload   preload.Settings           `toml:"preload,omitempty"`
	Resources ResourceOptions            `toml:"resources,omitempty"`
	Depends   map[string]deps.Dependency `toml:"dependencies,omitempty"`
	Slots     map[string]SlotOptions     `toml:"slots,omitempty"`
}

type SlotOptions struct {
	Disable []string `toml:"disable,omitempty"`
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

func loadLayers(path string, projectData []byte) (*Config, error) {
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
	return decodeLayers(projectRoot, userRoot, userData, projectData)
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
	}
	merged := vendor.Merge(vendor.Merge(base, user), project)
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
		h.ProjectRoot, h.UserRoot = projectRoot, userRoot
		cfg.Harness[name] = h
		if h.Profile != "" {
			profile, err := vendor.Load(h.Profile, projectRoot, userRoot)
			if err == nil {
				_, err = vendor.NewCompiler(profile)
			}
			if err != nil {
				return nil, fmt.Errorf("harness %s: %w", name, err)
			}
		}
		if h.Profile != "" && h.Layout == "flat" {
			return nil, fmt.Errorf("harness %s: skill profiles require the nested layout", name)
		}
	}
	if err := cfg.Lint.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
