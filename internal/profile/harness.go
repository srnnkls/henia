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
		if len(h.Config.Native) > 0 || h.Config.StatusLine != nil {
			return nil, fmt.Errorf("config.native needs a profile with [config] files")
		}
		return nil, nil
	}
	inputs, err := harnessInputs(h.Config)
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
	result, err := compiler.Render(h.Shared, inputs, h.Config.Native, context, schemas)
	if err != nil {
		return nil, err
	}
	if h.Config.StatusLine != nil && !compiler.ReadsHarness() {
		result.Omitted = append(result.Omitted, "statusline")
	}
	return result, nil
}

// harnessInputs adds the status line script a profile deploys as content: a
// file's text or the inline script, given a shebang when it has none.
func harnessInputs(config henia.HarnessConfig) (map[string]any, error) {
	inputs := config.Inputs()
	if config.StatusLine == nil {
		return inputs, nil
	}
	statusline := maps.Clone(config.StatusLine)
	script, _ := statusline["script"].(string)
	if file, ok := statusline["file"].(string); ok {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("config.statusline.file: %w", err)
		}
		script = string(data)
	}
	if script != "" {
		if !strings.HasPrefix(script, "#!") {
			script = "#!/bin/sh\n" + script
		}
		statusline["content"] = script
	}
	inputs["statusline"] = statusline
	return inputs, nil
}
