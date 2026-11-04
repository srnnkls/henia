// Package library catalogs the installed skill packages that the runtime serves.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/deps"
	"github.com/srnnkls/henia/internal/markup"
)

const (
	ProjectDir = ".henia"
	ConfigFile = "henia.toml"
	SkillsDir  = "skills"
	Project    = "project"
	Dependency = "dependency"
	Global     = "global"
	LockFile   = "henia.lock"
)

type File struct {
	Path     string
	Physical string
	Artifact *artifact.Artifact
}

func Scan(dir string) []File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []File
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		physical, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		if info, err := os.Stat(physical); err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(physical)
		if err != nil {
			continue
		}
		art, err := artifact.Parse(data)
		if err != nil {
			continue
		}
		files = append(files, File{Path: path, Physical: physical, Artifact: art})
	}
	return files
}

func dir(home func() string) string {
	xdg.Reload()
	return filepath.Join(home(), "henia")
}

func legacyDir() string { return filepath.Join(dir(func() string { return xdg.DataHome }), "sources") }

func CacheDir() string { return dir(func() string { return xdg.CacheHome }) }

func ConfigDir() string { return dir(func() string { return xdg.ConfigHome }) }

func (s Package) ProjectRoot() string {
	if filepath.Base(s.Root) == ProjectDir {
		return filepath.Dir(s.Root)
	}
	return s.Root
}

type Package struct {
	Name   string `json:"name"`
	Tier   string `json:"tier"`
	Root   string `json:"root"`
	Config string `json:"config,omitempty"`
}

func (s Package) Skills() string { return filepath.Join(s.Root, SkillsDir) }

type Entry struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Package     string             `json:"package"`
	Tier        string             `json:"tier"`
	Path        string             `json:"path"`
	Description string             `json:"description"`
	Digest      string             `json:"digest"`
	Sections    []markup.Section   `json:"sections"`
	Artifact    *artifact.Artifact `json:"-"`
	Origin      Package            `json:"-"`
	physical    string
}

type Library struct {
	Packages []Package
	Entries  []Entry
}

func Open(project string, extra []string) *Library {
	l := &Library{}
	if project != "" {
		root := filepath.Join(project, ProjectDir)
		config := ""
		for _, candidate := range []string{filepath.Join(root, ConfigFile), filepath.Join(project, ConfigFile)} {
			if _, err := os.Stat(candidate); err == nil {
				config = candidate
				break
			}
		}
		l.add(Package{Name: Project, Tier: Project, Root: root, Config: config})
		l.dependencies(project, config)
	}
	for _, root := range extra {
		l.add(readPackage(root, Global))
	}
	for _, dir := range []string{PackagesDir("global"), legacyDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if (entry.IsDir() || entry.Type()&os.ModeSymlink != 0) && !l.has(entry.Name()) {
				l.add(readPackage(filepath.Join(dir, entry.Name()), Global))
			}
		}
	}
	return l
}

func readPackage(root, tier string) Package {
	s := Package{Name: filepath.Base(root), Tier: tier, Root: root}
	for _, candidate := range []string{filepath.Join(root, ConfigFile), filepath.Join(root, ProjectDir, ConfigFile)} {
		if _, err := os.Stat(candidate); err == nil {
			s.Config = candidate
			break
		}
	}
	return s
}

func PackagesDir(scope string) string {
	return filepath.Join(dir(func() string { return xdg.DataHome }), "packages", scope)
}

func InstallManifest(harness string) string {
	return filepath.Join(dir(func() string { return xdg.StateHome }), "install", harness+".json")
}

func StateDir(scope string) string {
	return filepath.Join(dir(func() string { return xdg.StateHome }), "packages", scope)
}

func ProjectKey(project string) string {
	absolute, err := filepath.Abs(project)
	if err != nil {
		absolute = project
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	sum := sha256.Sum256([]byte(absolute))
	return filepath.Base(absolute) + "-" + hex.EncodeToString(sum[:])[:12]
}

func (l *Library) dependencies(project, manifest string) {
	key := ProjectKey(project)
	if manifest != "" {
		key = StoreKey(project, manifest)
	}
	dir := PackagesDir(key)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	Touch(key)
	for _, entry := range entries {
		if root := filepath.Join(dir, entry.Name()); entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			l.add(readPackage(root, Dependency))
		}
	}
}

