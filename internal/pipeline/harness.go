package pipeline

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/profile"
	"github.com/srnnkls/henia/internal/transform"
)

func Transformer(name, output string, h henia.Harness, kind artifact.Type) (henia.Harness, *transform.Transformer, error) {
	var loaded profile.Harness
	if h.Profile != "" {
		var err error
		if loaded, err = profile.Load(h.Profile, h.ProjectRoot, h.UserRoot); err != nil {
			return h, nil, fmt.Errorf("harness %s: %w", name, err)
		}
		if h.Directives == "" {
			h.Directives = loaded.Directives
		}
	}
	mapping := h.Type(artifact.TypeDirName(kind))
	if mapping.Profile != "" && mapping.Profile != h.Profile {
		h.Profile = mapping.Profile
		var err error
		if loaded, err = profile.Load(h.Profile, h.ProjectRoot, h.UserRoot); err != nil {
			return h, nil, fmt.Errorf("harness %s: %w", name, err)
		}
	}
	if mapping.Layout != "" {
		h.Layout = mapping.Layout
	}
	h.Frontmatter.Rename = mergeMap(h.Frontmatter.Rename, mapping.Frontmatter.Rename)
	h.Frontmatter.Values = mergeMap(h.Frontmatter.Values, mapping.Frontmatter.Values)
	section := loaded.Section(string(kind), mapping.Profile != "")
	if section == nil && (kind != artifact.TypeCommand || mapping.Profile == "") {
		h.Profile = ""
	}
	tr := &transform.Transformer{
		Profile: h.Profile, Strict: h.Strict, Variables: h.Variables,
		OutputFormat: markupFormat(h.Directives), Keys: h.Frontmatter.Rename, Values: h.Frontmatter.Values,
		Tools: h.Tools, References: References(h.References),
	}
	if section != nil {
		var err error
		if tr.Compiler, err = profile.NewCompiler(*section); err != nil {
			return h, nil, fmt.Errorf("harness %s: %w", name, err)
		}
		tr.Context = profile.Context{Name: name, Profile: h.Profile, Path: output, Variables: h.Variables, Tools: h.Tools, Keys: h.Frontmatter.Rename}
	}
	return h, tr, nil
}

func Renderer(art *artifact.Artifact, name string, h henia.Harness, served map[string]bool, head bool) (*transform.Transformer, error) {
	_, tr, err := Transformer(name, "", h, art.Type)
	if err != nil {
		return nil, err
	}
	tr.Served, tr.Head, tr.LibraryLinks = served, head, true
	return tr, nil
}

func Projected(arts []*artifact.Artifact, harness henia.Harness) []*artifact.Artifact {
	allowed := harness.Artifacts
	if len(allowed) == 0 {
		allowed = []string{"skills", "commands", "agents"}
	}
	types := make(map[string]bool)
	for _, t := range allowed {
		types[strings.TrimSuffix(t, "s")] = true
	}
	var projected []*artifact.Artifact
	for _, art := range arts {
		if types[string(art.Type)] && harness.Builds(artifact.TypeDirName(art.Type), art.Name) {
			projected = append(projected, art)
		}
	}
	return projected
}

func Served(all, projected []*artifact.Artifact) map[string]bool {
	served := make(map[string]bool)
	for _, art := range all {
		if art.Type == artifact.TypeSkill && (!slices.Contains(projected, art) || !Invocable(art.Frontmatter)) {
			served[art.FullName()] = true
		}
	}
	return served
}

func Invocable(frontmatter map[string]any) bool {
	auto, set := heniaSettings(frontmatter)["auto_invoke"].(bool)
	return !set || auto
}

func References(refs map[string]string) map[string]transform.ReferenceConfig {
	if refs == nil {
		return nil
	}
	result := make(map[string]transform.ReferenceConfig)
	for k, v := range refs {
		result[k] = transform.ReferenceConfig{Output: v}
	}
	return result
}

func ReferenceSyntax(template string) *regexp.Regexp {
	prefix, suffix, ok := strings.Cut(template, "{{.Name}}")
	if !ok || strings.Contains(prefix+suffix, "{{") {
		return nil
	}
	pattern := regexp.QuoteMeta(prefix) + `([a-z0-9][a-z0-9._-]*)` + regexp.QuoteMeta(suffix)
	if !strings.ContainsAny(template, "*[]") {
		pattern = "`" + pattern + "`"
	}
	return regexp.MustCompile(pattern)
}

func markupFormat(directives string) string {
	switch directives {
	case "keep":
		return "directives"
	case "md":
		return "markdown"
	}
	return directives
}

func mergeMap[V any](base, override map[string]V) map[string]V {
	result := make(map[string]V, len(base)+len(override))
	maps.Copy(result, base)
	maps.Copy(result, override)
	return result
}
