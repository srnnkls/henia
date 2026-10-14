package config

import (
	"bytes"
	"fmt"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/profile"
)

// resolveProfiles loads and compiles every profile a harness names, so a
// broken per-type profile fails at load rather than mid-build, and fills
// harness-wide defaults the configuration leaves unset.
func resolveProfiles(name string, h henia.Harness) (henia.Harness, error) {
	type use struct {
		profile, kind, layout string
		perType               bool
	}
	uses := []use{{h.Profile, "", h.Layout, false}}
	for _, kind := range []string{"skills", "agents", "commands"} {
		a := h.Type(kind)
		layout := a.Layout
		if layout == "" {
			layout = h.Layout
		}
		profile := a.Profile
		if profile == "" {
			profile = h.Profile
		}
		uses = append(uses, use{profile, kind, layout, a.Profile != ""})
	}
	for _, u := range uses {
		if u.profile == "" {
			continue
		}
		loaded, err := profile.Load(u.profile, h.ProjectRoot, h.UserRoot)
		if err != nil {
			return h, fmt.Errorf("harness %s: %w", name, err)
		}
		if u.kind == "" {
			if h.Directives == "" {
				h.Directives = loaded.Directives
			}
			if _, err := profile.NewConfigCompiler(loaded.Config); err != nil {
				return h, fmt.Errorf("harness %s: profile %s: %w", name, u.profile, err)
			}
			continue
		}
		section := loaded.Section(u.kind[:len(u.kind)-1], u.perType)
		if section == nil {
			continue
		}
		if _, err := profile.NewCompiler(*section); err != nil {
			return h, fmt.Errorf("harness %s: profile %s: %w", name, u.profile, err)
		}
		if u.kind == "skills" && u.layout == "flat" {
			return h, fmt.Errorf("harness %s: skill profiles require the nested layout", name)
		}
	}
	return h, nil
}

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
