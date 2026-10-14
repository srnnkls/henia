package build

import (
	"fmt"
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
	if file, ok := h.Config.StatusLine["file"].(string); ok {
		info, err := os.Stat(file)
		if err != nil {
			return nil, fmt.Errorf("config.statusline.file: %w", err)
		}
		if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			return nil, fmt.Errorf("config.statusline.file: %s is not an executable file", file)
		}
	}
	loaded, err := profile.Load(h.Profile, h.ProjectRoot, h.UserRoot)
	if err != nil {
		return nil, err
	}
	compiler, err := profile.NewConfigCompiler(loaded.Config)
	if err != nil {
		return nil, err
	}
	context := profile.Context{Name: name, Profile: h.Profile, Path: output, Variables: h.Variables, Tools: h.Tools, Keys: h.Frontmatter.Rename}
	result, err := compiler.Render(h.Shared, h.Config.Inputs(), h.Config.Native, context, schemas)
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
	return &artifact.Artifact{Name: "config", Type: artifact.TypeConfig, SourcePath: source, Files: result.Files, Warnings: result.Warnings}, nil
}
