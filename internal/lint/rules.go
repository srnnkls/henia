package lint

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/expr-lang/expr/vm"
	"github.com/srnnkls/henia/internal/expression"
	"github.com/srnnkls/henia/internal/markup"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	ID             string `toml:"id"`
	Select         string `toml:"select"`
	When           string `toml:"when,omitempty"`
	Assert         string `toml:"assert"`
	Message        string `toml:"message"`
	Severity       string `toml:"severity,omitempty"`
	IncludeDynamic bool   `toml:"include_dynamic,omitempty"`
}

type ruleDocument struct {
	Path  string `expr:"path"`
	Kind  string `expr:"kind"`
	Body  string `expr:"body"`
	Lines int    `expr:"lines"`
}

type ruleEnvironment struct {
	Node        markup.Node    `expr:"node"`
	Document    ruleDocument   `expr:"document"`
	Frontmatter map[string]any `expr:"frontmatter"`
}

type compiledRule struct {
	Rule
	when, assert *vm.Program
}

var ruleID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func compileRules(definitions []Rule) ([]compiledRule, error) {
	seen := make(map[string]bool)
	var result []compiledRule
	for _, rule := range definitions {
		if !ruleID.MatchString(rule.ID) || seen[rule.ID] || slices.Contains(rules, rule.ID) {
			return nil, fmt.Errorf("invalid, duplicate or reserved lint rule id %q", rule.ID)
		}
		seen[rule.ID] = true
		if !slices.Contains([]string{"document", "frontmatter", "directive", "heading", "paragraph", "link", "image"}, rule.Select) {
			return nil, fmt.Errorf("rule %s: unknown selector %q", rule.ID, rule.Select)
		}
		if rule.Severity == "" {
			rule.Severity = "warning"
		}
		if rule.Severity != "warning" && rule.Severity != "error" {
			return nil, fmt.Errorf("rule %s: severity must be warning or error", rule.ID)
		}
		if strings.TrimSpace(rule.Message) == "" {
			return nil, fmt.Errorf("rule %s: message is required", rule.ID)
		}
		check, err := expression.Compile(rule.Assert, ruleEnvironment{}, true)
		if err != nil {
			return nil, fmt.Errorf("rule %s assert: %w", rule.ID, err)
		}
		when := rule.When
		if when == "" {
			when = "true"
		}
		condition, err := expression.Compile(when, ruleEnvironment{}, true)
		if err != nil {
			return nil, fmt.Errorf("rule %s when: %w", rule.ID, err)
		}
		result = append(result, compiledRule{rule, condition, check})
	}
	return result, nil
}

func (c *checker) checkRules(d document) {
	if len(c.rules) == 0 {
		return
	}
	lines := bytes.Count(d.body, []byte{'\n'})
	if len(d.body) > 0 && d.body[len(d.body)-1] != '\n' {
		lines++
	}
	env := ruleEnvironment{Document: ruleDocument{d.path, string(d.kind), d.art.Body, lines}, Frontmatter: d.art.Frontmatter}
	nodes := []markup.Node{{Kind: "document", Attrs: map[string]string{}, Text: d.art.Body, Start: -d.offset, End: len(d.body)}}
	// YAML's source marks give frontmatter rules the location of the selected key.
	var header yaml.Node
	if d.offset > 0 && yaml.NewDecoder(bytes.NewReader(d.source)).Decode(&header) == nil && len(header.Content) > 0 {
		mapping := header.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key := mapping.Content[i]
			start := 0
			for range key.Line - 1 {
				if index := bytes.IndexByte(d.source[start:], '\n'); index >= 0 {
					start += index + 1
				}
			}
			start += key.Column - 1
			value := d.art.Frontmatter[key.Value]
			nodes = append(nodes, markup.Node{Kind: "frontmatter", Name: key.Value, Value: value, Attrs: map[string]string{}, Text: mapping.Content[i+1].Value, Start: start - d.offset, End: start - d.offset, Dynamic: strings.Contains(fmt.Sprint(value), "{{")})
		}
	}
	masked, err := templateMask(d.body)
	if err == nil {
		parsed, err := markup.Inspect(masked)
		if err == nil {
			for _, node := range parsed {
				node.Dynamic = bytes.Contains(d.body[max(0, node.Start):min(len(d.body), node.End)], []byte("{{"))
				nodes = append(nodes, node)
			}
		}
	}
	for _, rule := range c.rules {
		if slices.Contains(c.options.Disable, rule.ID) {
			continue
		}
		for _, node := range nodes {
			if node.Kind != rule.Select || node.Dynamic && !rule.IncludeDynamic {
				continue
			}
			env.Node = node
			matches, err := expression.Test(rule.when, env)
			if err == nil && !matches {
				continue
			}
			var passes bool
			if err == nil {
				passes, err = expression.Test(rule.assert, env)
			}
			if err != nil {
				c.add(d, d.offset+node.Start, "error", rule.ID, "rule evaluation failed: "+err.Error())
			} else if !passes {
				c.add(d, d.offset+node.Start, rule.Severity, rule.ID, rule.Message)
			}
		}
	}
}
