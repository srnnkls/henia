package lint

import (
	"fmt"
	"strings"
)

// SemanticOptions enables local Model2Vec inference. ModelPath is a directory
// containing tokenizer.json, config.json and model.safetensors. No downloads,
// subprocesses or servers participate in linting.
type SemanticOptions struct {
	Enabled   bool   `toml:"enabled,omitempty"`
	ModelPath string `toml:"model_path,omitempty"`
}

func (o SemanticOptions) validate() error {
	if o.Enabled && strings.TrimSpace(o.ModelPath) == "" {
		return fmt.Errorf("semantic.model_path is required when semantic checks are enabled; use a local Model2Vec model directory")
	}
	return nil
}
