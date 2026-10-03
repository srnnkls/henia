package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/slots"
)

func newSlotsCommand() *cobra.Command {
	var globals []string
	var project string
	var applying []string
	var check, explain, asJSON bool
	cmd := &cobra.Command{
		Use:   "slots [slot...]",
		Short: "Resolve the installed providers of skill slots",
		Long: `Resolve the installed providers of skill slots.

Prints "<slot>\t<tier>\t<SKILL.md>" for every provider of a requested slot, a
dotted sub-slot of it or an enclosing slot, with a fourth "\t<value>" column
when the slot's type carries command, text or path values, then problem rows whose tier is
invalid, unknown, undeclared or conflict. --check prints only the problem rows
of every provider and fails when any exist. --explain shows every provider of
the requested slots with its status, priority and its origin, the provider that
shadows it and the declaration that types it, or of every slot without any;
--json emits the same as JSON.

--for <skill> adds the slots that skill's metadata.applies names to the request.

Without --project, project skills are read from the Git top level, else the
working directory.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if check && (len(args) > 0 || explain || asJSON) {
				return fmt.Errorf("--check takes no slots, --explain or --json")
			}
			if explain && asJSON {
				return fmt.Errorf("--explain and --json are exclusive")
			}
			if check && len(applying) > 0 {
				return fmt.Errorf("--check takes no --for")
			}
			if !check && !explain && !asJSON && len(args) == 0 && len(applying) == 0 {
				return nil
			}
			if project == "" {
				project = projectRoot(cmd)
			}
			skills := slots.Discover(project, globals, library.Open(project, nil).Sources)
			for _, name := range applying {
				applied, found := slots.Applied(skills, name)
				if !found {
					return fmt.Errorf("no installed skill named %q", name)
				}
				args = append(args, applied...)
			}
			resolution := slots.Evaluate(skills)
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
				line := r.Slot + "\t" + r.Kind + "\t" + r.Path
				if r.Value != "" {
					line += "\t" + r.Value
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
					return err
				}
			}
			if check && slices.ContainsFunc(rows, slots.Row.Problem) {
				return fmt.Errorf("slots found %d problem(s)", len(rows))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&globals, "global", nil, "Global skills directory (repeatable)")
	cmd.Flags().StringVar(&project, "project", "", "Project root whose skill directories are read")
	cmd.Flags().StringArrayVar(&applying, "for", nil, "Resolve the slots that metadata.applies of this skill names (repeatable)")
	cmd.Flags().BoolVar(&check, "check", false, "Report problems of every provider and fail when any exist")
	cmd.Flags().BoolVar(&explain, "explain", false, "Show why each provider of the requested slots is selected, shadowed or unknown")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit declarations, providers and problems of the requested slots, or of all, as JSON")
	return cmd
}

func projectRoot(cmd *cobra.Command) string {
	git := exec.CommandContext(cmd.Context(), "git", "rev-parse", "--show-toplevel")
	if out, err := git.Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	wd, _ := os.Getwd()
	return wd
}
