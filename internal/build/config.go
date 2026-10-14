package build

import (
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/profile"
)

func configArtifact(name, output string, h henia.Harness, source string, schemas *profile.Schemas) (*artifact.Artifact, error) {
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
	loaded, err := profile.Load(h.Profile, h.ProjectRoot, h.UserRoot)
	if err != nil {
		return nil, err
	}
	compiler, err := profile.NewConfigCompiler(loaded.Config)
	if err != nil {
		return nil, err
	}
	context := profile.Context{Name: name, Profile: h.Profile, Path: output, Variables: h.Variables, Tools: h.Tools, Keys: h.Frontmatter.Rename, Install: h.Install.Scope, InstallDir: h.Install.Dir}
	result, err := compiler.Render(h.Shared, inputs, h.Config.Native, context, schemas)
	if err != nil {
		return nil, err
	}
	if h.Config.StatusLine != nil && !compiler.ReadsHarness() {
		result.Warnings = append(result.Warnings, "profile "+h.Profile+" has no status line; config.statusline omitted")
	}
	if h.Strict && len(result.Warnings) > 0 {
		return nil, fmt.Errorf("invalid config: %s", strings.Join(result.Warnings, "; "))
	}
	if len(result.Files) == 0 {
		return nil, nil
	}
	return &artifact.Artifact{Name: "config", Type: artifact.TypeConfig, SourcePath: source, Files: result.Files, Modes: result.Modes, Warnings: result.Warnings}, nil
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
