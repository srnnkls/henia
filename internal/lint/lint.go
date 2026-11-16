// Package lint checks canonical skills without fetching URLs or executing templates.
package lint

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/pipeline"
	"github.com/srnnkls/henia/internal/profile"
	"github.com/srnnkls/henia/internal/slots"
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
	Stage         string     `json:"stage,omitempty"`
	Harness       string     `json:"harness,omitempty"`
	Notes         []string   `json:"notes,omitempty"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type ModuleDir struct {
	Dir    string
	Prefix string
}

type Options struct {
	Rules        []InlineRule              `toml:"rules,omitempty"`
	Config       map[string]map[string]any `toml:"config,omitempty"`
	Modules      []ModuleDir               `toml:"-"`
	Disable      []string                  `toml:"disable,omitempty"`
	Semantic     SemanticOptions           `toml:"semantic,omitempty"`
	Builtin      []string                  `toml:"-"`
	Dependencies []string                  `toml:"-"`
	Project      string                    `toml:"-"`
	User         string                    `toml:"-"`
	Packages     []pipeline.Dependency     `toml:"-"`
	Configured   map[string]henia.Harness  `toml:"-"`
	Provided     []slots.Skill             `toml:"-"`
	Slots        *SlotSetup                `toml:"-"`
	Artifacts    []string                  `toml:"-"`
	ConfigFiles  []string                  `toml:"-"`
	Schemas      *profile.Schemas          `toml:"-"`
	Now          time.Time                 `toml:"-"`
}

func (o Options) Validate() error { return o.Semantic.validate() }

type document struct {
	path   string
	source []byte
	body   []byte
	offset int
	art    *artifact.Artifact
	view   *view
}

// Run checks Markdown files and directories recursively. Artifact references are
// resolved against the complete set of supplied paths, not an external registry.
func Run(ctx context.Context, paths []string, options Options) ([]Diagnostic, error) {
	plan, err := Compile(options)
	if err != nil {
		return nil, err
	}
	return plan.Run(ctx, paths)
}

func sortDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	slices.SortFunc(diagnostics, func(a, b Diagnostic) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column), strings.Compare(a.Rule, b.Rule), strings.Compare(a.Message, b.Message), strings.Compare(a.Harness, b.Harness))
	})
	return slices.CompactFunc(diagnostics, func(a, b Diagnostic) bool {
		return a.Path == b.Path && a.Line == b.Line && a.Column == b.Column && a.Rule == b.Rule && a.Message == b.Message && a.Harness == b.Harness
	})
}

func collect(ctx context.Context, paths []string) ([]string, error) {
	seen := make(map[string]bool)
	skillDirs := make(map[string]bool)
	var inSkill func(dir string) bool
	inSkill = func(dir string) bool {
		if known, ok := skillDirs[dir]; ok {
			return known
		}
		_, err := os.Stat(filepath.Join(dir, artifact.MainFileName(artifact.TypeSkill)))
		found := err == nil || filepath.Dir(dir) != dir && inSkill(filepath.Dir(dir))
		skillDirs[dir] = found
		return found
	}
	keepDir := func(_, path string, info fs.FileInfo) bool {
		return !artifact.SkippedDir(info.Name()) &&
			!(filepath.Base(filepath.Dir(path)) == ".henia" && slices.Contains([]string{"build", "cache", "state", "lint"}, info.Name()))
	}
	keepFile := func(root, path string, _ fs.FileInfo) bool {
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return false
		}
		absolute, err := filepath.Abs(path)
		return err == nil && (path == root || artifactKind(path) != artifact.TypeUnknown || inSkill(filepath.Dir(absolute)))
	}
	var files []string
	for _, root := range paths {
		found, err := artifact.Files(ctx, root, keepDir, keepFile)
		if err != nil {
			return nil, err
		}
		for _, path := range found {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			if !seen[absolute] {
				files = append(files, path)
				seen[absolute] = true
			}
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
