package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
)

func newBuildCommand() *cobra.Command {
	var output string
	var selected []string
	cmd := &cobra.Command{
		Use:   "build [source-directory]",
		Short: "Compile local canonical artifacts for multiple harnesses",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := "."
			if len(args) > 0 {
				source = args[0]
			}
			info, err := os.Stat(source)
			if err != nil {
				return fmt.Errorf("source: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("source must be a directory containing skills, commands or agents")
			}
			cfg, err := optionalConfig(cmd)
			if err != nil {
				return err
			}
			harnesses := cfg.Harness
			if len(harnesses) == 0 {
				return fmt.Errorf("no harnesses configured")
			}
			if len(selected) > 0 {
				harnesses = make(map[string]henia.Harness, len(selected))
				for _, name := range selected {
					h, ok := cfg.Harness[name]
					if !ok {
						return fmt.Errorf("unknown harness %q", name)
					}
					harnesses[name] = h
				}
			}
			for name, h := range harnesses {
				if len(h.Artifacts) == 0 {
					h.Artifacts = cfg.Artifacts
				}
				harnesses[name] = h
			}
			destination := output
			if !cmd.Flags().Changed("output") {
				destination = cfg.Build.Output
			}
			result, err := build.Run(cmd.Context(), []string{source}, destination, harnesses)
			if err != nil {
				return err
			}
			for _, warning := range result.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", warning)
			}
			if err := errors.Join(result.Errors...); err != nil {
				return err
			}
			if result.Built == 0 {
				return fmt.Errorf("no artifacts found for selected harnesses")
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Built %d artifact(s) in %s\n", result.Built, destination)
			return err
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", ".henia/build", "Build output directory (overrides [build].output)")
	cmd.Flags().StringSliceVar(&selected, "harness", nil, "Harnesses to build (comma-separated; default all configured)")
	return cmd
}

func optionalConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load(configPath)
	if err == nil {
		return cfg, nil
	}
	if cmd.Flags().Changed("config") || !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return config.LoadOptional(configPath)
}
