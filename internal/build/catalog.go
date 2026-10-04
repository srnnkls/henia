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
	CatalogName        = "henia"
	catalogDescription = "Skills served by Henia beyond those installed here: lists them and reads one on demand with henia show. Use when no installed skill fits the task."
	catalogTools       = "Bash(henia ls *), Bash(henia show *)"
)

var catalogTemplate = template.Must(template.New("catalog").Parse(defaults.CatalogBody))

func catalog(source string, all, built []*artifact.Artifact, h henia.Harness) (*artifact.Artifact, error) {
	if !h.Catalog() || !slices.Contains(h.Artifacts, "skills") && len(h.Artifacts) > 0 {
		return nil, nil
	}
	var dynamic []*artifact.Artifact
	for _, art := range all {
		if art.Type != artifact.TypeSkill {
			continue
		}
		if art.Name == CatalogName && slices.Contains(built, art) {
			return nil, fmt.Errorf("skill %q collides with the generated catalog skill; rename it or set catalog = false in [harness.<name>.skills]", CatalogName)
		}
		if !slices.Contains(built, art) {
			dynamic = append(dynamic, art)
		}
	}
	if len(dynamic) == 0 {
		return nil, nil
	}
	slices.SortFunc(dynamic, func(a, b *artifact.Artifact) int { return strings.Compare(a.Name, b.Name) })
	type entry struct{ Name, Description string }
	var entries []entry
	for _, art := range dynamic {
		description, _ := art.Frontmatter["description"].(string)
		entries = append(entries, entry{art.Name, strings.Join(strings.Fields(description), " ")})
	}
	var body strings.Builder
	if err := catalogTemplate.Execute(&body, entries); err != nil {
		return nil, err
	}
	frontmatter := map[string]any{"name": CatalogName, "description": catalogDescription}
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
		Body:        body.String(),
	}, nil
}
