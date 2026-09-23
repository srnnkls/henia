package target

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
)

type Target interface {
	Name() string
	Path() string
	TargetPath(art *artifact.Artifact) string
	Exists(art *artifact.Artifact) (bool, string)
	Write(art *artifact.Artifact) error
}

type HarnessTarget struct {
	name      string
	basePath  string
	structure string // "flat" or "nested" (default)
}

func New(basePath string) *HarnessTarget {
	return &HarnessTarget{basePath: basePath}
}

func NewFromConfig(name, output string, h henia.Harness) Target {
	return &HarnessTarget{name: name, basePath: output, structure: h.Structure}
}

func (t *HarnessTarget) Name() string { return t.name }
func (t *HarnessTarget) Path() string { return t.basePath }

func (t *HarnessTarget) TargetPath(art *artifact.Artifact) string {
	typeDir := artifact.TypeDirName(art.Type)
	if t.structure == "flat" {
		return filepath.Join(t.basePath, typeDir, art.FullName()+".md")
	}
	mainFile := artifact.MainFileName(art.Type)
	return filepath.Join(t.basePath, typeDir, art.FullName(), mainFile)
}

func (t *HarnessTarget) Exists(art *artifact.Artifact) (bool, string) {
	path := t.TargetPath(art)
	_, err := os.Stat(path)
	return err == nil, path
}

// OutputPaths lists every file that will be written, including resources and sidecars.
func OutputPaths(t Target, art *artifact.Artifact) ([]string, error) {
	main := t.TargetPath(art)
	var paths []string
	if art.Type != artifact.TypeUnknown {
		paths = append(paths, main)
	}
	resourceDir := filepath.Dir(main)
	if filepath.Base(main) != artifact.MainFileName(art.Type) {
		resourceDir = filepath.Join(resourceDir, art.FullName())
	}
	resources, err := resourceFiles(art)
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		paths = append(paths, filepath.Join(resourceDir, resource.path))
	}
	for _, name := range slices.Sorted(maps.Keys(art.Files)) {
		if !filepath.IsLocal(name) || filepath.Clean(name) != name || name == "." || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("invalid generated file path %q", name)
		}
		paths = append(paths, filepath.Join(t.Path(), name))
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			return nil, fmt.Errorf("output collision at %s (main file, resource or generated sidecar)", path)
		}
		seen[path] = true
	}
	return paths, nil
}

func (t *HarnessTarget) Write(art *artifact.Artifact) error {
	if _, err := OutputPaths(t, art); err != nil {
		return err
	}
	if err := os.MkdirAll(t.basePath, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(t.basePath)
	if err != nil {
		return err
	}
	defer root.Close()
	main, err := filepath.Rel(t.basePath, t.TargetPath(art))
	if err != nil {
		return err
	}
	write := func(path string, data []byte, mode os.FileMode) error {
		if err := root.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return root.WriteFile(path, data, mode)
	}
	if art.Type != artifact.TypeUnknown {
		if err := write(main, []byte(art.Render()), 0644); err != nil {
			return err
		}
	}
	resourceDir := filepath.Dir(main)
	if t.structure == "flat" {
		resourceDir = filepath.Join(resourceDir, art.FullName())
	}
	resources, err := resourceFiles(art)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		data, err := os.ReadFile(filepath.Join(art.SourcePath, resource.path))
		if err != nil {
			return err
		}
		if err := write(filepath.Join(resourceDir, resource.path), data, resource.mode); err != nil {
			return err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(art.Files)) {
		if err := write(name, art.Files[name], 0644); err != nil {
			return err
		}
	}
	return nil
}

type resourceFile struct {
	path string
	mode os.FileMode
}

// resourceFiles lists a directory artifact's resource files by their paths
// below the artifact, following symlinked files and directories.
func resourceFiles(art *artifact.Artifact) ([]resourceFile, error) {
	if !art.IsDirectory {
		return nil, nil
	}
	var files []resourceFile
	for _, resource := range art.Resources {
		if !filepath.IsLocal(resource) {
			return nil, fmt.Errorf("invalid resource path %q", resource)
		}
		err := artifact.Walk(filepath.Join(art.SourcePath, resource), func(path string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("resource is not a regular file: %s", path)
			}
			relative, err := filepath.Rel(art.SourcePath, path)
			if err != nil {
				return err
			}
			files = append(files, resourceFile{relative, info.Mode().Perm()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
