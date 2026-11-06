package lint

import (
	"fmt"
	"strings"

	"github.com/srnnkls/henia/internal/similarity"
)

// SemanticOptions enables local Model2Vec inference. Model names a Hugging Face
// repo id resolved through the local Hugging Face cache at Revision; ModelPath
// names a directory containing tokenizer.json, config.json and
// model.safetensors. No downloads, subprocesses or servers participate in
// linting.
type SemanticOptions struct {
	Enabled   bool   `toml:"enabled,omitempty"`
	Model     string `toml:"model,omitempty"`
	Revision  string `toml:"revision,omitempty"`
	ModelPath string `toml:"model_path,omitempty"`
}

func (o SemanticOptions) validate() error {
	model, path := strings.TrimSpace(o.Model) != "", strings.TrimSpace(o.ModelPath) != ""
	switch {
	case model && path:
		return fmt.Errorf("semantic.model and semantic.model_path are mutually exclusive")
	case o.Revision != "" && !model:
		return fmt.Errorf("semantic.revision requires semantic.model")
	case model && !similarity.IsRepoID(o.Model):
		return fmt.Errorf("semantic.model %q is not a Hugging Face repo id (org/name)", o.Model)
	case o.Enabled && !model && !path:
		return fmt.Errorf("semantic.model or semantic.model_path is required when semantic checks are enabled; name a Hugging Face repo id or a local Model2Vec model directory")
	}
	return nil
}

// Dir returns the local Model2Vec model directory.
func (o SemanticOptions) Dir() (string, error) {
	if o.Model != "" {
		return similarity.Snapshot(o.Model, o.Revision)
	}
	return o.ModelPath, nil
}

// Name identifies the model in diagnostics.
func (o SemanticOptions) Name() string {
	if o.Model == "" {
		return o.ModelPath
	}
	if o.Revision != "" {
		return o.Model + "@" + o.Revision
	}
	return o.Model
}
