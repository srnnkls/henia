// Package vendor compiles canonical skill metadata to documented vendor schemas.
package vendor

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func object(value any, path string) (map[string]any, error) {
	if value == nil {
		return make(map[string]any), nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a mapping", path)
	}
	return result, nil
}

func toolList(value any, mapping map[string]string) (string, error) {
	var names []string
	switch v := value.(type) {
	case string:
		// Preserve vendor expressions containing spaces. Use a YAML list when
		// tool-name translation is needed for multiple tools.
		names = []string{v}
	case []any:
		for _, item := range v {
			name, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("must contain only strings")
			}
			names = append(names, name)
		}
	case []string:
		names = slices.Clone(v)
	default:
		return "", fmt.Errorf("must be a string or list of strings")
	}
	for i, name := range names {
		if mapped, ok := mapping[name]; ok {
			names[i] = mapped
		}
	}
	return strings.Join(names, " "), nil
}

func validate(fm map[string]any, directory string) error {
	name, ok := fm["name"].(string)
	if !ok || len(name) > 64 || !skillName.MatchString(name) || name != directory {
		return fmt.Errorf("skill name must match directory %q and contain 1–64 lowercase letters, digits or single hyphens", directory)
	}
	description, ok := fm["description"].(string)
	if !ok || strings.TrimSpace(description) == "" || utf8.RuneCountInString(description) > 1024 {
		return fmt.Errorf("skill description must be a nonempty string of at most 1024 characters")
	}
	for _, key := range []string{"license", "compatibility", "model", "context", "agent", "effort", "argument-hint", "when_to_use", "shell", "icon", "color"} {
		if value, exists := fm[key]; exists {
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", key)
			}
			if key == "compatibility" && utf8.RuneCountInString(text) > 500 {
				return fmt.Errorf("compatibility exceeds 500 characters")
			}
		}
	}
	for _, key := range []string{"disable-model-invocation", "user-invocable", "background"} {
		if value, exists := fm[key]; exists {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", key)
			}
		}
	}
	metadata, err := object(fm["metadata"], "metadata")
	if err != nil {
		return err
	}
	for key, value := range metadata {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("metadata.%s must be a string", key)
		}
	}
	for _, key := range []string{"paths", "arguments", "allowed-tools", "disallowed-tools"} {
		if value, exists := fm[key]; exists {
			if _, err := toolList(value, nil); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	}
	if _, err := object(fm["hooks"], "hooks"); err != nil {
		return err
	}
	return nil
}

func validateOpenAI(value map[string]any) error {
	for key := range value {
		if !slices.Contains([]string{"interface", "policy", "dependencies"}, key) {
			return fmt.Errorf("unknown openai field %q", key)
		}
	}
	ui, err := object(value["interface"], "openai.interface")
	if err != nil {
		return err
	}
	for key, value := range ui {
		if !slices.Contains([]string{"display_name", "short_description", "icon_small", "icon_large", "brand_color", "default_prompt"}, key) {
			return fmt.Errorf("unknown openai.interface field %q", key)
		}
		if _, ok := value.(string); !ok {
			return fmt.Errorf("openai.interface.%s must be a string", key)
		}
	}
	policy, err := object(value["policy"], "openai.policy")
	if err != nil {
		return err
	}
	for key, value := range policy {
		if key != "allow_implicit_invocation" {
			return fmt.Errorf("unknown openai.policy field %q", key)
		}
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("openai.policy.%s must be a boolean", key)
		}
	}
	deps, err := object(value["dependencies"], "openai.dependencies")
	if err != nil {
		return err
	}
	for key, value := range deps {
		if key != "tools" {
			return fmt.Errorf("unknown openai.dependencies field %q", key)
		}
		list, ok := value.([]any)
		if !ok {
			return fmt.Errorf("openai.dependencies.tools must be a list")
		}
		for _, item := range list {
			tool, err := object(item, "dependency tool")
			if err != nil {
				return err
			}
			for key, value := range tool {
				if !slices.Contains([]string{"type", "value", "description", "transport", "url"}, key) {
					return fmt.Errorf("unknown dependency tool field %q", key)
				}
				if _, ok := value.(string); !ok {
					return fmt.Errorf("dependency tool %s must be a string", key)
				}
			}
			for _, key := range []string{"type", "value"} {
				if text, ok := tool[key].(string); !ok || text == "" {
					return fmt.Errorf("dependency tool requires %s", key)
				}
			}
		}
	}
	return nil
}
