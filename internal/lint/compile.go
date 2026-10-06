package lint

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/pattern"
)

type Plan struct {
	specs    []*spec
	files    map[string]moduleFile
	disabled map[string]bool
	options  Options
}

func Compile(options Options) (*Plan, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	registry, err := loadModules(options.Modules)
	if err != nil {
		return nil, err
	}
	specs, err := registry.specs(options.Rules)
	if err != nil {
		return nil, err
	}
	byID := map[string]*spec{}
	var ids []string
	for _, s := range specs {
		byID[s.id] = s
		ids = append(ids, s.id)
	}
	for _, id := range slices.Sorted(maps.Keys(options.Config)) {
		s, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("lint.config.%s: no lint rule %q; %s", id, id, pattern.Suggest(id, ids))
		}
		if err := s.apply(options.Config[id], "lint.config."+id); err != nil {
			return nil, err
		}
	}
	disabled := map[string]bool{}
	for _, id := range options.Disable {
		if _, ok := byID[id]; !ok {
			return nil, fmt.Errorf("unknown lint rule %q; %s", id, pattern.Suggest(id, ids))
		}
		disabled[id] = true
	}
	for _, s := range specs {
		for _, rule := range s.rules {
			for name, number := range rule.Params() {
				value, ok := s.params[name]
				if !ok {
					return nil, fmt.Errorf("lint rule %s uses $%s, which %s does not define under params", s.id, name, s.module)
				}
				if number && !value.IsNumber {
					return nil, fmt.Errorf("lint rule %s compares $%s as a number, but it is %q", s.id, name, value.Text)
				}
			}
		}
	}
	return &Plan{specs: specs, files: registry.files, disabled: disabled, options: options}, nil
}

func defaults(rawParams, rawData map[string]any, origin string) (map[string]pattern.Value, map[string]pattern.Table, error) {
	params := map[string]pattern.Value{}
	for key, raw := range rawParams {
		switch v := raw.(type) {
		case int:
			params[key] = pattern.Number(float64(v))
		case int64:
			params[key] = pattern.Number(float64(v))
		case float64:
			params[key] = pattern.Number(v)
		case string:
			params[key] = pattern.Text(v)
		case bool:
			params[key] = pattern.Text(strconv.FormatBool(v))
		default:
			return nil, nil, fmt.Errorf("%s: param %s must be a number, string or boolean", origin, key)
		}
	}
	data := map[string]pattern.Table{}
	for key, raw := range rawData {
		t, err := pattern.ParseTable(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: data %s: %w", origin, key, err)
		}
		data[key] = t
	}
	return params, data, nil
}

func (s *spec) apply(config map[string]any, origin string) error {
	params, data := maps.Clone(s.params), maps.Clone(s.data)
	for _, key := range slices.Sorted(maps.Keys(config)) {
		raw := config[key]
		if key == "severity" {
			severity, ok := raw.(string)
			if !ok || severity != "warning" && severity != "error" {
				return fmt.Errorf("%s.severity must be warning or error, not %v", origin, raw)
			}
			rules := make([]*pattern.Rule, len(s.rules))
			for i, rule := range s.rules {
				copied := *rule
				copied.Severity = severity
				rules[i] = &copied
			}
			s.rules = rules
			continue
		}
		if current, ok := params[key]; ok {
			value, err := parseParam(raw, current)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", origin, key, err)
			}
			params[key] = value
			continue
		}
		if current, ok := data[key]; ok {
			t, err := pattern.ParseTable(raw)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", origin, key, err)
			}
			if t.List != current.List {
				shape := map[bool]string{true: "a list", false: "a table"}
				return fmt.Errorf("%s.%s must be %s", origin, key, shape[current.List])
			}
			data[key] = t
			continue
		}
		known := append(slices.Sorted(maps.Keys(params)), slices.Sorted(maps.Keys(data))...)
		return fmt.Errorf("%s: %s has no param or data %q; %s", origin, s.id, key, pattern.Suggest(key, append(known, "severity")))
	}
	s.params, s.data = params, data
	return nil
}

func parseParam(raw any, current pattern.Value) (pattern.Value, error) {
	if !current.IsNumber {
		switch v := raw.(type) {
		case string:
			return pattern.Text(v), nil
		case bool:
			return pattern.Text(strconv.FormatBool(v)), nil
		}
		return pattern.Value{}, fmt.Errorf("%v is not text", raw)
	}
	switch v := raw.(type) {
	case int:
		return pattern.Number(float64(v)), nil
	case int64:
		return pattern.Number(float64(v)), nil
	case float64:
		return pattern.Number(v), nil
	case string:
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return pattern.Number(n), nil
		}
	}
	return pattern.Value{}, fmt.Errorf("%q is not a number", fmt.Sprint(raw))
}

func (p *Plan) Run(ctx context.Context, paths []string) ([]Diagnostic, error) {
	files, err := collect(ctx, paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no skills, commands, agents or skill resources found")
	}
	var docs []library.Document
	for _, path := range files {
		docs = append(docs, library.Document{Path: path, Kind: string(artifactKind(path))})
	}
	for _, path := range p.options.Dependencies {
		if !slices.Contains(files, path) {
			docs = append(docs, library.Document{Path: path, Kind: "skill", Dependency: true})
		}
	}
	ev := &evaluation{corpus: corpus(docs)}
	diagnostics := []Diagnostic{}
	for _, s := range p.specs {
		if p.disabled[s.id] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		found, err := ev.run(s, p.options)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, found...)
	}
	return sortDiagnostics(diagnostics), nil
}

func (p *Plan) Test() ([]ExampleFailure, int, error) {
	var failures []ExampleFailure
	count := 0
	for _, s := range p.specs {
		for _, ex := range s.examples {
			count++
			found, err := s.try(ex, p.options)
			if err != nil {
				return nil, 0, err
			}
			if ex.matches != (found > 0) {
				failures = append(failures, ExampleFailure{s.id, p.files[s.module].path, ex.line, ex.matches, found})
			}
		}
	}
	return failures, count, nil
}
