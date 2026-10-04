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
			if len(a.Static) > 0 && len(a.Dynamic) > 0 {
				return fmt.Errorf("harness %s: %s.static and %s.dynamic are both set; keep one", name, kind, kind)
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
