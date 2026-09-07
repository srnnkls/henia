package config

import (
	"fmt"
	"github.com/srnnkls/henia/internal/lint"
	"os"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/phora"
)

type Config struct {
	Lint      lint.Options             `toml:"lint,omitempty"`
	Artifacts []string                 `toml:"artifacts,omitempty"`
	Sources   map[string]phora.Source  `toml:"sources,omitempty"`
	Harness   map[string]henia.Harness `toml:"harness,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Sources == nil {
		cfg.Sources = make(map[string]phora.Source)
	}
	if cfg.Harness == nil {
		cfg.Harness = make(map[string]henia.Harness)
	}

	for name, h := range cfg.Harness {
		if h.Format != "" && h.Format != "directives" && h.Format != "xml" {
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
