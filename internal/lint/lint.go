// Package lint checks canonical skills without fetching URLs or executing templates.
package lint

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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
	Rules                 []Rule            `toml:"rules,omitempty"`
	Disable               []string          `toml:"disable,omitempty"`
	Outdated              map[string]string `toml:"outdated,omitempty"`
	MaxLines              int               `toml:"max_lines,omitempty"`
	MaxAgeDays            int               `toml:"max_age_days,omitempty"`
	DuplicateMinWords     int               `toml:"duplicate_min_words,omitempty"`
	DuplicateSimilarity   float64           `toml:"duplicate_similarity,omitempty"`
	DuplicateContainment  float64           `toml:"duplicate_containment,omitempty"`
	DuplicateShingleWords int               `toml:"duplicate_shingle_words,omitempty"`
	Semantic              SemanticOptions   `toml:"semantic,omitempty"`
	Now                   time.Time         `toml:"-"`
}

var rules = []string{"metadata", "invalid-template", "invalid-markup", "broken-link", "missing-reference", "duplicate-heading", "duplicate-content", "similar-content", "semantic-content", "duplicate-skill", "outdated-reference", "large-skill", "stale-review"}

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
	if _, err := compileRules(o.Rules); err != nil {
		return err
	}
	for _, rule := range o.Disable {
		if !slices.Contains(rules, rule) && !slices.ContainsFunc(o.Rules, func(r Rule) bool { return r.ID == rule }) {
			return fmt.Errorf("unknown lint rule %q", rule)
		}
	}
	if o.MaxLines < 0 || o.MaxAgeDays < 0 || o.DuplicateMinWords < 0 || o.DuplicateShingleWords < 0 {
		return fmt.Errorf("lint limits must not be negative")
	}
	for old := range o.Outdated {
		if strings.TrimSpace(old) == "" {
			return fmt.Errorf("outdated reference must not be empty")
		}
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
	rules             []compiledRule
	options           Options
	diagnostics       []Diagnostic
	names             map[string]Location
	paragraphs        map[string]Diagnostic
	similarParagraphs []paragraph
	shingleIndex      map[string][]int
	anchors           map[string]map[string]bool
}

// Run checks Markdown files and directories recursively. Artifact references are
// resolved against the complete set of supplied paths, not an external registry.
func Run(ctx context.Context, paths []string, options Options) ([]Diagnostic, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if options.MaxLines == 0 {
		options.MaxLines = 500
	}
	if options.MaxAgeDays == 0 {
		options.MaxAgeDays = 180
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
	c := checker{options: options, diagnostics: []Diagnostic{}, names: make(map[string]Location), paragraphs: make(map[string]Diagnostic), anchors: make(map[string]map[string]bool), shingleIndex: make(map[string][]int)}
	c.rules, err = compileRules(options.Rules)
	if err != nil {
		return nil, err
	}
	var documents []document
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		d := document{path: path, source: data, kind: artifactKind(path)}
		d.art, err = artifact.Parse(data)
		if err != nil {
			c.add(d, 0, "error", "metadata", err.Error())
			continue
		}
		d.body = []byte(d.art.Body)
		d.offset = len(data) - len(d.body)
		if d.kind != artifact.TypeUnknown {
			name, _ := d.art.Frontmatter["name"].(string)
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(path), ".md")
				if filepath.Base(path) == artifact.MainFileName(d.kind) {
					name = filepath.Base(filepath.Dir(path))
				}
			}
			key := string(d.kind) + ":" + name
			if first, ok := c.names[key]; ok {
				c.addDuplicate(d, 0, "duplicate-skill", fmt.Sprintf("duplicate %s name %q; first defined in %s", d.kind, name, first.Path), first)
			} else {
				c.names[key] = Location{Path: path, Line: 1, Column: 1}
			}
		}
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
	for _, path := range paths {
		err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				if slices.Contains([]string{".git", ".henia", "node_modules", "vendor"}, entry.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(path), ".md") {
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
			return nil, fmt.Errorf("scan %s: %w", path, err)
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
	c.checkRules(d)
	if d.kind != artifact.TypeUnknown {
		for _, key := range []string{"name", "description"} {
			value, _ := d.art.Frontmatter[key].(string)
			if strings.TrimSpace(value) == "" {
				c.add(d, 0, "warning", "metadata", "frontmatter requires a nonempty string "+key)
			}
		}
		lines := bytes.Count(d.body, []byte{'\n'})
		if len(d.body) > 0 && d.body[len(d.body)-1] != '\n' {
			lines++
		}
		if lines > c.options.MaxLines {
			c.add(d, d.offset, "warning", "large-skill", fmt.Sprintf("%d lines exceeds %d; consider moving reference material into resources", lines, c.options.MaxLines))
		}
	}
	if value, ok := d.art.Frontmatter["last_verified"]; ok {
		var date time.Time
		switch v := value.(type) {
		case time.Time:
			date = v
		case string:
			date, _ = time.Parse("2006-01-02", v)
		}
		if date.IsZero() {
			c.add(d, 0, "warning", "metadata", "last_verified must be a YYYY-MM-DD date")
		} else if c.options.Now.Sub(date) > time.Duration(c.options.MaxAgeDays)*24*time.Hour {
			c.add(d, 0, "warning", "stale-review", fmt.Sprintf("last verified %s; review references older than %d days", date.Format("2006-01-02"), c.options.MaxAgeDays))
		}
	}
	markupSource := d.body
	if d.kind != artifact.TypeUnknown {
		var templateErr error
		markupSource, templateErr = templateMask(d.body)
		if templateErr != nil {
			c.add(d, d.offset, "error", "invalid-template", templateErr.Error())
		}
	}
	if _, err := markup.Render(string(markupSource), "directives"); err != nil {
		offset := d.offset
		if location, ok := errors.AsType[*markup.Error](err); ok {
			for range location.Line - 1 {
				next := bytes.IndexByte(d.source[offset:], '\n')
				if next < 0 {
					break
				}
				offset += next + 1
			}
			offset += location.Column - 1
		}
		c.add(d, offset, "error", "invalid-markup", err.Error())
	}
	c.checkDuplicates(d, markupSource)
	headings := make(map[string]Location)
	root := goldmark.New().Parser().Parse(text.NewReader(d.body))
	// Plain Goldmark retains canonical directive text, which lets lint inspect
	// references in directive content without rewriting diagnostic positions.
	return ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Heading:
			key := normalize(string(n.Text(d.body)))
			offset := d.offset + n.Lines().At(0).Start
			if first, ok := headings[key]; ok {
				c.addDuplicate(d, offset, "duplicate-heading", fmt.Sprintf("heading repeated; first occurrence on line %d", first.Line), first)
			} else {
				headings[key] = sourceLocation(d, offset)
			}
		case *ast.CodeSpan:
			value := string(n.Text(d.body))
			offset := d.offset + nodeOffset(n)
			for _, ref := range reference.Parse("`" + value + "`") {
				if ref.Type == reference.TypeTool {
					continue
				}
				if ref.Type == reference.TypeFile {
					c.checkLink(d, offset, ref.Name)
					continue
				}
				if _, ok := c.names[ref.Type.String()+":"+ref.Name]; !ok {
					c.add(d, offset, "warning", "missing-reference", fmt.Sprintf("%s reference %q is absent from scanned artifacts", ref.Type, ref.Name))
				}
			}
			c.checkOutdated(d, offset, value)
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			c.checkLink(d, d.offset+nodeOffset(n), string(n.Destination))
			c.checkOutdated(d, d.offset+nodeOffset(n), string(n.Destination))
		case *ast.Image:
			c.checkLink(d, d.offset+nodeOffset(n), string(n.Destination))
		case *ast.Text:
			c.checkOutdated(d, d.offset+n.Segment.Start, string(n.Segment.Value(d.body)))
		}
		return ast.WalkContinue, nil
	})
}

