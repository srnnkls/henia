// Package canonical defines the authoring schema shared by harness transforms.
package canonical

import (
	"fmt"
	"maps"
)

type Frontmatter struct {
	Name          string
	Description   *string
	ModelTier     *string
	Tools         []string
	ToolsPolicy   map[string]any
	Enabled       *bool
	UserInvocable *bool
	Extra         map[string]any
}

func Parse(input map[string]any) (*Frontmatter, error) {
	c := &Frontmatter{Extra: maps.Clone(input)}
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("canonical name is required and must be a string")
	}
	c.Name = name
	delete(c.Extra, "name")
	for key, dest := range map[string]**string{"description": &c.Description, "model_tier": &c.ModelTier} {
		if value, exists := input[key]; exists {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("canonical %s must be a string", key)
			}
			*dest = &text
			delete(c.Extra, key)
		}
	}
	for key, dest := range map[string]**bool{"enabled": &c.Enabled, "user_invocable": &c.UserInvocable} {
		if value, exists := input[key]; exists {
			flag, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("canonical %s must be a boolean", key)
			}
			*dest = &flag
			delete(c.Extra, key)
		}
	}
	if value, exists := input["tools"]; exists {
		list, ok := value.([]any)
		if names, typed := value.([]string); typed {
			list = make([]any, len(names))
			for i, name := range names {
				list[i] = name
			}
			ok = true
		}
		if !ok {
			return nil, fmt.Errorf("canonical tools must be a list of strings")
		}
		c.Tools = make([]string, len(list))
		for i, value := range list {
			name, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("canonical tools must contain only strings")
			}
			c.Tools[i] = name
		}
		delete(c.Extra, "tools")
	}
	if value, exists := input["tools_policy"]; exists {
		policy, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("canonical tools_policy must be a mapping")
		}
		c.ToolsPolicy = maps.Clone(policy)
		delete(c.Extra, "tools_policy")
	}
	return c, nil
}

func (c *Frontmatter) ToMap() map[string]any {
	result := maps.Clone(c.Extra)
	if result == nil {
		result = make(map[string]any)
	}
	result["name"] = c.Name
	if c.Description != nil {
		result["description"] = *c.Description
	}
	if c.ModelTier != nil {
		result["model_tier"] = *c.ModelTier
	}
	if c.Tools != nil {
		result["tools"] = c.Tools
	}
	if c.ToolsPolicy != nil {
		result["tools_policy"] = maps.Clone(c.ToolsPolicy)
	}
	if c.Enabled != nil {
		result["enabled"] = *c.Enabled
	}
	if c.UserInvocable != nil {
		result["user_invocable"] = *c.UserInvocable
	}
	return result
}
