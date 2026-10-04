package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	henia "github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pattern"
	"github.com/srnnkls/henia/internal/similarity"
)

type queryOutput struct {
	text, json, count bool
	limit             int
}

func newQueryCommand() *cobra.Command {
	var flags runtimeFlags
	var output queryOutput
	var grammar, canonical bool
	var model string
	cmd := &cobra.Command{
		Use:     "query '<pattern>...'",
		Aliases: []string{"q"},
		Short:   "Match S-expression patterns against library skills and their resources",
		Long: `Match S-expression patterns against library skills and their resources, and
print one row per match with a line per capture. Skills are read as rendered for
the caller, like henia show; --canonical reads them as authored. henia query
--grammar prints the language. Problems print as text; the command always exits successfully so
a skill preload never aborts.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if grammar || len(args) == 0 {
				fmt.Fprint(out, henia.QueryGuide)
				return nil
			}
			q, err := pattern.Read(args[0])
			if err != nil {
				fmt.Fprint(out, explain(args[0], err))
				return nil
			}
			lib := flags.open(cmd)
			var rendering *library.Rendering
			if !canonical {
				r := newRenderer(detectHarness(flags.harness), lib)
				rendering = &library.Rendering{Body: r.body, Reference: r.reference}
			}
			corpus := lib.Corpus(rendering)
			for _, problem := range corpus.Problems {
				fmt.Fprintf(cmd.ErrOrStderr(), "henia: %s\n", problem)
			}
			env := pattern.Environment{Resolve: corpus}
			if q.Semantic {
				if model == "" {
					if cfg, err := config.LoadOptional(configPath); err == nil {
						model = cfg.Lint.Semantic.ModelPath
					}
				}
				if model != "" {
					encode, err := similarity.Model(model)
					if err != nil {
						fmt.Fprintf(out, "henia query: %v\n", err)
						return nil
					}
					env.Embed = encode
				}
			}
			rows, err := q.Run(corpus.Root, env)
			if err != nil {
				fmt.Fprintf(out, "henia query: %v\n", err)
				return nil
			}
			output.print(out, rows)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&grammar, "grammar", false, "Print the query language")
	cmd.Flags().BoolVar(&canonical, "canonical", false, "Read skills as authored instead of rendered for the caller")
	cmd.Flags().StringVar(&model, "model", "", "Local Model2Vec directory for (similar ...) (default: [lint.semantic] model_path)")
	cmd.Flags().BoolVar(&output.text, "text", false, "Print whole nodes")
	cmd.Flags().BoolVar(&output.json, "json", false, "Print rows as JSON objects keyed by capture")
	cmd.Flags().BoolVar(&output.count, "count", false, "Print the number of rows")
	cmd.Flags().IntVar(&output.limit, "limit", 0, "Print at most N rows")
	return cmd
}

func explain(query string, err error) string {
	var pe *pattern.Error
	if !errors.As(err, &pe) {
		return fmt.Sprintf("henia query: %v\n", err)
	}
	var b strings.Builder
	b.WriteString(pe.Explain(query))
	b.WriteString("examples:\n")
	examples := 0
	for line := range strings.SplitSeq(henia.QueryGuide, "\n") {
		if strings.HasPrefix(line, "henia query '") && examples < 2 {
			b.WriteString("  " + line + "\n")
			examples++
		}
	}
	b.WriteString("henia query --grammar prints the language\n")
	return b.String()
}

func (o queryOutput) print(out io.Writer, rows []pattern.Row) {
	total := len(rows)
	limited := o.limit > 0 && total > o.limit
	if o.count {
		fmt.Fprintln(out, total)
		return
	}
	if o.limit > 0 && len(rows) > o.limit {
		rows = rows[:o.limit]
	}
	if total == 0 {
		fmt.Fprintln(out, "henia query: no matches")
		return
	}
	if o.json {
		var records []map[string]any
		for _, row := range rows {
			record := map[string]any{}
			for _, cell := range row {
				name := cell.Name
				if name == "" {
					name = "match"
				}
				switch len(cell.Elements) {
				case 0:
					record[name] = nil
				case 1:
					record[name] = jsonNode(cell.Elements[0])
				default:
					var nodes []map[string]any
					for _, e := range cell.Elements {
						nodes = append(nodes, jsonNode(e))
					}
					record[name] = nodes
				}
			}
			records = append(records, record)
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(records)
		return
	}
	var b strings.Builder
	shown := 0
	for _, row := range rows {
		var block strings.Builder
		if shown > 0 && len(row) > 1 && !o.text {
			block.WriteString("\n")
		}
		for _, cell := range row {
			label := ""
			if cell.Name != "" {
				label = "@" + cell.Name + "  "
			}
			if len(cell.Elements) == 0 {
				fmt.Fprintf(&block, "%s-\n", label)
			}
			for _, e := range cell.Elements {
				fmt.Fprintf(&block, "%s%s\n", label, describe(e))
				if o.text && e.Text != "" {
					block.WriteString(e.Text + "\n\n")
				}
			}
		}
		if b.Len()+block.Len() > outputBudget {
			break
		}
		b.WriteString(block.String())
		shown++
	}
	fmt.Fprint(out, b.String())
	switch {
	case shown < len(rows):
		fmt.Fprintf(out, "henia query: %d of %d rows shown; narrow the query, or use --limit or --count\n", shown, total)
	case limited:
		fmt.Fprintf(out, "henia query: %d of %d rows shown\n", shown, total)
	}
}

func describe(e *markup.Element) string {
	parts := []string{address(e)}
	if e.Line > 0 && e.Type != "skill" && e.Type != "file" {
		lines := fmt.Sprintf("L%d", e.Line)
		if e.EndLine > e.Line {
			lines += fmt.Sprintf("-%d", e.EndLine)
		}
		parts = append(parts, lines)
	}
	parts = append(parts, e.Type)
	if snippet := firstLine(e); snippet != "" {
		parts = append(parts, snippet)
	}
	return strings.Join(parts, "  ")
}

func address(e *markup.Element) string {
	skill := e.Enclosing("skill")
	if skill == nil {
		return ""
	}
	addr := skill.Attrs["ref"]
	if file := e.Enclosing("file"); file != nil && file.Attrs["main"] != "true" {
		addr += "/" + file.Attrs["path"]
	}
	if section := e.Enclosing("section"); section != nil && section.Attrs["id"] != "" {
		addr += "#" + section.Attrs["id"]
	}
	return addr
}

func firstLine(e *markup.Element) string {
	text := e.Text
	if e.Type == "section" {
		text = e.Attrs["title"]
	}
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if utf8.RuneCountInString(line) > 80 {
				line = string([]rune(line)[:79]) + "…"
			}
			return line
		}
	}
	return ""
}

func jsonNode(e *markup.Element) map[string]any {
	node := map[string]any{"address": address(e), "type": e.Type, "attrs": e.Attrs}
	if e.Type != "skill" && e.Type != "file" {
		node["lines"] = []int{e.Line, e.EndLine}
		node["text"] = e.Text
	}
	return node
}
