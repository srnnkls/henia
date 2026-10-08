// Package lint checks canonical skills without fetching URLs or executing templates.
package lint

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/templates"
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
	Templates    []templates.File          `toml:"-"`
	Variables    map[string]string         `toml:"-"`
	Now          time.Time                 `toml:"-"`
}

func (o Options) Validate() error { return o.Semantic.validate() }

type document struct {
	path   string
	source []byte
	body   []byte
	offset int
	art    *artifact.Artifact
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
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column), strings.Compare(a.Rule, b.Rule), strings.Compare(a.Message, b.Message))
	})
	return slices.CompactFunc(diagnostics, func(a, b Diagnostic) bool {
		return a.Path == b.Path && a.Line == b.Line && a.Column == b.Column && a.Rule == b.Rule && a.Message == b.Message
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
			if path != root && artifactKind(path) == artifact.TypeUnknown && !inSkill(filepath.Dir(absolute)) {
				return nil
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
