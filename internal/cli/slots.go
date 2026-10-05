package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/slots"
)

func newSlotsCommand() *cobra.Command {
	var flags runtimeFlags
	var check, explain, asJSON bool
	cmd := &cobra.Command{
		Use:   "slots [slot...]",
		Short: "Resolve the providers of skill slots across the library",
		Long: `Resolve the providers of skill slots across the library: the project's
skills, its skill packages and the global packages.

Prints "<slot>\t<tier>\thenia show <package>:<skill>" for every provider of a
requested slot, a dotted sub-slot of it or an enclosing slot, with a fourth
"\t<value>" column when the slot's type carries command, text or path values,
then problem rows whose tier is invalid, unknown, undeclared or conflict.
--check prints only the problem rows of every provider and fails when any
exist. --explain shows every provider of the requested slots with its status,
priority and origin, the provider that shadows it and the declaration that
types it, or of every slot without any; --json emits the same as JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if check && (len(args) > 0 || explain || asJSON) {
				return fmt.Errorf("--check takes no slots, --explain or --json")
			}
			if explain && asJSON {
				return fmt.Errorf("--explain and --json are exclusive")
			}
			if !check && !explain && !asJSON && len(args) == 0 {
				return nil
			}
			lib := flags.open(cmd)
			resolution, err := resolveSlots(flags.project, lib)
			if err != nil {
				return err
			}
			switch {
			case explain:
				return resolution.Explain(cmd.OutOrStdout(), args)
			case asJSON:
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(resolution.Select(args))
			}
			rows := resolution.Rows(args, check)
			for _, r := range rows {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), r.Line()); err != nil {
					return err
				}
			}
			if check && slices.ContainsFunc(rows, slots.Row.Problem) {
				return fmt.Errorf("slots found %d problem(s)", len(rows))
			}
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&check, "check", false, "Report problems of every provider and fail when any exist")
	cmd.Flags().BoolVar(&explain, "explain", false, "Show why each provider of the requested slots is selected, shadowed, disabled or unknown")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit declarations, providers and problems of the requested slots, or of all, as JSON")
	return cmd
}

func resolveSlots(project string, lib *library.Library) (*slots.Resolution, error) {
	path, err := config.Find(project)
	if errors.Is(err, os.ErrNotExist) {
		path, err = filepath.Join(project, library.ConfigFile), nil
	}
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadOptional(path)
	if err != nil {
		return nil, err
	}
	return slots.Evaluate(slots.Discover(lib), cfg.DisabledProviders()), nil
}

func projectRoot(cmd *cobra.Command) string {
	git := exec.CommandContext(cmd.Context(), "git", "rev-parse", "--show-toplevel")
	if out, err := git.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	wd, _ := os.Getwd()
	return wd
}
