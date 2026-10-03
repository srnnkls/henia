package lint

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/expr-lang/expr/vm"
	"github.com/srnnkls/henia/internal/expression"
	"github.com/srnnkls/henia/internal/slots"
)

type Registry struct {
	ID        string `toml:"id"`
	Declare   string `toml:"declare"`
	Reference string `toml:"reference"`
	Match     string `toml:"match,omitempty"`
	Severity  string `toml:"severity,omitempty"`
}

type registryEnvironment struct {
	Document    ruleDocument   `expr:"document"`
	Frontmatter map[string]any `expr:"frontmatter"`
}

type compiledRegistry struct {
	Registry
	declare, reference *vm.Program
}

func compileRegistries(definitions []Registry, custom []Rule) ([]compiledRegistry, error) {
	seen := make(map[string]bool)
	var result []compiledRegistry
	for _, registry := range definitions {
		if !ruleID.MatchString(registry.ID) || seen[registry.ID] || slices.Contains(rules, registry.ID) || slices.ContainsFunc(custom, func(r Rule) bool { return r.ID == registry.ID }) {
			return nil, fmt.Errorf("invalid, duplicate or reserved lint registry id %q", registry.ID)
		}
		seen[registry.ID] = true
		if registry.Match == "" {
			registry.Match = "exact"
		}
		if registry.Match != "exact" && registry.Match != "dotted" {
			return nil, fmt.Errorf("registry %s: match must be exact or dotted", registry.ID)
		}
		if registry.Severity == "" {
			registry.Severity = "warning"
		}
		if registry.Severity != "warning" && registry.Severity != "error" {
			return nil, fmt.Errorf("registry %s: severity must be warning or error", registry.ID)
		}
		compiled := compiledRegistry{Registry: registry}
		for _, part := range []struct {
			name, source string
			program      **vm.Program
		}{{"declare", registry.Declare, &compiled.declare}, {"reference", registry.Reference, &compiled.reference}} {
			if strings.TrimSpace(part.source) == "" {
				return nil, fmt.Errorf("registry %s: %s is required", registry.ID, part.name)
			}
			program, err := expression.Compile(part.source, registryEnvironment{}, false)
			if err != nil {
				return nil, fmt.Errorf("registry %s %s: %w", registry.ID, part.name, err)
			}
			*part.program = program
		}
		result = append(result, compiled)
	}
	return result, nil
}

func registryNames(program *vm.Program, env registryEnvironment) ([]string, error) {
	value, err := expression.Run(program, env)
	if err != nil {
		return nil, err
	}
	return slots.Names(value)
}

func (r compiledRegistry) resolves(name string, declared map[string]bool) bool {
	if declared[name] {
		return true
	}
	return r.Match == "dotted" && slices.ContainsFunc(slices.Collect(maps.Keys(declared)), func(slot string) bool { return slots.Within(name, slot) })
}

func (c *checker) checkRegistries(documents []document) {
	for _, registry := range c.registries {
		if slices.Contains(c.options.Disable, registry.ID) {
			continue
		}
		declared := make(map[string]bool)
		references := make([][]string, len(documents))
		for i, d := range documents {
			env := registryEnvironment{Document: ruleDocument{Path: d.path, Kind: string(d.kind), Body: d.art.Body}, Frontmatter: d.art.Frontmatter}
			names, err := registryNames(registry.declare, env)
			if err != nil {
				c.add(d, 0, "error", registry.ID, "registry declare evaluation failed: "+err.Error())
				continue
			}
			for _, name := range names {
				declared[name] = true
			}
			if references[i], err = registryNames(registry.reference, env); err != nil {
				c.add(d, 0, "error", registry.ID, "registry reference evaluation failed: "+err.Error())
			}
		}
		for i, d := range documents {
			for _, name := range references[i] {
				if registry.resolves(name, declared) {
					continue
				}
				offset := 0
				if index := bytes.Index(d.source[:d.offset], []byte(name)); index >= 0 {
					offset = index
				}
				c.add(d, offset, registry.Severity, registry.ID, fmt.Sprintf("%q is not declared by any scanned document", name))
			}
		}
	}
}
