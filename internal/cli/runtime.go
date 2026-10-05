package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	henia "github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/caller"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/preload"
	"github.com/srnnkls/henia/internal/reference"
)

const outputBudget = 28000

type runtimeFlags struct {
	project  string
	packages []string
	harness  string
}

func (f *runtimeFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.project, "project", "", "Project root whose .henia library is read")
	cmd.Flags().StringArrayVar(&f.packages, "package", nil, "Additional skill package root, read before the installed packages (repeatable)")
	cmd.Flags().StringVar(&f.harness, "harness", "", "Harness to render for (default: HENIA_HARNESS, then the calling agent)")
}

func (f *runtimeFlags) open(cmd *cobra.Command) *library.Library {
	if f.project == "" {
		f.project = projectRoot(cmd)
	}
	return library.Open(f.project, f.packages)
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

type showMode struct {
	head, toc bool
	digest    string
}

func newShowCommand() *cobra.Command {
	var flags runtimeFlags
	var mode showMode
	cmd := &cobra.Command{
		Use:   "show <skill>[/<resource>][#section]",
		Short: "Print a library skill or one of its sections, rendered for the caller",
		Long: `Print a library skill or one of its sections, rendered for the caller.

A skill is <name> or <package>:<name>. Output longer than the budget prints the
sections instead, each addressable as <skill>#<section>. A full skill ends with
the list of its resources; <skill>/<path> reads one and <skill>/<dir>/ lists a
directory. --head prints what a hybrid skill carries upfront: its :::static
blocks, its contents and the contents of the skills it references; --toc
prints only its contents. --digest names the revision a harness head was
built from and reports a library that holds another. Problems print as text;
the command always exits successfully so a skill preload never aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			harness := detectHarness(flags.harness)
			settings, err := loadRuntime(flags.project)
			expand := func(e library.Entry, text string) string {
				if err != nil {
					return fmt.Sprintf("henia: preloads not run: %v\n\n%s", err, text)
				}
				return settings.runner.Expand(cmd.Context(), text, preload.Context{Dir: flags.project, Skill: e.Name, Package: e.Package, Tier: e.Tier, Caller: harness})
			}
			disclose := func(e library.Entry) bool { return discloses(settings.disclosure, e) }
			show(cmd.OutOrStdout(), lib, newRenderer(harness, lib), args[0], mode, expand, disclose)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&mode.head, "head", false, "Print the skill's :::static blocks, contents and related contents")
	cmd.Flags().BoolVar(&mode.toc, "toc", false, "Print the skill's contents")
	cmd.Flags().StringVar(&mode.digest, "digest", "", "Revision a harness head was built from")
	return cmd
}

func show(out io.Writer, lib *library.Library, r *renderer, target string, mode showMode, expand func(library.Entry, string) string, disclose func(library.Entry) bool) {
	ref, anchor, sectioned := strings.Cut(target, "#")
	ref, resource, isResource := strings.Cut(ref, "/")
	entry, err := lib.Resolve(ref)
	if err != nil {
		fmt.Fprintf(out, "henia: %v\n", err)
		return
	}
	name := lib.Reference(entry)
	if isResource {
		showResource(out, name, filepath.Dir(entry.Path), resource, anchor, sectioned)
		return
	}
	if mode.digest != "" {
		if canonical, err := os.ReadFile(entry.Path); err == nil && build.Digest(canonical) != mode.digest {
			fmt.Fprintf(out, "henia: this harness copy of %s was built from revision %s, but the library holds %s; run henia install to update it\n\n", name, mode.digest, build.Digest(canonical))
		}
	}
	body := r.body(entry)
	if mode.head || mode.toc {
		if mode.head {
			if static := strings.TrimSpace(r.render(entry, true)); static != "" {
				fmt.Fprint(out, expand(entry, static+"\n\n"))
			}
		}
		fmt.Fprintf(out, "Read the sections of %s as the task needs them:\n\n", name)
		contents(out, name, body)
		if disclose(entry) {
			resourceList(out, name, filepath.Dir(entry.Path), "")
		}
		if mode.head {
			related(out, lib, r, entry)
		}
		return
	}
	if sectioned {
		if _, text, ok := library.Section(body, anchor); ok {
			if len(text) <= outputBudget {
				fmt.Fprint(out, expand(entry, text))
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
		fmt.Fprint(out, expand(entry, body))
		if disclose(entry) {
			resourceList(out, name, filepath.Dir(entry.Path), "")
		}
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

func newPreloadCommand() *cobra.Command {
	var flags runtimeFlags
	var skill string
	cmd := &cobra.Command{
		Use:   "preload --skill <skill> -- <command>",
		Short: "Run one skill preload under Henia's sandbox and refusal rules",
		Long: `Run one skill preload under Henia's sandbox and refusal rules and print the
command above its output. The command must be one of the named library skill's
own preloads, as rendered for any harness its package configures; projected
skills call this for each preload. Problems print as text; the command always
exits successfully so a skill preload never aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			out := cmd.OutOrStdout()
			entry, err := lib.Resolve(skill)
			if err != nil {
				fmt.Fprint(out, preload.Show(args[0], fmt.Sprintf("henia: blocked by henia/not-a-preload: %v", err)))
				return nil
			}
			if !declaresPreload(lib, entry, args[0]) {
				fmt.Fprint(out, preload.Show(args[0], fmt.Sprintf("henia: blocked by henia/not-a-preload: %s declares no such preload", lib.Reference(entry))))
				return nil
			}
			c := preload.Context{Dir: flags.project, Skill: entry.Name, Package: entry.Package, Tier: entry.Tier, Caller: detectHarness(flags.harness)}
			runner, err := preloadRunner(flags.project)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "henia: preload not run: %v\n", err)
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), runner.Block(cmd.Context(), args[0], c))
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&skill, "skill", "", "Skill the preload belongs to")
	_ = cmd.MarkFlagRequired("skill")
	return cmd
}

