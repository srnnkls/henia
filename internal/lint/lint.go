// Package lint checks canonical skills without fetching URLs or executing templates.
package lint

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
)

type Diagnostic struct {
	Path          string     `json:"path"`
	Line          int        `json:"line"`
	Column        int        `json:"column"`
	Severity      string     `json:"severity"`
	Rule          string     `json:"rule"`
	Message       string     `json:"message"`
	Related       []Location `json:"related,omitempty"`
	Similarity    *float64   `json:"similarity,omitempty"`
	Method        string     `json:"method,omitempty"`
	SharedPhrases []string   `json:"shared_phrases,omitempty"`
	Model         string     `json:"model,omitempty"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Options struct {
	Rules                 []InlineRule              `toml:"rules,omitempty"`
	Config                map[string]map[string]any `toml:"config,omitempty"`
	Modules               []string                  `toml:"-"`
	Registries            []Registry                `toml:"registries,omitempty"`
	Disable               []string                  `toml:"disable,omitempty"`
	Outdated              map[string]string         `toml:"outdated,omitempty"`
	External              []string                  `toml:"external,omitempty"`
	MaxLines              int                       `toml:"max_lines,omitempty"`
	MaxAgeDays            int                       `toml:"max_age_days,omitempty"`
	DuplicateMinWords     int                       `toml:"duplicate_min_words,omitempty"`
	DuplicateSimilarity   float64                   `toml:"duplicate_similarity,omitempty"`
	DuplicateContainment  float64                   `toml:"duplicate_containment,omitempty"`
	DuplicateShingleWords int                       `toml:"duplicate_shingle_words,omitempty"`
	Semantic              SemanticOptions           `toml:"semantic,omitempty"`
	Now                   time.Time                 `toml:"-"`
}

var rules = []string{"similar-content", "semantic-content", "invalid-slot"}

func (o Options) Validate() error {
	for _, limit := range []struct {
		name  string
		value float64
	}{{"duplicate_similarity", o.DuplicateSimilarity}, {"duplicate_containment", o.DuplicateContainment}} {
		if math.IsNaN(limit.value) || limit.value < 0 || limit.value > 1 {
			return fmt.Errorf("%s must be between 0 and 1 (0 disables this measure)", limit.name)
		}
	}
	if err := o.Semantic.validate(); err != nil {
		return err
	}
	var custom []string
	for _, rule := range o.Rules {
		custom = append(custom, rule.ID)
	}
	if _, err := compileRegistries(o.Registries, custom); err != nil {
		return err
	}
	if o.DuplicateMinWords < 0 || o.DuplicateShingleWords < 0 {
		return fmt.Errorf("lint limits must not be negative")
	}
	return nil
}

type document struct {
	path   string
	source []byte
	body   []byte
	offset int
	art    *artifact.Artifact
	kind   artifact.Type
}

type checker struct {
	registries        []compiledRegistry
	options           Options
	diagnostics       []Diagnostic
	paragraphs        map[string]Diagnostic
	similarParagraphs []paragraph
	shingleIndex      map[string][]int
}

// Run checks Markdown files and directories recursively. Artifact references are
// resolved against the complete set of supplied paths, not an external registry.
func Run(ctx context.Context, paths []string, options Options) ([]Diagnostic, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if options.DuplicateMinWords == 0 {
		options.DuplicateMinWords = 12
	}
	if options.DuplicateShingleWords == 0 {
		options.DuplicateShingleWords = 3
	}
	if options.Now.IsZero() {
		options.Now = time.Now()
	}
	files, err := collect(ctx, paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Markdown files found")
	}
	c := checker{options: options, diagnostics: []Diagnostic{}, paragraphs: make(map[string]Diagnostic), shingleIndex: make(map[string][]int)}
	registry, err := loadModules(options.Modules)
	if err != nil {
		return nil, err
	}
	specs, err := registry.specs(options.Rules)
	if err != nil {
		return nil, err
	}
	known := slices.Clone(rules)
	for _, s := range specs {
		known = append(known, s.id)
		if err := s.configure(options.Config[s.id]); err != nil {
			return nil, err
		}
	}
	for _, r := range options.Registries {
		known = append(known, r.ID)
	}
	for _, rule := range slices.Concat(options.Disable, slices.Collect(maps.Keys(options.Config))) {
		if !slices.Contains(known, rule) {
			return nil, fmt.Errorf("unknown lint rule %q", rule)
		}
	}
	var custom []string
	for _, rule := range options.Rules {
		custom = append(custom, rule.ID)
	}
	c.registries, err = compileRegistries(options.Registries, custom)
	if err != nil {
		return nil, err
	}
	var documents []document
	var docs []library.Document
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		d := document{path: path, source: data, kind: artifactKind(path)}
		docs = append(docs, library.Document{Path: path, Kind: string(d.kind)})
		d.art, err = artifact.Parse(data)
		if err != nil {
			continue
		}
		d.body = []byte(d.art.Body)
		d.offset = len(data) - len(d.body)
		documents = append(documents, d)
	}
	for _, d := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := c.check(d); err != nil {
			return nil, err
		}
	}
	ev := &evaluation{corpus: library.Documents(docs)}
	for _, s := range specs {
		if slices.Contains(options.Disable, s.id) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		diagnostics, err := ev.run(s, options)
		if err != nil {
			return nil, err
		}
		c.diagnostics = append(c.diagnostics, diagnostics...)
	}
	c.checkRegistries(documents)
	c.checkSlots(documents)
	if err := c.checkSemantic(ctx); err != nil {
		return nil, err
	}
	slices.SortFunc(c.diagnostics, func(a, b Diagnostic) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column), strings.Compare(a.Rule, b.Rule), strings.Compare(a.Message, b.Message))
	})
	return c.diagnostics, nil
}

func collect(ctx context.Context, paths []string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string
	for _, root := range paths {
		ignored, err := gitIgnored(ctx, root)
		if err != nil {
			return nil, err
		}
		err = artifact.Walk(root, func(path string, info fs.FileInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if absolute, err := filepath.Abs(path); err == nil && path != root && ignored[absolute] {
				if info.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				if slices.Contains([]string{".git", "node_modules", "vendor"}, info.Name()) ||
					filepath.Base(filepath.Dir(path)) == ".henia" && slices.Contains([]string{"build", "cache", "state", "lint"}, info.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if !seen[absolute] {
				files = append(files, path)
				seen[absolute] = true
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", root, err)
		}
	}
	slices.Sort(files)
	return files, nil
}

func artifactKind(path string) artifact.Type {
	for _, kind := range []artifact.Type{artifact.TypeSkill, artifact.TypeCommand, artifact.TypeAgent} {
		if filepath.Base(path) == artifact.MainFileName(kind) || filepath.Base(filepath.Dir(path)) == artifact.TypeDirName(kind) {
			return kind
		}
	}
	return artifact.TypeUnknown
}

func (c *checker) add(d document, offset int, severity, rule, message string) {
	if slices.Contains(c.options.Disable, rule) {
		return
	}
	offset = min(max(offset, 0), len(d.source))
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Path: d.path, Line: bytes.Count(d.source[:offset], []byte{'\n'}) + 1,
		Column: offset - bytes.LastIndexByte(d.source[:offset], '\n'), Severity: severity, Rule: rule, Message: message,
	})
}

func (c *checker) check(d document) error {
	markupSource := d.body
	if d.kind != artifact.TypeUnknown {
		if masked, err := markup.MaskTemplates(d.body); err == nil {
			markupSource = masked
		}
	}
	c.checkDuplicates(d, markupSource)
	return nil
}

// gitIgnored returns the absolute paths Git ignores below root. Ignored
// directories are reported once rather than per file. Outside a repository,
// or without git, nothing is ignored.
func gitIgnored(ctx context.Context, root string) (map[string]bool, error) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory").Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, nil
	}
	ignored := make(map[string]bool)
	for entry := range strings.SplitSeq(string(out), "\x00") {
		if entry != "" {
			ignored[filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(entry, "/")))] = true
		}
	}
	return ignored, nil
}
