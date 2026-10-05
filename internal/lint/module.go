package lint

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
	"github.com/srnnkls/henia/internal/similarity"
)

//go:embed std/*.md
var stdlib embed.FS

type InlineRule struct {
	ID       string         `toml:"id"`
	Query    string         `toml:"query"`
	Message  string         `toml:"message"`
	Severity string         `toml:"severity,omitempty"`
	At       string         `toml:"at,omitempty"`
	Related  string         `toml:"related,omitempty"`
	Params   map[string]any `toml:"params,omitempty"`
	Data     map[string]any `toml:"data,omitempty"`
}

type spec struct {
	id       string
	rules    []*pattern.Rule
	module   string
	params   map[string]string
	data     map[string]any
	examples []example
}

type example struct {
	matches bool
	source  string
	line    int
	params  map[string]any
}

type moduleFile struct {
	name, path string
	data       []byte
}

type registry struct {
	files   map[string]moduleFile
	defines map[string]map[string]pattern.Define
	loading map[string]bool
}

func loadModules(dirs []string) (*registry, error) {
	r := &registry{files: map[string]moduleFile{}, defines: map[string]map[string]pattern.Define{}, loading: map[string]bool{}}
	err := fs.WalkDir(stdlib, "std", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := stdlib.ReadFile(path)
		name := strings.TrimSuffix(path, ".md")
		r.files[name] = moduleFile{name, "henia:" + path, data}
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fs.SkipDir
				}
				return err
			}
			if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			name := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
			if first, taken := r.files[name]; taken {
				return fmt.Errorf("lint module %s is defined by %s and %s; rename one and import it where needed", name, first.path, path)
			}
			r.files[name] = moduleFile{name, path, data}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *registry) specs(inline []InlineRule) ([]*spec, error) {
	var out []*spec
	byID := map[string]*spec{}
	add := func(s *spec) error {
		if first, taken := byID[s.id]; taken {
			return fmt.Errorf("lint rule %s is defined in %s and %s; rule ids are unique, so disable one or rename it", s.id, first.module, s.module)
		}
		byID[s.id] = s
		out = append(out, s)
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(r.files)) {
		specs, _, err := r.parse(r.files[name])
		if err != nil {
			return nil, err
		}
		for _, s := range specs {
			if err := add(s); err != nil {
				return nil, err
			}
		}
	}
	for _, in := range inline {
		rule, err := pattern.NewRule(in.ID, in.Severity, in.Message, in.At, in.Related, in.Query)
		if err != nil {
			return nil, err
		}
		if err := add(&spec{id: rule.ID, rules: []*pattern.Rule{rule}, module: "henia.toml", params: scalars(in.Params), data: in.Data}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *registry) imported(name string) (map[string]pattern.Define, error) {
	if defines, ok := r.defines[name]; ok {
		return defines, nil
	}
	file, ok := r.files[name]
	if !ok {
		return nil, fmt.Errorf("no lint module %q", name)
	}
	if r.loading[name] {
		return nil, fmt.Errorf("lint module %q imports itself", name)
	}
	r.loading[name] = true
	defer delete(r.loading, name)
	_, defines, err := r.parse(file)
	return defines, err
}

type block struct {
	start, line int
	text        string
}

func (r *registry) parse(file moduleFile) ([]*spec, map[string]pattern.Define, error) {
	art, err := artifact.Parse(file.data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file.path, err)
	}
	shift := strings.Count(string(file.data[:len(file.data)-len(art.Body)]), "\n")
	tree, err := markup.Tree([]byte(art.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file.path, err)
	}
	var source strings.Builder
	var blocks []block
	examples := map[string][]example{}
	tree.Walk(func(e *markup.Element) bool {
		if e.Type != "code" {
			return true
		}
		switch e.Attrs["lang"] {
		case "hq":
			blocks = append(blocks, block{source.Len(), e.Line + 1 + shift, e.Text})
			source.WriteString(e.Text + "\n")
		case "md", "markdown":
			heading := e.Enclosing("section")
			if heading == nil || heading.Attrs["level"] != "3" || heading.Parent == nil || heading.Parent.Type != "section" {
				return true
			}
			kind := strings.ToLower(heading.Attrs["title"])
			if kind == "matches" || kind == "passes" {
				id := heading.Parent.Attrs["title"]
				params := map[string]any{}
				for _, field := range strings.Fields(e.Attrs["info"])[1:] {
					key, value, ok := strings.Cut(field, "=")
					if !ok {
						continue
					}
					if table, row, nested := strings.Cut(key, "."); nested {
						rows, _ := params[table].(map[string]any)
						if rows == nil {
							rows = map[string]any{}
						}
						rows[row] = value
						params[table] = rows
						continue
					}
					params[key] = value
				}
				examples[id] = append(examples[id], example{kind == "matches", e.Text, e.Line + 1 + shift, params})
			}
		}
		return true
	})
	module, err := pattern.ReadModule(source.String(), r.imported)
	if err != nil {
		var pe *pattern.Error
		if errors.As(err, &pe) {
			for i := len(blocks) - 1; i >= 0; i-- {
				if pe.Offset >= blocks[i].start {
					pe.Offset -= blocks[i].start
					line := blocks[i].line + strings.Count(blocks[i].text[:min(pe.Offset, len(blocks[i].text))], "\n")
					return nil, nil, fmt.Errorf("%s:%d: %s", file.path, line, strings.TrimPrefix(pe.Explain(blocks[i].text), "henia query: "))
				}
			}
		}
		return nil, nil, fmt.Errorf("%s: %w", file.path, err)
	}
	params, _ := art.Frontmatter["params"].(map[string]any)
	data, _ := art.Frontmatter["data"].(map[string]any)
	var specs []*spec
	byID := map[string]*spec{}
	for _, rule := range module.Rules {
		if s, ok := byID[rule.ID]; ok {
			s.rules = append(s.rules, rule)
			continue
		}
		byID[rule.ID] = &spec{id: rule.ID, rules: []*pattern.Rule{rule}, module: file.name, params: scalars(params), data: data, examples: examples[rule.ID]}
		specs = append(specs, byID[rule.ID])
	}
	return specs, module.Defines, nil
}

func scalars(values map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		switch value.(type) {
		case map[string]any, []any:
		default:
			out[key] = fmt.Sprint(value)
		}
	}
	return out
}