func declaresPreload(lib *library.Library, entry library.Entry, command string) bool {
	if build.HeadCommand(entry.Name, command) {
		return true
	}
	harnesses := []string{""}
	if data, err := os.ReadFile(entry.Origin.Config); err == nil {
		if configured, err := config.Harnesses(data); err == nil {
			harnesses = append(harnesses, slices.Sorted(maps.Keys(configured))...)
		}
	}
	for _, harness := range harnesses {
		for _, p := range preload.Find([]byte(newRenderer(harness, lib).body(entry))) {
			if preload.Declares(p.Command, command) {
				return true
			}
		}
	}
	return false
}

type runtimeSettings struct {
	runner     *preload.Runner
	disclosure bool
}

func loadRuntime(project string) (runtimeSettings, error) {
	settings := runtimeSettings{disclosure: true}
	var layers [2]preload.Settings
	paths := []string{filepath.Join(library.ConfigDir(), "henia.toml"), ""}
	if path, err := config.Find(project); err == nil {
		paths[1] = path
	} else if !errors.Is(err, os.ErrNotExist) {
		return settings, err
	}
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if path == "" || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return settings, err
		}
		var file struct {
			Preload   preload.Settings       `toml:"preload"`
			Resources config.ResourceOptions `toml:"resources"`
		}
		if err := toml.Unmarshal(data, &file); err != nil {
			return settings, fmt.Errorf("%s: %w", path, err)
		}
		layers[i] = file.Preload
		if file.Resources.Disclosure != nil {
			settings.disclosure = *file.Resources.Disclosure
		}
	}
	runner, err := preload.NewRunner(layers[0], layers[1])
	settings.runner = runner
	return settings, err
}

func preloadRunner(project string) (*preload.Runner, error) {
	settings, err := loadRuntime(project)
	return settings.runner, err
}

func discloses(global bool, e library.Entry) bool {
	if settings, ok := e.Artifact.Frontmatter["henia"].(map[string]any); ok {
		if resources, ok := settings["resources"].(map[string]any); ok {
			if disclosure, ok := resources["disclosure"].(bool); ok {
				return disclosure
			}
		}
	}
	return global
}

func resourceList(out io.Writer, name, skillDir, sub string) {
	resources := library.Resources(skillDir, sub)
	if len(resources) == 0 {
		return
	}
	fmt.Fprintf(out, "\n## Resources\n\nRead one with `henia show %s/<path>`:\n\n", name)
	for _, r := range resources {
		line := "- `" + r.Path + "`"
		if r.Files > 0 {
			line += fmt.Sprintf(" (%d files)", r.Files)
		}
		if r.Description != "" {
			line += ": " + r.Description
		}
		fmt.Fprintln(out, line)
	}
}

func showResource(out io.Writer, name, skillDir, path, anchor string, sectioned bool) {
	full, err := library.ResourcePath(skillDir, path)
	if err != nil {
		fmt.Fprintf(out, "henia: %s/%s: %v\n", name, path, err)
		return
	}
	if info, err := os.Stat(full); err == nil && info.IsDir() {
		rel, _ := filepath.Rel(skillDir, full)
		if rel == "." {
			rel = ""
		}
		resourceList(out, name, skillDir, rel)
		return
	}
	text, err := library.ReadResource(full)
	if err != nil {
		fmt.Fprintf(out, "henia: %s/%s: %v\n", name, path, err)
		return
	}
	address := name + "/" + path
	if sectioned {
		_, section, ok := library.Section(text, anchor)
		if !ok {
			fmt.Fprintf(out, "henia: %s has no section %q\n", address, anchor)
			contents(out, address, text)
			return
		}
		text = section
	}
	if len(text) > outputBudget {
		fmt.Fprintf(out, "%s exceeds the output budget; read its sections with henia show %s#<section>:\n", address, address)
		contents(out, address, text)
		return
	}
	fmt.Fprint(out, text)
}

