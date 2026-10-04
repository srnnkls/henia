package config

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia"
)

func migrateHarnesses(layer map[string]any) ([]string, error) {
	harnesses, _ := layer["harness"].(map[string]any)
	var warnings []string
	for _, name := range sortedKeys(harnesses) {
		harness, ok := harnesses[name].(map[string]any)
		if !ok {
			continue
		}
		renamed, err := migrateHarness(name, harness)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, renamed...)
	}
	return warnings, nil
}

func migrateHarness(name string, h map[string]any) ([]string, error) {
	var warnings []string
	move := func(table map[string]any, prefix, old string, path ...string) error {
		value, ok := table[old]
		if !ok {
			return nil
		}
		target := table
		for _, key := range path[:len(path)-1] {
			next, ok := target[key].(map[string]any)
			if !ok {
				next = map[string]any{}
				target[key] = next
			}
			target = next
		}
		last := path[len(path)-1]
		if _, exists := target[last]; exists {
			return fmt.Errorf("harness.%s: %s%s and its replacement %s%s are both set", name, prefix, old, prefix, joinPath(path))
		}
		target[last] = value
		delete(table, old)
		warnings = append(warnings, fmt.Sprintf("harness.%s: %s%s is deprecated; use %s%s", name, prefix, old, prefix, joinPath(path)))
		return nil
	}
	steps := []struct {
		old  string
		path []string
	}{
		{"format", []string{"directives"}},
		{"structure", []string{"layout"}},
		{"keys", []string{"frontmatter", "rename"}},
		{"values", []string{"frontmatter", "values"}},
	}
	for _, step := range steps {
		if err := move(h, "", step.old, step.path...); err != nil {
			return nil, err
		}
	}
	for old, list := range map[string]string{"include": "static", "exclude": "dynamic"} {
		names, ok := h[old]
		if !ok {
			continue
		}
		for _, kind := range []string{"skills", "agents", "commands"} {
			table, _ := h[kind].(map[string]any)
			if table == nil {
				table = map[string]any{}
				h[kind] = table
			}
			if _, exists := table[list]; exists {
				return nil, fmt.Errorf("harness.%s: %s and its replacement %s.%s are both set", name, old, kind, list)
			}
			table[list] = names
		}
		delete(h, old)
		warnings = append(warnings, fmt.Sprintf("harness.%s: %s is deprecated; use skills.%s (and agents.%s or commands.%s)", name, old, list, list, list))
	}
	if h["directives"] == "directives" {
		h["directives"] = "keep"
	}
	if refs, ok := h["references"].(map[string]any); ok {
		for _, kind := range sortedKeys(refs) {
			if table, ok := refs[kind].(map[string]any); ok {
				refs[kind] = table["output"]
				warnings = append(warnings, fmt.Sprintf("harness.%s: references.%s.output is deprecated; use references.%s", name, kind, kind))
			}
		}
	}
	if mappings, ok := h["artifact_mappings"].(map[string]any); ok {
		for _, kind := range sortedKeys(mappings) {
			mapping, ok := mappings[kind].(map[string]any)
			if !ok {
				continue
			}
			target, _ := h[kind].(map[string]any)
			if target == nil {
				target = map[string]any{}
				h[kind] = target
			}
			renames := map[string][]string{"structure": {"layout"}, "keys": {"frontmatter", "rename"}, "values": {"frontmatter", "values"}}
			for _, key := range sortedKeys(mapping) {
				path, renamed := renames[key]
				if !renamed {
					path = []string{key}
				}
				parent := target
				for _, segment := range path[:len(path)-1] {
					next, ok := parent[segment].(map[string]any)
					if !ok {
						next = map[string]any{}
						parent[segment] = next
					}
					parent = next
				}
				if _, exists := parent[path[len(path)-1]]; exists {
					return nil, fmt.Errorf("harness.%s: artifact_mappings.%s.%s and %s.%s are both set", name, kind, key, kind, joinPath(path))
				}
				parent[path[len(path)-1]] = mapping[key]
				warnings = append(warnings, fmt.Sprintf("harness.%s: artifact_mappings.%s.%s is deprecated; use %s.%s", name, kind, key, kind, joinPath(path)))
			}
		}
		delete(h, "artifact_mappings")
	}
	return warnings, nil
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

func Harnesses(data []byte) (map[string]henia.Harness, []string, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	warnings, err := migrateHarnesses(raw)
	if err != nil {
		return nil, nil, err
	}
	encoded, err := toml.Marshal(map[string]any{"harness": raw["harness"]})
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		Harness map[string]henia.Harness `toml:"harness"`
	}
	if err := toml.NewDecoder(bytes.NewReader(encoded)).Decode(&parsed); err != nil {
		return nil, nil, err
	}
	return parsed.Harness, warnings, nil
}

func joinPath(path []string) string { return strings.Join(path, ".") }

func sortedKeys(m map[string]any) []string { return slices.Sorted(maps.Keys(m)) }
