package lint

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/similarity"
)

// SemanticOptions enables local Model2Vec inference. ModelPath is a directory
// containing tokenizer.json, config.json and model.safetensors. No downloads,
// subprocesses or servers participate in linting.
type SemanticOptions struct {
	Enabled   bool    `toml:"enabled,omitempty"`
	ModelPath string  `toml:"model_path,omitempty"`
	Threshold float64 `toml:"threshold,omitempty"`
}

func (o SemanticOptions) validate() error {
	if math.IsNaN(o.Threshold) || o.Threshold < 0 || o.Threshold > 1 {
		return fmt.Errorf("semantic.threshold must be between 0 and 1")
	}
	if o.Enabled && strings.TrimSpace(o.ModelPath) == "" {
		return fmt.Errorf("semantic.model_path is required when semantic checks are enabled; use a local Model2Vec model directory")
	}
	if o.Enabled && o.Threshold == 0 {
		return fmt.Errorf("semantic.threshold must be set above 0 when semantic checks are enabled; calibrate it for the selected model")
	}
	return nil
}

func (c *checker) checkSemantic(ctx context.Context) error {
	if !c.options.Semantic.Enabled || slices.Contains(c.options.Disable, "semantic-content") || len(c.similarParagraphs) < 2 {
		return nil
	}
	if !slices.ContainsFunc(c.similarParagraphs[1:], func(p paragraph) bool { return !p.lexicalMatch }) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encode, err := similarity.Model(c.options.Semantic.ModelPath)
	if err != nil {
		return err
	}
	return c.compareSemantic(ctx, encode)
}

func (c *checker) compareSemantic(ctx context.Context, encode func(string) []float32) error {
	vectors := make([][]float64, 0, len(c.similarParagraphs))
	dimension := 0
	for _, current := range c.similarParagraphs {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw := encode(current.text)
		if dimension == 0 {
			dimension = len(raw)
		}
		if dimension == 0 || len(raw) != dimension {
			return fmt.Errorf("semantic model returned inconsistent or empty embedding dimensions")
		}
		vector, err := similarity.Unit(raw)
		if err != nil {
			return fmt.Errorf("embed %s:%d: %w", current.location.Path, current.location.Line, err)
		}
		if vector == nil && !current.lexicalMatch {
			c.diagnostics = append(c.diagnostics, Diagnostic{
				Path: current.location.Path, Line: current.location.Line, Column: current.location.Column,
				Severity: "warning", Rule: "semantic-content", Method: "cosine", Model: c.options.Semantic.ModelPath,
				Message: "paragraph has a zero embedding (possibly no known tokens); semantic comparison is unavailable",
			})
		}
		if vector != nil && !current.lexicalMatch {
			bestIndex, bestScore := -1, -1.0
			for j, previous := range vectors {
				if j%128 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if previous == nil {
					continue
				}
				score := similarity.Cosine(vector, previous)
				if score >= c.options.Semantic.Threshold && score > bestScore {
					bestIndex, bestScore = j, score
				}
			}
			if bestIndex >= 0 {
				first := c.similarParagraphs[bestIndex].location
				c.diagnostics = append(c.diagnostics, Diagnostic{
					Path: current.location.Path, Line: current.location.Line, Column: current.location.Column,
					Severity: "warning", Rule: "semantic-content", Method: "cosine", Model: c.options.Semantic.ModelPath,
					Message: fmt.Sprintf("paragraph may express similar instructions to %s:%d (cosine %.3f); review meaning and constraints", first.Path, first.Line, bestScore),
					Related: []Location{first}, Similarity: new(bestScore),
				})
			}
		}
		vectors = append(vectors, vector)
	}
	return ctx.Err()
}
