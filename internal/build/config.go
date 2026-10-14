package build

import (
	"fmt"
	"strings"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/profile"
)

func configArtifact(name, output string, h henia.Harness, source string, schemas *profile.Schemas) (*artifact.Artifact, error) {
	result, err := profile.RenderHarness(name, output, h, schemas)
	if err != nil || result == nil {
		return nil, err
	}
	for _, input := range result.Omitted {
		result.Warnings = append(result.Warnings, "profile "+h.Profile+" has no status line; config."+input+" omitted")
	}
	if h.Strict && len(result.Warnings) > 0 {
		return nil, fmt.Errorf("invalid config: %s", strings.Join(result.Warnings, "; "))
	}
	if len(result.Files) == 0 {
		return nil, nil
	}
	return &artifact.Artifact{Name: "config", Type: artifact.TypeConfig, SourcePath: source, Files: result.Files, Modes: result.Modes, Warnings: result.Warnings}, nil
}
