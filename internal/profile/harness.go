package profile

import (
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/srnnkls/henia"
)

// RenderHarness renders a harness's config files from its profile. It returns
// nil when the harness has no profile. Omitted names the harness inputs the
// profile never reads.
func RenderHarness(name, output string, h henia.Harness, schemas *Schemas) (*Result, error) {
	if h.Profile == "" {
		if len(h.Config) > 0 || h.StatusLine != nil {
			return nil, fmt.Errorf("config and statusline need a profile with [config] files")
		}
		return nil, nil
	}
	inputs, err := harnessInputs(h.StatusLine)
	if err != nil {
		return nil, err
	}
	loaded, err := Load(h.Profile, h.ProjectRoot, h.UserRoot)
	if err != nil {
		return nil, err
	}
	compiler, err := NewConfigCompiler(loaded.Config)
	if err != nil {
		return nil, err
	}
	context := Context{Name: name, Profile: h.Profile, Path: output, Variables: h.Variables, Tools: h.Tools, Keys: h.Frontmatter.Rename, Install: h.Install.Scope, InstallDir: h.Install.Dir}
	result, err := compiler.Render(h.Shared, inputs, h.Config, context, schemas)
	if err != nil {
		return nil, err
	}
	if h.StatusLine != nil && !compiler.ReadsHarness() {
		result.Omitted = append(result.Omitted, "statusline")
	}
	return result, nil
}

// harnessInputs is the data config expressions read as harness: the status
// line, with the script a profile deploys as content: a file's text or the
// inline script, given a shebang when it has none.
func harnessInputs(statusline map[string]any) (map[string]any, error) {
	if statusline == nil {
		return nil, nil
	}
	statusline = maps.Clone(statusline)
	script, _ := statusline["script"].(string)
	if file, ok := statusline["file"].(string); ok {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("statusline.file: %w", err)
		}
		script = string(data)
	}
	if script != "" {
		if !strings.HasPrefix(script, "#!") {
			script = "#!/bin/sh\n" + script
		}
		statusline["content"] = script
	}
	return map[string]any{"statusline": statusline}, nil
}