func nodeOffset(node ast.Node) int {
	if n, ok := node.(*ast.Text); ok {
		return n.Segment.Start
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if offset := nodeOffset(child); offset > 0 {
			return offset
		}
	}
	return 0
}

func normalize(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), " ")) }

func (c *checker) checkLink(d document, offset int, destination string) {
	if destination == "" || strings.Contains(destination, "{{") {
		return
	}
	u, err := url.Parse(destination)
	if err != nil {
		c.add(d, offset, "warning", "broken-link", "invalid link "+destination)
		return
	}
	if u.IsAbs() || u.Host != "" || strings.HasPrefix(u.Path, "~") || filepath.IsAbs(u.Path) {
		return
	}
	path := d.path
	if u.Path != "" {
		path = filepath.Join(filepath.Dir(d.path), filepath.FromSlash(u.Path))
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.add(d, offset, "warning", "broken-link", "local reference does not exist: "+destination)
		} else {
			c.add(d, offset, "error", "broken-link", fmt.Sprintf("inspect local reference %s: %v", destination, err))
		}
		return
	}
	if u.Fragment != "" && strings.EqualFold(filepath.Ext(path), ".md") {
		anchors, err := c.headingAnchors(path)
		if err != nil {
			c.add(d, offset, "error", "broken-link", err.Error())
			return
		}
		if anchors != nil && !anchors[u.Fragment] {
			c.add(d, offset, "warning", "broken-link", "Markdown heading anchor does not exist: "+destination)
		}
	}
}

func (c *checker) headingAnchors(path string) (map[string]bool, error) {
	if anchors, ok := c.anchors[path]; ok {
		return anchors, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read reference %s: %w", path, err)
	}
	art, err := artifact.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse reference %s: %w", path, err)
	}
	// Dynamic headings and explicit HTML anchors need the rendered harness output.
	if strings.Contains(art.Body, "{{") || strings.Contains(art.Body, "id=") || strings.Contains(art.Body, "name=") {
		c.anchors[path] = nil
		return nil, nil
	}
	anchors := make(map[string]bool)
	root := goldmark.New(goldmark.WithParserOptions(parser.WithAutoHeadingID())).Parser().Parse(text.NewReader([]byte(art.Body)))
	if err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && node.Kind() == ast.KindHeading {
			if id, ok := node.AttributeString("id"); ok {
				anchors[string(id.([]byte))] = true
			}
		}
		return ast.WalkContinue, nil
	}); err != nil {
		return nil, err
	}
	c.anchors[path] = anchors
	return anchors, nil
}

func (c *checker) checkOutdated(d document, offset int, value string) {
	for old, replacement := range c.options.Outdated {
		if index := strings.Index(value, old); index >= 0 {
			c.add(d, offset+index, "warning", "outdated-reference", fmt.Sprintf("%q is marked outdated; use %q", old, replacement))
		}
	}
}
