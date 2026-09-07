package config

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/lint"
	"github.com/srnnkls/henia/internal/vendor"
)

type Config struct {
	Lint      lint.Options             `toml:"lint,omitempty"`
	Artifacts []string                 `toml:"artifacts,omitempty"`
	Build     BuildOptions             `toml:"build,omitempty"`
	Harness   map[string]henia.Harness `toml:"harness,omitempty"`
}

type BuildOptions struct {
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

func loadLayers(path string, projectData []byte) (*Config, error) {
	projectRoot, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	userRoot := filepath.Join(home, ".config", "henia")
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
		if _, ok := (*layer.out)["sources"]; ok {
			return nil, fmt.Errorf("source fetching belongs to Phora; remove [sources] from henia.toml and pass a local directory to henia build")
		}
		if harnesses, ok := (*layer.out)["harness"].(map[string]any); ok {
			for name, value := range harnesses {
				if harness, ok := value.(map[string]any); ok {
					if _, exists := harness["path"]; exists {
						return nil, fmt.Errorf("harness.%s.path is a deployment setting; use [build].output for artifacts and configure final destinations in Phora", name)
					}
				}
			}
		}
	}
	// Explicit configurations select their harnesses. Merge each selected harness
	// with its preset when harnesses are explicitly selected.
	userHarnesses, _ := user["harness"].(map[string]any)
	projectHarnesses, _ := project["harness"].(map[string]any)
	if len(userHarnesses) > 0 || len(projectHarnesses) > 0 {
		delete(base, "artifacts")
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
		// Existing explicit configurations remain on the legacy mapper until they
		// opt into a profile. New builds without configuration use all vendor profiles.
		for name, value := range selected {
			preset := maps.Clone(value.(map[string]any))
			explicit := false
			for _, layer := range []map[string]any{user, project} {
				if harnesses, ok := layer["harness"].(map[string]any); ok {
					if harness, ok := harnesses[name].(map[string]any); ok {
						if _, ok := harness["profile"]; ok {
							explicit = true
						}
					}
				}
			}
			if !explicit {
				preset = map[string]any{}
			}
			selected[name] = preset
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
		return nil, err
	}
	if cfg.Build.Output == "" {
		cfg.Build.Output = ".henia/build"
	}
	if cfg.Harness == nil {
		cfg.Harness = make(map[string]henia.Harness)
	}
	for name, h := range cfg.Harness {
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
		if h.Profile != "" && h.Structure == "flat" {
			return nil, fmt.Errorf("harness %s: skill profiles require nested structure", name)
		}
		if h.Format != "" && h.Format != "directives" && h.Format != "xml" && h.Format != "markdown" {
			return nil, fmt.Errorf("harness %s: unsupported format %q", name, h.Format)
		}
		if h.Structure != "" && h.Structure != "flat" && h.Structure != "nested" {
			return nil, fmt.Errorf("harness %s: unsupported structure %q", name, h.Structure)
		}
	}
	if err := cfg.Lint.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
