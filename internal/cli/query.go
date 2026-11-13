package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	henia "github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
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
	var sort string
	var paths []string
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
			q, err := pattern.Read(args[0], sortKeys(sort)...)
			if err != nil {
				fmt.Fprint(out, explain(args[0], err))
				return nil
			}
			var corpus *library.Corpus
			if len(paths) > 0 {
				for _, name := range []string{"project", "package", "harness", "canonical"} {
					if cmd.Flags().Changed(name) {
						fmt.Fprintf(out, "henia query: --path replaces the library; drop --%s\n", name)
						return nil
					}
				}
				if corpus = pathCorpus(cmd, paths); corpus == nil {
					return nil
				}
			} else {
				lib := flags.open(cmd)
				var rendering *library.Rendering
				if !canonical {
					r := newRenderer(detectHarness(flags.harness), lib)
					rendering = &library.Rendering{Body: r.body, Reference: r.reference, Providers: provisions(flags.project, lib)}
				}
				corpus = lib.Corpus(rendering)
			}
			for _, problem := range corpus.Problems {
				fmt.Fprintf(cmd.ErrOrStderr(), "henia: %s\n", problem)
			}
			env := pattern.Environment{Resolve: corpus}
			if q.Semantic {
				dir, err := modelDir(model, configPath)
				if err != nil {
					fmt.Fprintf(out, "henia query: %v\n", err)
					return nil
				}
				if dir != "" {
					encode, err := similarity.Model(dir)
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
	cmd.Flags().StringVar(&model, "model", "", "Model2Vec directory or Hugging Face repo id for (similar ...) (default: [lint.semantic] model or model_path)")
	cmd.Flags().BoolVar(&output.text, "text", false, "Print whole nodes")
	cmd.Flags().BoolVar(&output.json, "json", false, "Print rows as JSON objects keyed by capture")
	cmd.Flags().BoolVar(&output.count, "count", false, "Print the number of rows")
	cmd.Flags().StringVar(&sort, "sort", "", "Order rows by ?var or @capture.key, descending with a leading -")
	cmd.Flags().IntVar(&output.limit, "limit", 0, "Print at most N rows")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "Markdown directory or file to query instead of the library (repeatable)")
	return cmd
}

// pathCorpus builds the corpus from the files under paths, printing problems
// to the command's output. It returns nil when the query must not run.
func pathCorpus(cmd *cobra.Command, paths []string) *library.Corpus {
	out := cmd.OutOrStdout()
	missing := false
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			fmt.Fprintf(out, "henia query: %s: %v\n", p, cmp.Or(errors.Unwrap(err), err))
			missing = true
		}
	}
	if missing {
		return nil
	}
	keepDir := func(root, path string, info fs.FileInfo) bool {
		return path == root || !strings.HasPrefix(info.Name(), ".") && !artifact.SkippedDir(info.Name())
	}
	keepFile := func(root, path string, info fs.FileInfo) bool {
		return path == root || !strings.HasPrefix(info.Name(), ".") && strings.EqualFold(filepath.Ext(path), ".md")
	}
	collected := map[string]string{}
	named := map[string]bool{}
	var docs []library.Document
	for _, root := range paths {
		found, err := artifact.Files(cmd.Context(), root, keepDir, keepFile)
		if err != nil {
			fmt.Fprintf(out, "henia query: %v\n", err)
			return nil
		}
		for _, path := range found {
			absolute, err := filepath.Abs(path)
			if err != nil {
				continue
			}
			if path == root {
				named[absolute] = true
			}
			if _, seen := collected[absolute]; seen {
				continue
			}
			doc := library.Document{Path: filepath.Clean(path)}
			collected[absolute] = doc.Path
			if filepath.Base(path) == "SKILL.md" {
				doc.Kind = "skill"
			}
			docs = append(docs, doc)
		}
	}
	skillIDs(docs)
	corpus := library.Documents(docs)
	for _, o := range corpus.Omitted {
		absolute, err := filepath.Abs(o.Path)
		if err != nil || !named[absolute] {
			continue
		}
		fmt.Fprintf(out, "henia query: %s: omitted: %v\n", collected[absolute], cmp.Or(errors.Unwrap(o.Cause), o.Cause))
	}
	return corpus
}

