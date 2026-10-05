package config

import (
	"bytes"
	"fmt"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
)

func validateHarness(name string, h henia.Harness) error {
	switch h.Directives {
	case "", "xml", "markdown", "md", "keep":
	default:
		return fmt.Errorf("harness %s: directives must be xml, markdown, md or keep, got %q", name, h.Directives)
	}
	for _, kind := range []string{"", "skills", "agents", "commands"} {
		layout, label := h.Layout, "layout"
		if kind != "" {
			a := h.Type(kind)
			switch a.Default {
			case "", henia.Static, henia.Dynamic, henia.Hybrid:
			default:
				return fmt.Errorf("harness %s: %s.default must be static, dynamic or hybrid, got %q", name, kind, a.Default)
			}
			seen := map[string]string{}
			for _, mode := range []struct {
				name  string
				items []string
			}{{henia.Static, a.Static}, {henia.Dynamic, a.Dynamic}, {henia.Hybrid, a.Hybrid}} {
				for _, item := range mode.items {
					if other, ok := seen[item]; ok && other != mode.name {
						return fmt.Errorf("harness %s: %s %q is listed as both %s and %s", name, kind, item, other, mode.name)
					}
					seen[item] = mode.name
				}
			}
			if len(a.Hybrid) > 0 && kind != "skills" {
				return fmt.Errorf("harness %s: hybrid applies to skills only", name)
			}
			if a.Catalog != nil && kind != "skills" {
				return fmt.Errorf("harness %s: catalog applies to skills only", name)
			}
			layout, label = a.Layout, kind+".layout"
		}
		if layout != "" && layout != "flat" && layout != "nested" {
			return fmt.Errorf("harness %s: %s must be flat or nested, got %q", name, label, layout)
		}
	}
	return nil
}

func Harnesses(data []byte) (map[string]henia.Harness, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	encoded, err := toml.Marshal(map[string]any{"harness": raw["harness"]})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Harness map[string]henia.Harness `toml:"harness"`
	}
	if err := toml.NewDecoder(bytes.NewReader(encoded)).DisallowUnknownFields().Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Harness, nil
}
