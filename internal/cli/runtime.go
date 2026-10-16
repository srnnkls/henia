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
	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/caller"
	"github.com/srnnkls/henia/internal/components"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
	"github.com/srnnkls/henia/internal/markup"
	"github.com/srnnkls/henia/internal/pipeline"
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
				lib.Index()
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
}

func newShowCommand() *cobra.Command {
	var flags runtimeFlags
	var mode showMode
	cmd := &cobra.Command{
		Use:   "show <skill>[.<module>…][#section]",
		Short: "Print a library skill or one of its sections, rendered for the caller",
		Long: `Print a library skill or one of its sections, rendered for the caller.

A skill is <name> or <package>:<name>. Output longer than the budget prints the
sections instead, each addressable as <skill>#<section>. A full skill ends with
the list of its resources, addressed like modules: <skill>.<dir>.<file> reads
<dir>/<file>.md and <skill>.<dir> lists <dir>/. Other files are read by path,
<skill>/<path>. --head prints what a hybrid skill carries upfront: its :::head
blocks, its contents and the contents of the skills it references; --toc
prints only its contents, and both report on stderr a harness copy that henia
install wrote from an older revision of the skill. Problems print as text;
the command always exits successfully so a skill preload never aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := flags.open(cmd)
			harness := detectHarness(flags.harness)
			settings, err := loadRuntime(flags.project)
			r := newRenderer(harness, lib)
			resolve := newResolver(lib, r, flags.project).resolve
			expand := func(e library.Entry, text string) string {
				if err != nil {
					return fmt.Sprintf("henia: preloads not run: %v\n\n%s", err, preload.Substitute(text, resolve))
				}
				return settings.runner.Expand(cmd.Context(), text, preload.Context{Dir: flags.project, SkillDir: filepath.Dir(e.Path), Skill: e.Name, Package: e.Package, Tier: e.Tier, Caller: harness, Resolve: resolve})
			}
			disclose := func(e library.Entry) bool { return discloses(settings.disclosure, e) }
			notice := func(e library.Entry) {
				if stale(r.harness, installManifest(flags.project, e), e) {
					fmt.Fprintf(cmd.ErrOrStderr(), "henia: the %s copy of %s was installed from an older revision of the skill; run henia install to update it\n", r.harness, lib.Reference(e))
				}
			}
			show(cmd.OutOrStdout(), lib, r, args[0], mode, expand, disclose, notice)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&mode.head, "head", false, "Print the skill's :::head blocks, contents and related contents")
	cmd.Flags().BoolVar(&mode.toc, "toc", false, "Print the skill's contents")
	return cmd
}

func show(out io.Writer, lib *library.Library, r *renderer, target string, mode showMode, expand func(library.Entry, string) string, disclose func(library.Entry) bool, notice func(library.Entry)) {
	ref, anchor, sectioned := strings.Cut(target, "#")
	ref, modules, resource, isResource, err := library.SplitAddress(ref)
	if err != nil {
		fmt.Fprintf(out, "henia: %v\n", err)
		return
	}
	entry, err := lib.Resolve(ref)
	if err != nil {
		fmt.Fprintf(out, "henia: %v\n", err)
		return
	}
	if len(modules) > 0 {
		if resource, err = library.ModulePath(filepath.Dir(entry.Path), modules); err != nil {
			fmt.Fprintf(out, "henia: %s.%s: %v\n", lib.Reference(entry), strings.Join(modules, "."), err)
			return
		}
		isResource = true
	}
	name := lib.Reference(entry)
	if isResource {
		showResource(out, name, filepath.Dir(entry.Path), resource, anchor, sectioned, mode.toc)
		return
	}
	body, stopped := r.rendered(entry, false)
	static := ""
	if stopped == "" && mode.head {
		static, stopped = r.rendered(entry, true)
	}
	if stopped != "" {
		fmt.Fprint(out, stopped)
		return
	}
	if mode.head || mode.toc {
		notice(entry)
		placedContents, placedRelated := false, false
		if mode.head {
			static = strings.TrimSpace(static)
			placedContents, placedRelated = places(static, preload.ShowCommand, entry.Name, name), places(static, preload.ContextCommand, entry.Name, name)
			if static != "" {
				fmt.Fprint(out, expand(entry, static+"\n\n"))
			}
		}
		if !placedContents {
			fmt.Fprintf(out, "Read the sections of %s as the task needs them:\n\n", name)
			contents(out, name, body)
		}
		if disclose(entry) {
			resourceList(out, name, filepath.Dir(entry.Path), "")
		}
		if mode.head && !placedRelated {
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
			fmt.Fprintf(out, "%s#%s exceeds the output budget; read its subsections:\n\n", name, anchor)
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
	fmt.Fprintf(out, "%s exceeds the output budget; read its sections:\n\n", name)
	contents(out, name, body)
}

func contents(out io.Writer, name, body string) {
	sections, err := markup.Sections([]byte(body))
	if err != nil {
		fmt.Fprintf(out, "henia: %v\n", err)
		return
	}
	var open []int
	for _, section := range sections {
		for len(open) > 0 && open[len(open)-1] >= section.Level {
			open = open[:len(open)-1]
		}
		fmt.Fprintf(out, "%s- %s: `%s`\n", strings.Repeat("  ", len(open)), section.Title, library.ShowCommand(name+"#"+section.Anchor))
		open = append(open, section.Level)
	}
}

func places(static string, command *regexp.Regexp, names ...string) bool {
	for _, p := range preload.Find([]byte(static)) {
		if m := command.FindStringSubmatch(p.Command); m != nil && slices.Contains(names, m[1]) {
			return true
		}
	}
	return false
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
			harness := detectHarness(flags.harness)
			c := preload.Context{Dir: flags.project, SkillDir: filepath.Dir(entry.Path), Skill: entry.Name, Package: entry.Package, Tier: entry.Tier, Caller: harness, Resolve: newResolver(lib, newRenderer(harness, lib), flags.project).resolve}
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
	fmt.Fprintf(out, "\n## Resources\n\nRead one with `henia show <address>`:\n\n")
	for _, r := range resources {
		line := "- `" + library.Address(name, r.Path) + "`"
		if r.Files > 0 {
			line += fmt.Sprintf(" (%d files)", r.Files)
		}
		if r.Description != "" {
			line += ": " + r.Description
		}
		fmt.Fprintln(out, line)
	}
}

func showResource(out io.Writer, name, skillDir, path, anchor string, sectioned, toc bool) {
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
	address := library.Address(name, path)
	if toc {
		fmt.Fprintf(out, "Read the sections of %s as the task needs them:\n\n", address)
		contents(out, address, text)
		return
	}
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
		fmt.Fprintf(out, "%s exceeds the output budget; read its sections:\n\n", address)
		contents(out, address, text)
		return
	}
	fmt.Fprint(out, text)
}

// installManifest is the install record of the scope that installs entry: the
// project's for its own and its dependencies' skills, the global one otherwise.
func installManifest(project string, entry library.Entry) func(harness string) string {
	if entry.Tier == library.Global {
		return library.InstallManifest
	}
	return projectManifest(project)
}

func stale(harness string, manifest func(harness string) string, entry library.Entry) bool {
	if harness == "" {
		return false
	}
	data, err := os.ReadFile(manifest(harness))
	if err != nil {
		return false
	}
	var record installRecord
	if json.Unmarshal(data, &record) != nil {
		return false
	}
	installed, ok := record.Revisions[entry.Name]
	if !ok {
		return false
	}
	canonical, err := os.ReadFile(entry.Path)
	return err == nil && build.Digest(canonical) != installed
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
			text := strings.TrimLeft(b.String(), "\n")
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
	knowns  map[string]map[string]bool
}

func newRenderer(harness string, lib *library.Library) *renderer {
	return &renderer{harness: harness, lib: lib, configs: make(map[string][]byte), knowns: make(map[string]map[string]bool)}
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

func (r *renderer) harnessFor(e library.Entry) (string, henia.Harness, []byte) {
	var harness henia.Harness
	name := ""
	packageConfig := r.config(e.Origin.Config)
	if r.harness != "" && packageConfig != nil {
		if harnesses, err := config.Harnesses(packageConfig); err == nil {
			if h, ok := harnesses[r.harness]; ok {
				harness, name = h, r.harness
			}
		}
	}
	harness.ProjectRoot, harness.UserRoot = e.Origin.ProjectRoot(), library.ConfigDir()
	if name == "" {
		packageConfig = nil
	}
	return name, harness, packageConfig
}

func (r *renderer) render(e library.Entry, head bool) string {
	body, stopped := r.rendered(e, head)
	return body + stopped
}

type resolution struct {
	Package, Name, ID string
	Resource          string `json:",omitempty"`
}

type cached struct {
	Refs   []resolution
	Digest string
	Body   string
}

func (r *renderer) rendered(e library.Entry, head bool) (string, string) {
	if _, err := os.Stat(e.Path); err != nil {
		return e.Artifact.Body, ""
	}
	name, harness, packageConfig := r.harnessFor(e)
	served := r.lib.Names()
	if name != "" {
		for _, other := range r.lib.Entries {
			if other.Package == e.Package && build.Projects(harness, other.Name) && build.Invocable(other.Artifact.Frontmatter) {
				delete(served, other.Name)
			}
		}
	}
	key := digest([]byte(rootCmd.Version+executableStamp()), []byte(r.harness), []byte(name), []byte(strconv.FormatBool(head)), packageConfig, []byte(strings.Join(slices.Sorted(maps.Keys(served)), " ")), []byte(e.ID), []byte(e.Path))
	inputs := func(refs []resolution) (string, bool) {
		closure := []library.Entry{e}
		var resources [][]byte
		for _, ref := range refs {
			target, err := r.locate(ref.Package, ref.Name)
			if err != nil || target.ID != ref.ID {
				return "", false
			}
			if !slices.ContainsFunc(closure, func(c library.Entry) bool { return c.ID == target.ID }) {
				closure = append(closure, target)
			}
			if ref.Resource != "" {
				path, err := library.AddressedResource(filepath.Dir(target.Path), ref.Resource)
				if err != nil {
					return "", false
				}
				data, _ := os.ReadFile(path)
				resources = append(resources, []byte(path), data)
			}
		}
		return digest(append(r.dependencies(closure), resources...)...), true
	}
	cache := filepath.Join(library.CacheDir(), "render", key[:2], key)
	if data, err := os.ReadFile(cache); err == nil {
		var entry cached
		if json.Unmarshal(data, &entry) == nil {
			if current, ok := inputs(entry.Refs); ok && current == entry.Digest {
				return entry.Body, ""
			}
		}
	}
	rendered, refs, err := r.expand(e, served, head)
	if diagnostics, ok := errors.AsType[build.Diagnostics](err); ok {
		return "", fmt.Sprintf("henia: rendering %s for %q stopped:\n%v\n", e.ID, r.harness, diagnostics)
	}
	if err != nil {
		return fmt.Sprintf("henia: rendering %s for %q failed (%v); canonical text follows\n\n%s", e.ID, r.harness, err, e.Artifact.Body), ""
	}
	if current, ok := inputs(refs); ok {
		if data, err := json.Marshal(cached{Refs: refs, Digest: current, Body: rendered.Body}); err == nil && os.MkdirAll(filepath.Dir(cache), 0o755) == nil {
			_ = os.WriteFile(cache, data, 0o644)
		}
	}
	return rendered.Body, ""
}

func digest(parts ...[]byte) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write(part)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r *renderer) targets() (map[string]map[string]string, error) {
	var layers []build.Dependency
	for _, p := range r.lib.Packages {
		if configured, err := config.Harnesses(r.config(p.Config)); err == nil {
			layers = append(layers, build.Dependency{Harnesses: configured})
		}
	}
	return build.Targets(nil, layers)
}

func (r *renderer) known(pkg string, targets map[string]map[string]string) map[string]bool {
	if known, ok := r.knowns[pkg]; ok {
		return known
	}
	var arts []*artifact.Artifact
	for _, other := range r.lib.Entries {
		if other.Package == pkg {
			arts = append(arts, other.Artifact)
		}
	}
	r.knowns[pkg] = build.Known(arts, targets)
	return r.knowns[pkg]
}

func (r *renderer) expand(e library.Entry, served map[string]bool, head bool) (*artifact.Artifact, []resolution, error) {
	targets, err := r.targets()
	if err != nil {
		return nil, nil, err
	}
	unit := func(e library.Entry, head bool) *build.Unit {
		art := *e.Artifact
		art.SourcePath = e.Path
		u := &build.Unit{Artifact: &art, Package: e.Package}
		name, harness, _ := r.harnessFor(e)
		if u.Transformer, u.Err = build.Renderer(e.Artifact, name, harness, served, head); u.Err != nil {
			return u
		}
		u.Root, u.User, u.Known = e.Origin.ProjectRoot(), componentUser(e), r.known(e.Package, targets)
		if name != "" {
			u.Variables = harness.Variables
		}
		return u
	}
	first := unit(e, head)
	if first.Err != nil {
		return nil, nil, first.Err
	}
	var refs []resolution
	resolved := map[components.Key]library.Entry{}
	build.Expand([]*build.Unit{first}, build.Target{
		Name: r.harness, Harnesses: targets,
		Resolve: func(from components.Unit, ref string) (components.Key, string, error) {
			address, err := reference.ParseAddress(ref)
			if err != nil {
				return components.Key{}, "", err
			}
			if address.Kind != reference.TypeSkill {
				return components.Key{}, "", fmt.Errorf("%s: henia show serves skills, not %ss", ref, address.Kind)
			}
			name, resource, anchor := address.Ref(), address.Resource, address.Anchor
			target, err := r.locate(from.Package, name)
			key := components.Key{Package: target.Package, Name: target.Name}
			if err == nil {
				refs = append(refs, resolution{from.Package, name, target.ID, resource})
				resolved[key] = target
			}
			key.Resource = resource
			return key, anchor, err
		},
		Load: func(k components.Key) *build.Unit {
			if target, ok := resolved[k]; ok {
				return unit(target, false)
			}
			return nil
		},
	})
	rendered, err := first.Render()
	return rendered, refs, err
}

func componentUser(e library.Entry) string {
	if e.Tier == library.Project {
		return library.ConfigDir()
	}
	return ""
}

func (r *renderer) locate(pkg, name string) (library.Entry, error) {
	if !strings.Contains(name, ":") {
		for _, e := range r.lib.Entries {
			if e.Package == pkg && e.Name == name {
				return e, nil
			}
		}
	}
	return r.lib.Resolve(name)
}

func (r *renderer) dependencies(closure []library.Entry) [][]byte {
	targets, _ := r.targets()
	var parts [][]byte
	for _, p := range r.lib.Packages {
		parts = append(parts, []byte(p.Config), r.config(p.Config))
	}
	packages := map[string]bool{}
	for _, e := range closure {
		canonical, _ := os.ReadFile(e.Path)
		parts = append(parts, []byte(e.ID), []byte(e.Path), canonical, r.config(e.Origin.Config))
		if packages[e.Origin.Root] {
			continue
		}
		packages[e.Origin.Root] = true
		parts = append(parts, []byte(strings.Join(slices.Sorted(maps.Keys(r.known(e.Package, targets))), " ")))
		decls, problems := components.Load(e.Origin.ProjectRoot(), componentUser(e))
		for _, name := range slices.Sorted(maps.Keys(decls)) {
			if path := decls[name].Path; path != "" {
				declared, _ := os.ReadFile(path)
				parts = append(parts, []byte(path), declared)
			}
		}
		for _, p := range problems {
			parts = append(parts, []byte(p.Path), []byte(p.Message))
		}
	}
	return parts
}

func (r *renderer) reference(e library.Entry) *regexp.Regexp {
	if r.harness == "" {
		return nil
	}
	harnesses, err := config.Harnesses(r.config(e.Origin.Config))
	if err != nil {
		return nil
	}
	return pipeline.ReferenceSyntax(harnesses[r.harness].References["skill"])
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
