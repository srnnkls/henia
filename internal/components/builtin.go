package components

const Head = "head"

func Builtins() map[string]Declaration {
	return map[string]Declaration{
		Head: builtin(Head, []string{Container}, "markdown", nil, false),
		"include": builtin("include", []string{Leaf}, "none", map[string]Attribute{
			"ref":      {Type: "ref"},
			"template": {Type: "string"},
		}, true),
		"when":     builtin("when", []string{Container}, "markdown", map[string]Attribute{"target": {Type: "list"}}, true),
		"unless":   builtin("unless", []string{Container}, "markdown", map[string]Attribute{"target": {Type: "list"}}, true),
		"slot":     builtin("slot", []string{Text}, "text", nil, false),
		"contents": builtin("contents", []string{Text}, "text", nil, false),
		"related":  builtin("related", []string{Text, Container}, "markdown", nil, false),
		"body":     builtin("body", []string{Leaf}, "none", nil, false),
	}
}

func builtin(name string, forms []string, kind string, attributes map[string]Attribute, open bool) Declaration {
	if attributes == nil {
		attributes = map[string]Attribute{}
	}
	return Declaration{
		Name:       name,
		Forms:      forms,
		Attributes: attributes,
		Body:       Body{Kind: kind},
		Render:     Expand,
		Builtin:    true,
		Open:       open,
	}
}

func Reserved(name string) bool {
	_, ok := Builtins()[name]
	return ok && name != Head
}
