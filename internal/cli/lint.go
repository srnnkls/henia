package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/lint"
)

func newLintCommand() *cobra.Command {
	var format string
	var strict bool
	var disabled []string
	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Check skill metadata, references, duplication and freshness",
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "json" {
				return fmt.Errorf("unknown lint format %q (use text or json)", format)
			}
			if len(args) == 0 {
				args = []string{"."}
			}
			var options lint.Options
			cfg, err := config.Load(configPath)
			if err == nil {
				options = cfg.Lint
			} else if cmd.Flags().Changed("config") || !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("load config: %w", err)
			}
			options.Disable = append(options.Disable, disabled...)
			diagnostics, err := lint.Run(cmd.Context(), args, options)
			if err != nil {
				return err
			}
			if format == "json" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(diagnostics); err != nil {
					return err
				}
			} else {
				for _, d := range diagnostics {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:%d:%d: %s [%s] %s\n", d.Path, d.Line, d.Column, d.Severity, d.Rule, d.Message); err != nil {
						return err
					}
				}
			}
			for _, d := range diagnostics {
				if d.Severity == "error" || strict {
					return fmt.Errorf("lint found %d diagnostic(s)", len(diagnostics))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Diagnostic output: text or json")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit unsuccessfully on warnings as well as errors")
	cmd.Flags().StringSliceVar(&disabled, "disable", nil, "Disable lint rules (comma-separated)")
	return cmd
}
