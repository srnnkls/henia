package library

import (
	"fmt"
	"github.com/srnnkls/henia/internal/reference"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var moduleSegment = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func SplitAddress(target string) (skill string, modules []string, resource string, isResource bool, err error) {
	a, err := reference.ParseAddress(target)
	if err != nil {
		return "", nil, "", false, err
	}
	if a.Kind != reference.TypeSkill {
		return "", nil, "", false, fmt.Errorf("%s: the library serves skills, not %ss", target, a.Kind)
	}
	return a.Ref(), a.Modules(), a.Path(), a.Path() != "", nil
}

func ModulePath(skillDir string, modules []string) (string, error) {
	for _, m := range modules {
		if !moduleSegment.MatchString(m) {
			return "", fmt.Errorf("%q is not a module name; address the file by path instead", m)
		}
	}
	rel := filepath.Join(modules...)
	file := rel + ".md"
	_, fileErr := os.Stat(filepath.Join(skillDir, file))
	info, dirErr := os.Stat(filepath.Join(skillDir, rel))
	isDir := dirErr == nil && info.IsDir()
	switch {
	case fileErr == nil && isDir:
		return "", fmt.Errorf("%s names both %s and %s/; address one by path", strings.Join(modules, "."), filepath.ToSlash(file), filepath.ToSlash(rel))
	case fileErr == nil:
		return filepath.ToSlash(file), nil
	case isDir:
		return filepath.ToSlash(rel) + "/", nil
	}
	return "", fmt.Errorf("no module %s; henia show the skill to list its resources", strings.Join(modules, "."))
}

func Module(path string) (string, bool) {
	rel, ok := strings.CutSuffix(filepath.ToSlash(path), ".md")
	if !ok {
		return "", false
	}
	segments := strings.Split(rel, "/")
	for _, s := range segments {
		if !moduleSegment.MatchString(s) {
			return "", false
		}
	}
	return strings.Join(segments, "."), true
}

func Address(skill, path string) string {
	rel := strings.TrimSuffix(filepath.ToSlash(path), "/")
	if rel == "" {
		return skill
	}
	if module, ok := Module(rel); ok {
		return skill + "." + module
	}
	segments := strings.Split(rel, "/")
	for _, s := range segments {
		if !moduleSegment.MatchString(s) {
			return skill + "/" + filepath.ToSlash(path)
		}
	}
	return skill + "." + strings.Join(segments, ".")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)

func ShowCommand(address string) string {
	if shellSafe.MatchString(address) {
		return "henia show " + address
	}
	return "henia show '" + strings.ReplaceAll(address, "'", `'\''`) + "'"
}

func AddressedResource(skillDir, resource string) (string, error) {
	rel, isPath := strings.CutPrefix(resource, "/")
	if !isPath {
		module, err := ModulePath(skillDir, strings.Split(strings.TrimPrefix(resource, "."), "."))
		if err != nil {
			return "", err
		}
		if strings.HasSuffix(module, "/") {
			return "", fmt.Errorf("%s is a directory; address a file in it", module)
		}
		rel = module
	}
	return ResourcePath(skillDir, rel)
}
