package lint

import (
	"maps"
	"slices"

	"github.com/srnnkls/henia/internal/components"
	"github.com/srnnkls/henia/internal/profile"
)

const (
	MCPTransport  = "mcp-transport"
	ConfigInvalid = "config-invalid"
	ConfigSchema  = "config-schema"
	ConfigUnused  = "config-unused"
)

// configDiagnostics checks the config of every harness that builds it.
func (p *Plan) configDiagnostics() ([]Diagnostic, error) {
	built, err := builtHarnesses(p.options.Configured)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range slices.Sorted(maps.Keys(built)) {
		artifacts := built[name].Artifacts
		if len(artifacts) == 0 {
			artifacts = p.options.Artifacts
		}
		if slices.Contains(artifacts, "config") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	keys := newKeyLocator(p.options.ConfigFiles)
	at := func(key, severity, rule, message string) Diagnostic {
		loc := keys.locate(key)
		return Diagnostic{Path: loc.Path, Line: loc.Line, Column: loc.Column, Severity: severity, Rule: rule, Message: message, Stage: StageConfig}
	}
	out := mcpTransport(built[names[0]].Shared, at)
	var specific perHarness
	for _, name := range names {
		h := built[name]
		result, err := profile.RenderHarness(name, name, h, p.options.Schemas)
		if err != nil {
			specific.add(name, []Diagnostic{at("harness."+name, components.LevelError, ConfigInvalid, err.Error())})
			continue
		}
		if result == nil {
			continue
		}
		var found []Diagnostic
		for _, warning := range result.Warnings {
			found = append(found, at("harness."+name, components.LevelWarning, ConfigSchema, warning))
		}
		for _, input := range result.Omitted {
			found = append(found, at("harness."+name+".config."+input, components.LevelWarning, ConfigUnused, "profile "+h.Profile+" reads no config."+input))
		}
		specific.add(name, found)
	}
	out = append(out, specific.diagnostics(len(names))...)
	return slices.DeleteFunc(out, func(d Diagnostic) bool { return p.disabled[d.Rule] }), nil
}

func mcpTransport(shared map[string]any, at func(key, severity, rule, message string) Diagnostic) []Diagnostic {
	servers, _ := shared["mcp"].(map[string]any)
	var out []Diagnostic
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		server, _ := servers[name].(map[string]any)
		_, command := server["command"]
		_, url := server["url"]
		var problem string
		switch {
		case !command && !url:
			problem = "sets neither command nor url"
		case command && url:
			problem = "sets both command and url"
		default:
			continue
		}
		out = append(out, at("shared.mcp."+name, components.LevelError, MCPTransport, "MCP server "+name+" "+problem))
	}
	return out
}
