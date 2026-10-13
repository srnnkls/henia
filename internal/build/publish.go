package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/target"
)

func validateCleanOutput(sources []string, output string) error {
	if output == "" {
		return fmt.Errorf("build output directory is required")
	}
	destination, err := resolvedPath(output)
	if err != nil {
		return err
	}
	for _, source := range sources {
		canonical, err := resolvedPath(source)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(destination, canonical)
		if err != nil {
			return err
		}
		if filepath.IsLocal(relative) {
			return fmt.Errorf("clean output %s contains canonical source %s", output, source)
		}
	}
	info, err := os.Lstat(output)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if info != nil && !info.IsDir() {
		return fmt.Errorf("clean output must be a real directory: %s", output)
	}
	return nil
}

func publish(ctx context.Context, output string, jobs []writeJob, result *Result) (*Result, error) {
	if len(jobs) == 0 {
		return result, nil
	}
	err := swap(output, func(stage string) error {
		for _, job := range jobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			tgt := target.NewFromConfig(job.target.Name(), filepath.Join(stage, job.target.Name()), job.harness)
			if err := tgt.Write(job.artifact); err != nil {
				return err
			}
			if job.artifact.Type != artifact.TypeUnknown {
				result.Built++
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Publish writes a complete output tree through write into a staging sibling
// and replaces output with it only when write succeeds.
func Publish(sources []string, output string, write func(stage string) error) error {
	if err := validateCleanOutput(sources, output); err != nil {
		return err
	}
	return swap(output, write)
}

func swap(output string, write func(stage string) error) error {
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".henia-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0755); err != nil {
		return err
	}
	if err := write(stage); err != nil {
		return err
	}
	backup, err := os.MkdirTemp(parent, ".henia-previous-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	hadOutput := false
	if err := os.Rename(output, backup); err == nil {
		hadOutput = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, output); err != nil {
		if hadOutput {
			err = errors.Join(err, os.Rename(backup, output))
		}
		return err
	}
	if hadOutput {
		return os.RemoveAll(backup)
	}
	return nil
}
