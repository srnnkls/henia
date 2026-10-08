package components

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const Dir = "components"

const InvalidComponent = "invalid-component"

type Problem struct {
	ID      string
	Path    string
	Message string
}

func Load(project, user string) (map[string]Declaration, []Problem) {
	decls := Builtins()
	var problems []Problem
	var dirs []string
	if user != "" {
		dirs = append(dirs, filepath.Join(user, Dir))
	}
	if project != "" {
		dirs = append(dirs, filepath.Join(project, ".henia", Dir))
	}
	for _, dir := range dirs {
		problems = append(problems, loadDir(dir, decls)...)
	}
	return decls, problems
}

func loadDir(dir string, decls map[string]Declaration) []Problem {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []Problem{{ID: InvalidComponent, Path: dir, Message: err.Error()}}
	}
	var problems []Problem
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok || entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		decl, err := read(name, path)
		if err != nil {
			problems = append(problems, Problem{ID: InvalidComponent, Path: path, Message: err.Error()})
			continue
		}
		decls[name] = decl
	}
	return problems
}

func read(name, path string) (Declaration, error) {
	if Reserved(name) {
		return Declaration{}, fmt.Errorf("%s is a built-in component", name)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return Declaration{}, err
	}
	decl, err := Parse(name, source)
	if err != nil {
		return Declaration{}, err
	}
	decl.Path = path
	if name == Head {
		if !slices.Equal(decl.Forms, []string{Container}) {
			return Declaration{}, fmt.Errorf("head must have form %s", Container)
		}
		decl.Builtin = true
	}
	return decl, nil
}
