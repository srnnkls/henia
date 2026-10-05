package build

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/defaults"
	"github.com/srnnkls/henia/internal/vendor"
)

const (
	CatalogName  = "henia"
	catalogTools = "Bash(henia ls *), Bash(henia show *), Bash(henia query *)"
)

var heniaSkill, catalogTemplate = func() (*artifact.Artifact, *template.Template) {
	art, err := artifact.Parse([]byte(defaults.HeniaSkill))
	if err != nil {
		panic(err)
	}
	return art, template.Must(template.New("henia").Parse(art.Body))
}()

type Entry struct{ Name, Description string }

func catalog(source string, all, built []*artifact.Artifact, h henia.Harness) (*artifact.Artifact, error) {
	if !h.Catalog() || !slices.Contains(h.Artifacts, "skills") && len(h.Artifacts) > 0 {
		return nil, nil
	}
	entries, err := dynamicEntries(all, built)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	return HeniaArtifact(source, entries, h)
}

func dynamicEntries(all, built []*artifact.Artifact) ([]Entry, error) {
	var entries []Entry
	for _, art := range all {
		if art.Type != artifact.TypeSkill {
			continue
		}
		if art.Name == CatalogName && slices.Contains(built, art) {
			return nil, fmt.Errorf("skill %q collides with the generated catalog skill; rename it or set catalog = false in [harness.<name>.skills]", CatalogName)
		}
		if !slices.Contains(built, art) {
			description, _ := art.Frontmatter["description"].(string)
			entries = append(entries, Entry{art.Name, strings.Join(strings.Fields(description), " ")})
		}
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return entries, nil
}

func HeniaArtifact(source string, entries []Entry, h henia.Harness) (*artifact.Artifact, error) {
	var body strings.Builder
	if err := catalogTemplate.Execute(&body, entries); err != nil {
		return nil, err
	}
	frontmatter := map[string]any{"name": heniaSkill.Frontmatter["name"], "description": heniaSkill.Frontmatter["description"]}
	if h.Profile != "" {
		if profile, err := vendor.Load(h.Profile, h.ProjectRoot, h.UserRoot); err == nil && slices.Contains(profile.Fields, "allowed-tools") {
			frontmatter["allowed-tools"] = catalogTools
		}
	}
	return &artifact.Artifact{
		Name:        CatalogName,
		Type:        artifact.TypeSkill,
		SourcePath:  filepath.Join(source, "skills", CatalogName),
		IsDirectory: true,
		Frontmatter: frontmatter,
		Body:        strings.TrimRight(body.String(), "\n") + "\n",
	}, nil
}