// StoreKey names a project's package store and state. Checkouts whose
// declared dependencies and locks agree share one store, lock-<digest>, so a
// checkout that switches locks deploys a fresh store instead of rewriting one
// in place. A path dependency deploys a working copy that no lock records, so
// its store stays with the checkout.
func StoreKey(project, manifest string) string {
	digest := sha256.New()
	declares := false
	for _, file := range []string{manifest, deps.Local(manifest)} {
		declared, err := deps.Read(file)
		if err != nil {
			return ProjectKey(project)
		}
		declares = declares || len(declared) > 0
		for _, name := range slices.Sorted(maps.Keys(declared)) {
			if declared[name].Path != "" {
				return ProjectKey(project)
			}
			fmt.Fprintf(digest, "%s %+v\n", name, declared[name])
		}
	}
	if !declares {
		return ProjectKey(project)
	}
	lock := filepath.Join(filepath.Dir(manifest), LockFile)
	for _, file := range []string{lock, deps.Local(lock)} {
		data, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ProjectKey(project)
		}
		fmt.Fprintf(digest, "%d\n", len(data))
		digest.Write(data)
	}
	return "lock-" + hex.EncodeToString(digest.Sum(nil))[:16]
}

// Touch marks the store and state of key as in use, which keeps PruneStores
// from removing them.
func Touch(key string) {
	now := time.Now()
	for _, dir := range []string{PackagesDir(key), StateDir(key)} {
		_ = os.Chtimes(dir, now, now)
	}
}

// PruneStores removes every project store and state untouched for longer
// than unused, keeping keep and the global store.
func PruneStores(keep string, unused time.Duration) error {
	cutoff := time.Now().Add(-unused)
	for _, root := range []string{filepath.Dir(PackagesDir(keep)), filepath.Dir(StateDir(keep))} {
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == keep || entry.Name() == "global" {
				continue
			}
			info, err := entry.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Library) has(name string) bool {
	return slices.ContainsFunc(l.Packages, func(p Package) bool { return p.Name == name })
}

func (l *Library) add(pkg Package) {
	files := Scan(pkg.Skills())
	if len(files) == 0 {
		return
	}
	l.Packages = append(l.Packages, pkg)
	for _, f := range files {
		name := filepath.Base(filepath.Dir(f.Path))
		f.Artifact.Name, f.Artifact.Type = name, artifact.TypeSkill
		description, _ := f.Artifact.Frontmatter["description"].(string)
		l.Entries = append(l.Entries, Entry{
			ID: pkg.Name + ":" + name, Name: name, Package: pkg.Name, Tier: pkg.Tier, Path: f.Path,
			Description: strings.TrimSpace(description), Artifact: f.Artifact, Origin: pkg, physical: f.Physical,
		})
	}
}

func (l *Library) Index() {
	for i := range l.Entries {
		e := &l.Entries[i]
		e.Digest = digest(filepath.Dir(e.physical))
		e.Sections, _ = markup.Sections([]byte(e.Artifact.Body))
	}
}

func (l *Library) Resolve(ref string) (Entry, error) {
	pkg, name, qualified := strings.Cut(ref, ":")
	if !qualified {
		name, pkg = ref, ""
	}
	var matches []Entry
	for _, e := range l.Entries {
		if e.Name == name && (pkg == "" || e.Package == pkg) {
			matches = append(matches, e)
		}
	}
	switch {
	case len(matches) == 0:
		return Entry{}, fmt.Errorf("no skill named %q in the library; henia ls lists them", ref)
	case len(matches) == 1:
		return matches[0], nil
	}
	for _, tier := range []string{Project, Dependency} {
		if found := slices.DeleteFunc(slices.Clone(matches), func(e Entry) bool { return e.Tier != tier }); len(found) == 1 {
			return found[0], nil
		}
	}
	var ids []string
	for _, e := range matches {
		ids = append(ids, e.ID)
	}
	return Entry{}, fmt.Errorf("%q is ambiguous; name one of %s", ref, strings.Join(ids, ", "))
}

func (l *Library) Reference(e Entry) string {
	if resolved, err := l.Resolve(e.Name); err == nil && resolved.ID == e.ID {
		return e.Name
	}
	return e.ID
}

func (l *Library) Names() map[string]bool {
	names := make(map[string]bool, len(l.Entries))
	for _, e := range l.Entries {
		names[e.Name] = true
	}
	return names
}

func Section(body, anchor string) (markup.Section, string, bool) {
	sections, err := markup.Sections([]byte(body))
	if err != nil {
		return markup.Section{}, "", false
	}
	for _, section := range sections {
		if section.Anchor == anchor {
			return section, body[section.Start:section.End], true
		}
	}
	return markup.Section{}, "", false
}

func digest(dir string) string {
	h := sha256.New()
	var files []string
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	slices.Sort(files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(dir, path)
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

const gitignore = "/*\n!/.gitignore\n!/henia.toml\n!/henia.lock\n!/skills/\n!/harnesses/\n!/lint/\n!/components/\n"

func EnsureGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(gitignore), 0o644)
}