// skillIDs names each skill after its absolute directory, qualifying skills
// whose names collide by their collected directory path.
func skillIDs(docs []library.Document) {
	names := map[string]int{}
	name := func(d library.Document) string {
		dir, _ := filepath.Abs(filepath.Dir(d.Path))
		return filepath.Base(dir)
	}
	for _, d := range docs {
		if d.Kind == "skill" {
			names[name(d)]++
		}
	}
	for i, d := range docs {
		if d.Kind != "skill" {
			continue
		}
		switch dir := filepath.Dir(d.Path); {
		case names[name(d)] > 1:
			docs[i].ID = filepath.ToSlash(dir)
		case name(d) != filepath.Base(dir):
			docs[i].Name = name(d)
		}
	}
}

// modelDir resolves --model, a directory or a Hugging Face repo id, falling back
// to the configured semantic model.
func modelDir(model, configPath string) (string, error) {
	if model == "" {
		cfg, err := config.LoadOptional(configPath)
		if err != nil {
			return "", nil
		}
		return cfg.Lint.Semantic.Dir()
	}
	if info, err := os.Stat(model); (err == nil && info.IsDir()) || !similarity.IsRepoID(model) {
		return model, nil
	}
	return similarity.Snapshot(model, "")
}

func sortKeys(key string) []string {
	if key == "" {
		return nil
	}
	return []string{key}
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
			for _, cell := range row.Cells {
				name := cell.Name
				if name == "" {
					name = "match"
				}
				if cell.Value {
					record[name] = jsonValues(cell)
					continue
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
			for name, value := range row.Vars {
				if !strings.HasPrefix(name, "@") {
					record["?"+name] = value
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
		var vars []string
		for _, name := range row.Shown {
			if value, ok := row.Vars[name]; ok {
				vars = append(vars, "?"+name+"="+value)
			}
		}
		if shown > 0 && (len(row.Cells) > 1 || len(vars) > 0) && !o.text {
			block.WriteString("\n")
		}
		if len(vars) > 0 {
			block.WriteString(strings.Join(vars, "  ") + "\n")
		}
		for _, cell := range row.Cells {
			label := ""
			if cell.Name != "" {
				label = "@" + cell.Name + "  "
			}
			if cell.Value {
				fmt.Fprintf(&block, "%s%s\n", label, cmp.Or(strings.Join(cell.Values, ", "), "-"))
				continue
			}
			if len(cell.Elements) == 0 {
				fmt.Fprintf(&block, "%s-\n", label)
				continue
			}
			if cell.Collected && !o.text {
				one, many := "node", "nodes"
				if cell.Name == "" {
					one, many = "match", "matches"
				}
				if len(cell.Elements) != 1 {
					one = many
				}
				fmt.Fprintf(&block, "%s%d %s\n", label, len(cell.Elements), one)
				continue
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

func jsonValues(cell pattern.Cell) any {
	value := func(v string) any {
		if n, err := strconv.ParseFloat(v, 64); err == nil && cell.Number && !math.IsNaN(n) && !math.IsInf(n, 0) {
			return n
		}
		return v
	}
	switch {
	case len(cell.Values) == 0:
		return nil
	case len(cell.Values) == 1 && !cell.Collected:
		return value(cell.Values[0])
	}
	values := make([]any, len(cell.Values))
	for i, v := range cell.Values {
		values[i] = value(v)
	}
	return values
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
	var addr string
	file := e.Enclosing("file")
	switch skill := e.Enclosing("skill"); {
	case skill != nil:
		addr = skill.Attrs["ref"]
		if file != nil && file.Attrs["main"] != "true" {
			addr += "/" + file.Attrs["path"]
		}
	case file != nil:
		addr = file.Attrs["path"]
	default:
		return ""
	}
	if section := e.Enclosing("section"); section != nil && section.Attrs["id"] != "" {
		addr += "#" + section.Attrs["id"]
	}
	return addr
}

func firstLine(e *markup.Element) string {
	text := e.Text
	switch {
	case e.Type == "section":
		text = e.Attrs["title"]
	case e.Type == "link" && e.Attrs["ref"] == "slot":
		text = e.Attrs["slot"] + ": " + e.Attrs["dest"]
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
