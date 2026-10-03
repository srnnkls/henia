package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	henia "github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/caller"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/reference"
	"github.com/srnnkls/henia/internal/slots"
)

const outputBudget = 28000

type runtimeFlags struct {
	project string
	sources []string
	harness string
}

func (f *runtimeFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.project, "project", "", "Project root whose .henia library is read")
	cmd.Flags().StringArrayVar(&f.sources, "source", nil, "Additional source root, read before the installed sources (repeatable)")
	cmd.Flags().StringVar(&f.harness, "harness", "", "Harness to render for (default: HENIA_HARNESS, then the calling agent)")
}

func (f *runtimeFlags) open(cmd *cobra.Command) *library.Library {
	if f.project == "" {
		f.project = projectRoot(cmd)
	}
	return library.Open(f.project, f.sources)
}

func newLsCommand() *cobra.Command {
	var flags runtimeFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List the skills in the library",
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			if asJSON {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(lib.Entries)
			}
			for _, e := range lib.Entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", lib.Reference(e), e.Tier, e.Description)
			}
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the catalog as JSON")
	return cmd
}

func newShowCommand() *cobra.Command {
	var flags runtimeFlags
	cmd := &cobra.Command{
		Use:   "show <skill>[#section]",
		Short: "Print a library skill or one of its sections, rendered for the caller",
		Long: `Print a library skill or one of its sections, rendered for the caller.

A skill is <name> or <source>:<name>. Output longer than the budget prints the
sections instead, each addressable as <skill>#<section>. Problems print as
text; the command always exits successfully so a skill preload never aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			show(cmd.OutOrStdout(), lib, newRenderer(detectHarness(flags.harness), lib), args[0])
			return nil
		},
	}
	flags.register(cmd)
	return cmd
}

func show(out io.Writer, lib *library.Library, r *renderer, target string) {
	ref, anchor, sectioned := strings.Cut(target, "#")
	entry, err := lib.Resolve(ref)
	if err != nil {
		fmt.Fprintf(out, "henia: %v\n", err)
		return
	}
	name := lib.Reference(entry)
	body := r.body(entry)
	if sectioned {
		if _, text, ok := library.Section(body, anchor); ok {
			if len(text) <= outputBudget {
				fmt.Fprint(out, text)
				return
			}
			fmt.Fprintf(out, "%s#%s exceeds the output budget; read its subsections with henia show:\n", name, anchor)
			contents(out, name, body)
			return
		}
		fmt.Fprintf(out, "henia: %s has no section %q\n", name, anchor)
		contents(out, name, body)
		return
	}
	if len(body) <= outputBudget {
		fmt.Fprint(out, body)
		return
	}
	fmt.Fprintf(out, "%s exceeds the output budget; read its sections with henia show %s#<section>:\n", name, name)
	contents(out, name, body)
}

func contents(out io.Writer, name, body string) {
	sections, err := markup.Sections([]byte(body))
	if err != nil {
		fmt.Fprintf(out, "henia: %v\n", err)
		return
	}
	for _, section := range sections {
		fmt.Fprintf(out, "%s%s#%s  %s\n", strings.Repeat("  ", max(section.Level-1, 0)), name, section.Anchor, section.Title)
	}
}

func newContextCommand() *cobra.Command {
	var flags runtimeFlags
	var globals []string
	cmd := &cobra.Command{
		Use:   "context <skill>",
		Short: "Print a skill's dynamic context",
		Long: `Print a skill's dynamic context: the providers of the slots it applies and the
sections of the library skills it references, each readable with henia show.
Problems print as text; the command always exits successfully so a skill
preload never aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			out := cmd.OutOrStdout()
			skills := slots.Discover(flags.project, globals, lib.Sources)
			body, found := skillBody(args[0], flags.project, slices.Concat(globals, sourceSkillDirs(lib.Sources)))
			if !found {
				fmt.Fprintf(out, "henia: no skill named %q\n", args[0])
				return nil
			}
			var b strings.Builder
			if applied, _ := slots.Applied(skills, args[0]); len(applied) > 0 {
				b.WriteString("Slot providers:\n")
				for _, row := range slots.Evaluate(skills).Rows(applied, false) {
					line := row.Slot + "\t" + row.Kind + "\t" + row.Path
					if row.Value != "" {
						line += "\t" + row.Value
					}
					b.WriteString(line + "\n")
				}
			}
			r := newRenderer(detectHarness(flags.harness), lib)
			var seen []string
			for _, ref := range libraryReferences(body) {
				entry, err := lib.Resolve(ref)
				if err != nil || entry.Name == args[0] || slices.Contains(seen, entry.ID) {
					continue
				}
				seen = append(seen, entry.ID)
				name := lib.Reference(entry)
				fmt.Fprintf(&b, "\nSkill %s: %s\n", name, entry.Description)
				contents(&b, name, r.body(entry))
			}
			text := b.String()
			if len(text) > outputBudget {
				text = text[:strings.LastIndexByte(text[:outputBudget], '\n')+1] + "henia: context truncated at the budget\n"
			}
			fmt.Fprint(out, text)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().StringArrayVar(&globals, "global", nil, "Harness skills directory searched for the skill and its global providers (repeatable)")
	return cmd
}

func sourceSkillDirs(sources []library.Source) []string {
	var dirs []string
	for _, source := range sources {
		dirs = append(dirs, source.Skills())
	}
	return dirs
}

var shown = regexp.MustCompile("`henia show ([a-z0-9][a-z0-9._:-]*)")

func libraryReferences(body string) []string {
	var refs []string
	for _, ref := range reference.Parse(body) {
		if ref.Type == reference.TypeSkill {
			refs = append(refs, ref.Name)
		}
	}
	for _, match := range shown.FindAllStringSubmatch(body, -1) {
		refs = append(refs, match[1])
	}
	return refs
}

func skillBody(name, project string, dirs []string) (string, bool) {
	var search []string
	for _, dir := range slots.ProjectDirs {
		search = append(search, filepath.Join(project, dir))
	}
	for _, dir := range append(search, dirs...) {
		for _, f := range library.Scan(dir) {
			if filepath.Base(filepath.Dir(f.Path)) == name {
				return f.Artifact.Body, true
			}
		}
	}
	return "", false
}

func detectHarness(flag string) string {
	return caller.Harness(flag, os.Getenv, caller.Ancestors())
}

type renderer struct {
	harness string
	lib     *library.Library
	configs map[string][]byte
}

func newRenderer(harness string, lib *library.Library) *renderer {
	return &renderer{harness: harness, lib: lib, configs: make(map[string][]byte)}
}

func (r *renderer) config(path string) []byte {
	if path == "" {
		return nil
	}
	if data, ok := r.configs[path]; ok {
		return data
	}
	data, _ := os.ReadFile(path)
	r.configs[path] = data
	return data
}

func (r *renderer) body(e library.Entry) string {
	canonical, err := os.ReadFile(e.Path)
	if err != nil {
		return e.Artifact.Body
	}
	var harness henia.Harness
	name := ""
	config := r.config(e.Origin.Config)
	if r.harness != "" && config != nil {
		var parsed struct {
			Harness map[string]henia.Harness `toml:"harness"`
		}
		if toml.Unmarshal(config, &parsed) == nil {
			if h, ok := parsed.Harness[r.harness]; ok {
				harness, name = h, r.harness
				harness.ProjectRoot, harness.UserRoot = e.Origin.ProjectRoot(), library.ConfigDir()
			}
		}
	}
	if name == "" {
		config = nil
	}
	served := r.lib.Names()
	if name != "" {
		for _, other := range r.lib.Entries {
			if other.Source == e.Source && (len(harness.Include) == 0 || slices.Contains(harness.Include, other.Name)) {
				delete(served, other.Name)
			}
		}
	}
	h := sha256.New()
	for _, part := range [][]byte{[]byte(rootCmd.Version + executableStamp()), []byte(name), config, canonical, []byte(strings.Join(slices.Sorted(maps.Keys(served)), " "))} {
		h.Write(part)
		h.Write([]byte{0})
	}
	key := hex.EncodeToString(h.Sum(nil))
	cache := filepath.Join(library.CacheDir(), "render", key[:2], key)
	if cached, err := os.ReadFile(cache); err == nil {
		return string(cached)
	}
	rendered, err := build.Render(e.Artifact, name, harness, served)
	if err != nil {
		return fmt.Sprintf("henia: rendering %s for %q failed (%v); canonical text follows\n\n%s", e.ID, name, err, e.Artifact.Body)
	}
	if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
		_ = os.WriteFile(cache, []byte(rendered.Body), 0o644)
	}
	return rendered.Body
}

func executableStamp() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(" %d %d", info.Size(), info.ModTime().UnixNano())
}