func (s *spec) configure(config map[string]any) error {
	params, data := maps.Clone(s.params), maps.Clone(s.data)
	if data == nil {
		data = map[string]any{}
	}
	for key, value := range config {
		switch v := value.(type) {
		case map[string]any, []any:
			data[key] = v
		default:
			if key == "severity" {
				if v != "warning" && v != "error" {
					return fmt.Errorf("lint.config.%s.severity must be warning or error", s.id)
				}
				rules := make([]*pattern.Rule, len(s.rules))
				for i, rule := range s.rules {
					copied := *rule
					copied.Severity = v.(string)
					rules[i] = &copied
				}
				s.rules = rules
				continue
			}
			params[key] = fmt.Sprint(v)
		}
	}
	s.params, s.data = params, data
	return nil
}

func dataRoot(tables map[string]any) *markup.Element {
	if len(tables) == 0 {
		return nil
	}
	root := &markup.Element{Type: "data", Attrs: map[string]string{}}
	add := func(attrs map[string]string) {
		root.Children = append(root.Children, &markup.Element{Type: "row", Attrs: attrs, Parent: root, Index: len(root.Children)})
	}
	for _, table := range slices.Sorted(maps.Keys(tables)) {
		switch rows := tables[table].(type) {
		case map[string]any:
			for _, key := range slices.Sorted(maps.Keys(rows)) {
				add(map[string]string{"table": table, "key": key, "value": fmt.Sprint(rows[key])})
			}
		case []any:
			for i, value := range rows {
				add(map[string]string{"table": table, "index": strconv.Itoa(i), "value": fmt.Sprint(value)})
			}
		case []string:
			for i, value := range rows {
				add(map[string]string{"table": table, "index": strconv.Itoa(i), "value": value})
			}
		}
	}
	return root
}

type evaluation struct {
	corpus *library.Corpus
	embed  func(string) []float32
	model  string
	failed error
}

func (ev *evaluation) run(s *spec, options Options) ([]Diagnostic, error) {
	var out []Diagnostic
	for _, rule := range s.rules {
		diagnostics, err := ev.clause(s, rule, options)
		if err != nil {
			return nil, err
		}
		out = append(out, diagnostics...)
	}
	return out, nil
}

func (ev *evaluation) clause(s *spec, rule *pattern.Rule, options Options) ([]Diagnostic, error) {
	env := pattern.Environment{Resolve: ev.corpus, Params: s.params, Data: dataRoot(s.data), Now: options.Now}
	if rule.Query.Semantic {
		if !options.Semantic.Enabled {
			return nil, nil
		}
		if ev.embed == nil && ev.failed == nil {
			ev.embed, ev.failed = similarity.Model(options.Semantic.ModelPath)
		}
		if ev.failed != nil {
			return nil, ev.failed
		}
		env.Embed = ev.embed
	}
	rows, err := rule.Query.Run(ev.corpus.Root, env)
	if err != nil {
		return nil, fmt.Errorf("lint rule %s: %w", rule.ID, err)
	}
	var out []Diagnostic
	for _, row := range rows {
		at := first(row, append(slices.Clone(rule.At), ""))
		if at == nil || disabledHere(at, rule.ID) {
			continue
		}
		d := Diagnostic{Severity: rule.Severity, Rule: rule.ID, Message: render(rule.Message, row, s.params)}
		location := locate(at)
		if rule.Focus != "" {
			location = focus(at, location, row.Vars[rule.Focus])
		}
		d.Path, d.Line, d.Column = location.Path, location.Line, location.Column
		if related := first(row, rule.Related); related != nil {
			d.Related = []Location{locate(related)}
		}
		out = append(out, d)
	}
	return out, nil
}