func newContextCommand() *cobra.Command {
	var flags runtimeFlags
	cmd := &cobra.Command{
		Use:   "context <skill>",
		Short: "Print a skill's runtime context",
		Long: `Print a skill's runtime context: the sections of the library skills it
references, each readable with henia show.
Problems print as text; the command always exits successfully so a skill
preload never aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			out := cmd.OutOrStdout()
			entry, err := lib.Resolve(args[0])
			if err != nil {
				fmt.Fprintf(out, "henia: %v\n", err)
				return nil
			}
			var b strings.Builder
			related(&b, lib, newRenderer(detectHarness(flags.harness), lib), entry)
			if b.Len() == 0 {
				fmt.Fprintf(&b, "%s references no other library skills\n", lib.Reference(entry))
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
	return cmd
}

func related(out io.Writer, lib *library.Library, r *renderer, entry library.Entry) {
	var seen []string
	for _, ref := range libraryReferences(entry.Artifact.Body) {
		referenced, err := lib.Resolve(ref)
		if err != nil || referenced.ID == entry.ID || slices.Contains(seen, referenced.ID) {
			continue
		}
		seen = append(seen, referenced.ID)
		name := lib.Reference(referenced)
		fmt.Fprintf(out, "\nSkill %s: %s\n", name, referenced.Description)
		contents(out, name, r.body(referenced))
	}
}

func libraryReferences(body string) []string {
	var refs []string
	for _, ref := range reference.Skills(body) {
		refs = append(refs, ref.Name)
	}
	return refs
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

func (r *renderer) body(e library.Entry) string { return r.render(e, false) }

func (r *renderer) render(e library.Entry, head bool) string {
	canonical, err := os.ReadFile(e.Path)
	if err != nil {
		return e.Artifact.Body
	}
	var harness henia.Harness
	name := ""
	packageConfig := r.config(e.Origin.Config)
	if r.harness != "" && packageConfig != nil {
		if harnesses, err := config.Harnesses(packageConfig); err == nil {
			if h, ok := harnesses[r.harness]; ok {
				harness, name = h, r.harness
				harness.ProjectRoot, harness.UserRoot = e.Origin.ProjectRoot(), library.ConfigDir()
			}
		}
	}
	if name == "" {
		packageConfig = nil
	}
	served := r.lib.Names()
	if name != "" {
		for _, other := range r.lib.Entries {
			if other.Package == e.Package && build.Projects(harness, other.Name) && build.Invocable(other.Artifact.Frontmatter) {
				delete(served, other.Name)
			}
		}
	}
	h := sha256.New()
	for _, part := range [][]byte{[]byte(rootCmd.Version + executableStamp()), []byte(name), []byte(strconv.FormatBool(head)), packageConfig, canonical, []byte(strings.Join(slices.Sorted(maps.Keys(served)), " "))} {
		h.Write(part)
		h.Write([]byte{0})
	}
	key := hex.EncodeToString(h.Sum(nil))
	cache := filepath.Join(library.CacheDir(), "render", key[:2], key)
	if cached, err := os.ReadFile(cache); err == nil {
		return string(cached)
	}
	rendered, err := build.Render(e.Artifact, name, harness, served, head)
	if err != nil {
		return fmt.Sprintf("henia: rendering %s for %q failed (%v); canonical text follows\n\n%s", e.ID, name, err, e.Artifact.Body)
	}
	if os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
		_ = os.WriteFile(cache, []byte(rendered.Body), 0o644)
	}
	return rendered.Body
}

func (r *renderer) reference(e library.Entry) *regexp.Regexp {
	if r.harness == "" {
		return nil
	}
	harnesses, err := config.Harnesses(r.config(e.Origin.Config))
	if err != nil {
		return nil
	}
	template := harnesses[r.harness].References["skill"]
	prefix, suffix, ok := strings.Cut(template, "{{.Name}}")
	if !ok || strings.Contains(prefix+suffix, "{{") {
		return nil
	}
	pattern := regexp.QuoteMeta(prefix) + `([a-z0-9][a-z0-9._-]*)` + regexp.QuoteMeta(suffix)
	if !strings.ContainsAny(template, "*[]") {
		pattern = "`" + pattern + "`"
	}
	return regexp.MustCompile(pattern)
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
