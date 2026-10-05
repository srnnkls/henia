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
	Rules    []InlineRule              `toml:"rules,omitempty"`
	Config   map[string]map[string]any `toml:"config,omitempty"`
	Modules  []string                  `toml:"-"`
	Disable  []string                  `toml:"disable,omitempty"`
	Semantic SemanticOptions           `toml:"semantic,omitempty"`
	Builtin  []string                  `toml:"-"`
	Known    []string                  `toml:"-"`
	Now      time.Time                 `toml:"-"`
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

type scanned struct {
	path   string
	linted bool
}

func collect(ctx context.Context, paths []string) ([]scanned, error) {
	seen := make(map[string]bool)
	var files []scanned
	for _, root := range paths {
		ignored, err := gitIgnored(ctx, root)
		if err != nil {
			return nil, err
		}
		var dependencies []string
		err = artifact.Walk(root, func(path string, info fs.FileInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			dependency := path != root && ignored[absolute] || slices.ContainsFunc(dependencies, func(dir string) bool {
				return strings.HasPrefix(absolute, dir+string(filepath.Separator))
			})
			if info.IsDir() {
				if slices.Contains([]string{".git", "node_modules", "vendor"}, info.Name()) ||
					filepath.Base(filepath.Dir(path)) == ".henia" && slices.Contains([]string{"build", "cache", "state", "lint"}, info.Name()) {
					return fs.SkipDir
				}
				if dependency && path != root && ignored[absolute] {
					dependencies = append(dependencies, absolute)
				}
				return nil
			}
			if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".md") || seen[absolute] {
				return nil
			}
			if dependency && artifactKind(path) == artifact.TypeUnknown {
				return nil
			}
			seen[absolute] = true
			files = append(files, scanned{path, !dependency})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", root, err)
		}
	}
	slices.SortFunc(files, func(a, b scanned) int { return strings.Compare(a.path, b.path) })
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