func focus(e *markup.Element, at Location, value string) Location {
	index := strings.Index(e.Text, value)
	if value == "" || index < 0 {
		return at
	}
	before := e.Text[:index]
	if newlines := strings.Count(before, "\n"); newlines > 0 {
		at.Line += newlines
		at.Column = index - strings.LastIndexByte(before, '\n')
	} else {
		at.Column += index
	}
	return at
}

func first(row pattern.Row, names []string) *markup.Element {
	for _, name := range names {
		if e := capture(row, name); e != nil {
			return e
		}
	}
	return nil
}

func capture(row pattern.Row, name string) *markup.Element {
	for _, cell := range row.Cells {
		if (name == "" || cell.Name == name) && len(cell.Elements) > 0 {
			return cell.Elements[0]
		}
	}
	return nil
}

func locate(e *markup.Element) Location {
	file := e.Enclosing("file")
	if file == nil && e.Type == "skill" && len(e.Children) > 0 {
		file = e.Children[0]
	}
	if file == nil {
		return Location{}
	}
	return Location{Path: file.Attrs["file"], Line: max(e.Line, 1), Column: max(e.Column, 1)}
}

func disabledHere(e *markup.Element, rule string) bool {
	file := e.Enclosing("file")
	if file == nil || len(file.Children) == 0 || file.Children[0].Type != "frontmatter" {
		return false
	}
	return slices.ContainsFunc(file.Children[0].Children, func(entry *markup.Element) bool {
		return entry.Attrs["key"] == "henia.lint.disable" && entry.Attrs["value"] == rule
	})
}

func render(message string, row pattern.Row, params map[string]string) string {
	for _, m := range pattern.Placeholders(message) {
		value := ""
		switch m[1] {
		case "$":
			value = params[m[2]]
		case "?":
			value = row.Vars[m[2]]
		case "@":
			e := capture(row, m[2])
			if e == nil {
				break
			}
			switch m[3] {
			case "":
				l := locate(e)
				value = fmt.Sprintf("%s:%d", l.Path, l.Line)
			case "line":
				value = strconv.Itoa(e.Line)
			case "column":
				value = strconv.Itoa(e.Column)
			case "text":
				value = strings.TrimSpace(e.Text)
			case "lines":
				value = strconv.Itoa(e.EndLine - e.Line + 1)
			default:
				value = e.Attrs[m[3]]
			}
		}
		message = strings.Replace(message, m[0], value, 1)
	}
	return message
}

type ExampleFailure struct {
	Rule    string
	Module  string
	Line    int
	Matches bool
	Found   int
}

func (f ExampleFailure) Error() string {
	if f.Matches {
		return fmt.Sprintf("%s:%d: %s should report this example, but did not", f.Module, f.Line, f.Rule)
	}
	return fmt.Sprintf("%s:%d: %s should pass this example, but reported %d diagnostics", f.Module, f.Line, f.Rule, f.Found)
}

func TestModules(dirs []string, inline []InlineRule, options Options) ([]ExampleFailure, int, error) {
	registry, err := loadModules(dirs)
	if err != nil {
		return nil, 0, err
	}
	specs, err := registry.specs(inline)
	if err != nil {
		return nil, 0, err
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	var failures []ExampleFailure
	count := 0
	for _, s := range specs {
		for _, ex := range s.examples {
			count++
			found, err := s.try(ex, options)
			if err != nil {
				return nil, 0, err
			}
			if ex.matches != (found > 0) {
				failures = append(failures, ExampleFailure{s.id, registry.files[s.module].path, ex.line, ex.matches, found})
			}
		}
	}
	return failures, count, nil
}

func (s *spec) try(ex example, options Options) (int, error) {
	dir, err := os.MkdirTemp("", "henia-lint-example")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "skills", "example", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, []byte(ex.source), 0o644); err != nil {
		return 0, err
	}
	trial := *s
	if err := trial.configure(ex.params); err != nil {
		return 0, err
	}
	ev := &evaluation{corpus: library.Documents([]library.Document{{Path: path, Kind: "skill"}})}
	diagnostics, err := ev.run(&trial, options)
	return len(diagnostics), err
}
