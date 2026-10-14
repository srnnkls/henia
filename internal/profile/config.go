package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/expr-lang/expr/vm"
	"github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia/internal/expression"
	"gopkg.in/yaml.v3"
)

// MainFile receives the native keys that name no other config file.
const MainFile = "main"

type Config struct {
	Files map[string]ConfigFile `toml:"files"`
}

type ConfigFile struct {
	Path   string `toml:"path"` // Relative to harness root.
	Format string `toml:"format"`
	Value  string `toml:"value"`  // Expr expression; optional when native data fills the file.
	Schema string `toml:"schema"` // JSON Schema URL.
	Arrays string `toml:"arrays"` // How native arrays combine with generated ones: replace (default) or concat.
}

type configEnvironment struct {
	Shared  map[string]any                                      `expr:"shared"`
	Context Context                                             `expr:"ctx"`
	Merge   func(map[string]any, map[string]any) map[string]any `expr:"merge"`
	Require func(any, string) (any, error)                      `expr:"require"`
}

type ConfigCompiler struct {
	config Config
	values map[string]*vm.Program
}

func NewConfigCompiler(config Config) (*ConfigCompiler, error) {
	c := &ConfigCompiler{config: config, values: map[string]*vm.Program{}}
	paths := map[string]string{}
	for _, id := range slices.Sorted(maps.Keys(config.Files)) {
		file := config.Files[id]
		switch file.Format {
		case "yaml", "json", "toml", "text":
		default:
			return nil, fmt.Errorf("config file %s: format must be yaml, json, toml or text", id)
		}
		switch file.Arrays {
		case "", "replace", "concat":
		default:
			return nil, fmt.Errorf("config file %s: arrays must be replace or concat", id)
		}
		if err := validPath(file.Path); err != nil {
			return nil, fmt.Errorf("config file %s: %w", id, err)
		}
		if other, ok := paths[file.Path]; ok {
			return nil, fmt.Errorf("config files %s and %s both write %s", other, id, file.Path)
		}
		paths[file.Path] = id
		if file.Value == "" {
			continue
		}
		program, err := expression.Compile(file.Value, configEnvironment{}, false)
		if err != nil {
			return nil, fmt.Errorf("config file %s: %w", id, err)
		}
		c.values[id] = program
	}
	return c, nil
}

// Render evaluates every config file and merges this harness's native data
// over it. A file whose result is empty is not emitted.
func (c *ConfigCompiler) Render(shared, native map[string]any, context Context, schemas *Schemas) (*Result, error) {
	r := &Result{Files: map[string][]byte{}}
	natives, err := c.natives(native)
	if err != nil {
		return nil, err
	}
	env := configEnvironment{shared, context, Merge, require}
	for _, id := range slices.Sorted(maps.Keys(c.config.Files)) {
		file := c.config.Files[id]
		var value any
		if program, ok := c.values[id]; ok {
			if value, err = expression.Run(program, env); err != nil {
				return nil, fmt.Errorf("config file %s: %w", id, err)
			}
		}
		value = pruned(value)
		if file.Format == "text" {
			if natives[id] != nil {
				return nil, fmt.Errorf("config file %s: text files take no native data", id)
			}
		} else if extra := natives[id]; len(extra) > 0 {
			generated, ok := value.(map[string]any)
			if value != nil && !ok {
				return nil, fmt.Errorf("config file %s: value must be a table to take native data", id)
			}
			if file.Arrays == "concat" {
				value = Merge(generated, extra)
			} else {
				value = Replace(generated, extra)
			}
		}
		if value == nil || value == "" {
			continue
		}
		if object, ok := value.(map[string]any); ok && len(object) == 0 {
			continue
		}
		if file.Schema != "" && schemas != nil {
			if err := schemas.Validate(file.Schema, value); err != nil {
				r.Warnings = append(r.Warnings, fmt.Sprintf("config file %s: %v", file.Path, err))
			}
		}
		data, err := encode(file, value)
		if err != nil {
			return nil, fmt.Errorf("config file %s: %w", id, err)
		}
		r.Files[file.Path] = data
	}
	return r, nil
}

// natives splits native data by file: a key naming a config file addresses
// that file, and the remaining keys address the main file.
func (c *ConfigCompiler) natives(native map[string]any) (map[string]map[string]any, error) {
	result := map[string]map[string]any{}
	rest := map[string]any{}
	for key, value := range native {
		if _, ok := c.config.Files[key]; !ok || key == MainFile {
			rest[key] = value
			continue
		}
		table, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config.native.%s must be a table", key)
		}
		result[key] = table
	}
	if main, ok := rest[MainFile].(map[string]any); ok {
		delete(rest, MainFile)
		rest = Merge(rest, main)
	}
	if len(rest) > 0 {
		if _, ok := c.config.Files[MainFile]; !ok {
			return nil, fmt.Errorf("config.native keys %v name no config file", slices.Sorted(maps.Keys(rest)))
		}
		result[MainFile] = rest
	}
	return result, nil
}

func require(value any, message string) (any, error) {
	if value == nil || value == "" {
		return nil, fmt.Errorf("%s", message)
	}
	return value, nil
}

// pruned drops nil table entries and array items, so absent shared data
// leaves no empty keys.
// Tables built by fromPairs arrive with untyped keys.
func pruned(value any) any {
	var object map[string]any
	switch v := value.(type) {
	case map[string]any:
		object = v
	case map[any]any:
		object = make(map[string]any, len(v))
		for key, item := range v {
			object[fmt.Sprint(key)] = item
		}
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			if item = pruned(item); item != nil {
				items = append(items, item)
			}
		}
		return items
	default:
		return value
	}
	result := map[string]any{}
	for key, item := range object {
		item = pruned(item)
		if item == nil {
			continue
		}
		if table, ok := item.(map[string]any); ok && len(table) == 0 {
			continue
		}
		result[key] = item
	}
	return result
}

func encode(file ConfigFile, value any) ([]byte, error) {
	if _, err := json.Marshal(value); err != nil {
		return nil, fmt.Errorf("must contain serializable data: %w", err)
	}
	var buf bytes.Buffer
	switch file.Format {
	case "text":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("text must evaluate to a string")
		}
		buf.WriteString(text)
	case "json":
		if object, ok := value.(map[string]any); ok && file.Schema != "" {
			if _, set := object["$schema"]; !set {
				value = Merge(object, map[string]any{"$schema": file.Schema})
			}
		}
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		buf.Write(append(data, '\n'))
	case "yaml":
		if file.Schema != "" {
			fmt.Fprintf(&buf, "# yaml-language-server: $schema=%s\n", file.Schema)
		}
		data, err := yaml.Marshal(value)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
	case "toml":
		if _, ok := value.(map[string]any); !ok {
			return nil, fmt.Errorf("toml must evaluate to a table")
		}
		if file.Schema != "" {
			fmt.Fprintf(&buf, "#:schema %s\n\n", file.Schema)
		}
		data, err := toml.Marshal(value)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
	}
	return buf.Bytes(), nil
}
