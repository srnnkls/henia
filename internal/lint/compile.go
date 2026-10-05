package lint

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
)

type Plan struct {
	specs    []*spec
	files    map[string]moduleFile
	disabled map[string]bool
	options  Options
}

type table struct {
	keys   []string
	values []string
	list   bool
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

func defaults(rawParams, rawData map[string]any, origin string) (map[string]pattern.Value, map[string]table, error) {
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
	data := map[string]table{}
	for key, raw := range rawData {
		t, err := parseTable(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: data %s: %w", origin, key, err)
		}
		data[key] = t
	}
	return params, data, nil
}

func parseTable(raw any) (table, error) {
	scalar := func(v any) (string, error) {
		switch v := v.(type) {
		case string:
			return v, nil
		case int, int64, float64, bool:
			return fmt.Sprint(v), nil
		}
		return "", fmt.Errorf("rows must be strings, numbers or booleans, not %T", v)
	}
	switch rows := raw.(type) {
	case map[string]any:
		t := table{}
		for _, key := range slices.Sorted(maps.Keys(rows)) {
			value, err := scalar(rows[key])
			if err != nil {
				return table{}, err
			}
			t.keys, t.values = append(t.keys, key), append(t.values, value)
		}
		return t, nil
	case []any:
		t := table{list: true}
		for _, row := range rows {
			value, err := scalar(row)
			if err != nil {
				return table{}, err
			}
			t.values = append(t.values, value)
		}
		return t, nil
	case []string:
		return table{list: true, values: rows}, nil
	}
	return table{}, fmt.Errorf("must be a table or a list, not %T", raw)
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
			t, err := parseTable(raw)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", origin, key, err)
			}
			if t.list != current.list {
				shape := map[bool]string{true: "a list", false: "a table"}
				return fmt.Errorf("%s.%s must be %s", origin, key, shape[current.list])
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

func dataRoot(tables map[string]table) *markup.Element {
	if len(tables) == 0 {
		return nil
	}
	root := &markup.Element{Type: "data", Attrs: map[string]string{}}
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		t := tables[name]
		for i, value := range t.values {
			attrs := map[string]string{"table": name, "value": value}
			if t.list {
				attrs["index"] = strconv.Itoa(i)
			} else {
				attrs["key"] = t.keys[i]
			}
			root.Children = append(root.Children, &markup.Element{Type: "row", Attrs: attrs, Parent: root, Index: len(root.Children)})
		}
	}
	return root
}

func (p *Plan) Run(ctx context.Context, paths []string) ([]Diagnostic, error) {
	files, err := collect(ctx, paths)
	if err != nil {
		return nil, err
	}
	var docs []library.Document
	for _, file := range files {
		docs = append(docs, library.Document{Path: file.path, Kind: string(artifactKind(file.path)), Dependency: !file.linted})
	}
	if !slices.ContainsFunc(files, func(file scanned) bool { return file.linted }) {
		return nil, fmt.Errorf("no Markdown files found")
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
