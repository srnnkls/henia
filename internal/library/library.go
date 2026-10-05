// Package library catalogs the installed skill sources that the runtime serves.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/markup"
)

const (
	ProjectDir = ".henia"
	ConfigFile = "henia.toml"
	SkillsDir  = "skills"
	Project    = "project"
	Global     = "global"
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

func xdg(variable string, fallback ...string) string {
	if dir := os.Getenv(variable); filepath.IsAbs(dir) {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(append([]string{home}, fallback...)...)
}

func SourcesDir() string {
	return filepath.Join(xdg("XDG_DATA_HOME", ".local", "share"), "henia", "sources")
}

func CacheDir() string { return filepath.Join(xdg("XDG_CACHE_HOME", ".cache"), "henia") }

func ConfigDir() string { return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "henia") }

func (s Source) ProjectRoot() string {
	if filepath.Base(s.Root) == ProjectDir {
		return filepath.Dir(s.Root)
	}
	return s.Root
}

type Source struct {
	Name   string `json:"name"`
	Tier   string `json:"tier"`
	Root   string `json:"root"`
	Config string `json:"config,omitempty"`
}

func (s Source) Skills() string { return filepath.Join(s.Root, SkillsDir) }

type Entry struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Source      string             `json:"source"`
	Tier        string             `json:"tier"`
	Path        string             `json:"path"`
	Description string             `json:"description"`
	Digest      string             `json:"digest"`
	Sections    []markup.Section   `json:"sections"`
	Artifact    *artifact.Artifact `json:"-"`
	Origin      Source             `json:"-"`
}

type Library struct {
	Sources []Source
	Entries []Entry
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
		l.add(Source{Name: Project, Tier: Project, Root: root, Config: config})
	}
	roots := slices.Clone(extra)
	if entries, err := os.ReadDir(SourcesDir()); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				roots = append(roots, filepath.Join(SourcesDir(), entry.Name()))
			}
		}
	}
	for _, root := range roots {
		source := Source{Name: filepath.Base(root), Tier: Global, Root: root}
		if _, err := os.Stat(filepath.Join(root, ConfigFile)); err == nil {
			source.Config = filepath.Join(root, ConfigFile)
		}
		l.add(source)
	}
	return l
}

func (l *Library) add(source Source) {
	files := Scan(source.Skills())
	if len(files) == 0 {
		return
	}
	l.Sources = append(l.Sources, source)
	for _, f := range files {
		name := filepath.Base(filepath.Dir(f.Path))
		f.Artifact.Name, f.Artifact.Type = name, artifact.TypeSkill
		description, _ := f.Artifact.Frontmatter["description"].(string)
		sections, _ := markup.Sections([]byte(f.Artifact.Body))
		l.Entries = append(l.Entries, Entry{
			ID: source.Name + ":" + name, Name: name, Source: source.Name, Tier: source.Tier, Path: f.Path,
			Description: strings.TrimSpace(description), Digest: digest(filepath.Dir(f.Physical)),
			Sections: sections, Artifact: f.Artifact, Origin: source,
		})
	}
}

func (l *Library) Resolve(ref string) (Entry, error) {
	source, name, qualified := strings.Cut(ref, ":")
	if !qualified {
		name, source = ref, ""
	}
	var matches []Entry
	for _, e := range l.Entries {
		if e.Name == name && (source == "" || e.Source == source) {
			matches = append(matches, e)
		}
	}
	switch {
	case len(matches) == 0:
		return Entry{}, fmt.Errorf("no skill named %q in the library; henia ls lists them", ref)
	case len(matches) == 1, matches[0].Tier == Project:
		return matches[0], nil
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

const gitignore = "/*\n!/.gitignore\n!/henia.toml\n!/skills/\n!/harnesses/\n!/lint/\n"

func EnsureGitignore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(gitignore), 0o644)
}
