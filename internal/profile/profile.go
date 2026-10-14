package profile

import (
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/expr-lang/expr/vm"
	"github.com/pelletier/go-toml/v2"
	"github.com/srnnkls/henia/internal/canonical"
	"github.com/srnnkls/henia/internal/expression"
	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.toml
var bundled embed.FS

type File struct {
	Path   string `toml:"path"` // Relative to harness root; {name} expands to artifact name.
	Format string `toml:"format"`
	Value  string `toml:"value"` // Expr expression, not a template.
}

// Harness is everything Henia knows about one harness: harness-wide defaults
// and one section per artifact type.
type Harness struct {
	Directives string         `toml:"directives"`
	Commands   []string       `toml:"commands"` // Built-in command names, used by lint.
	Agents     []string       `toml:"agents"`   // Built-in agent names, used by lint.
	Skill      *Profile       `toml:"skill"`
	Agent      *Profile       `toml:"agent"`
	Command    map[string]any `toml:"command"` // Reserved: commands are not mapped yet.
	Config     Config         `toml:"config"`
}

// Profile maps canonical metadata of one artifact type onto a harness.
type Profile struct {
	Fields   []string          `toml:"fields"`
	Consume  []string          `toml:"consume"`
	Controls []string          `toml:"controls"`
	Aliases  map[string]string `toml:"aliases"`
	Computed map[string]string `toml:"computed"`
	Files    map[string]File   `toml:"files"`
	Preloads bool              `toml:"preloads"`
}

// skillKeys are the top-level keys of the flat profile form, read as [skill].
var skillKeys = []string{"fields", "consume", "controls", "aliases", "computed", "files", "preloads"}

type Context struct {
	Name       string            `expr:"name"`
	Profile    string            `expr:"profile"`
	Path       string            `expr:"path"`
	Variables  map[string]string `expr:"variables"`
	Tools      map[string]string `expr:"tools"`
	Keys       map[string]string `expr:"keys"`
	Install    string            `expr:"install"`
	InstallDir string            `expr:"install_dir"`
}

type environment struct {
	Input     map[string]any                                      `expr:"input"`
	Settings  map[string]any                                      `expr:"settings"`
	Native    map[string]any                                      `expr:"native"`
	Context   Context                                             `expr:"ctx"`
	ToolNames func(any, map[string]string) (string, error)        `expr:"toolNames"`
	Merge     func(map[string]any, map[string]any) map[string]any `expr:"merge"`
	Require   func(any, string) (any, error)                      `expr:"require"`
}

type Compiler struct {
	profile  Profile
	computed map[string]*vm.Program
	files    map[string]*vm.Program
}

type Result struct {
	Frontmatter map[string]any
	Files       map[string][]byte
	Modes       map[string]os.FileMode
	Warnings    []string
}

// Load merges a profile's tables from bundled, user and project definitions.
// A custom name needs only a TOML file, never a Go change.
func Load(name, project, user string) (Harness, error) {
	if !filepath.IsLocal(name) || filepath.Base(name) != name || name == "." {
		return Harness{}, fmt.Errorf("invalid profile name %q", name)
	}
	merged := map[string]any{}
	found := false
	paths := []string{"profiles/" + name + ".toml", filepath.Join(user, "harnesses", name, "transform.toml"), filepath.Join(project, ".henia", "harnesses", name, "transform.toml")}
	for i, path := range paths {
		if i == 1 && user == "" {
			continue
		}
		var data []byte
		var err error
		if i == 0 {
			data, err = bundled.ReadFile(path)
		} else {
			data, err = os.ReadFile(path)
		}
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Harness{}, err
		}
		found = true
		var layer map[string]any
		if err := toml.Unmarshal(data, &layer); err != nil {
			return Harness{}, fmt.Errorf("%s: %w", path, err)
		}
		if layer, err = sectioned(layer); err != nil {
			return Harness{}, fmt.Errorf("%s: %w", path, err)
		}
		merged = Merge(merged, layer)
	}
	if !found {
		return Harness{}, fmt.Errorf("unknown profile %q", name)
	}
	data, err := toml.Marshal(merged)
	if err != nil {
		return Harness{}, err
	}
	var h Harness
	if err := toml.NewDecoder(strings.NewReader(string(data))).DisallowUnknownFields().Decode(&h); err != nil {
		return Harness{}, err
	}
	switch h.Directives {
	case "", "xml", "markdown", "md", "keep":
	default:
		return Harness{}, fmt.Errorf("directives must be xml, markdown, md or keep, got %q", h.Directives)
	}
	if len(h.Command) > 0 {
		return Harness{}, fmt.Errorf("[command] is reserved; commands are not mapped through profiles")
	}
	return h, nil
}

// sectioned moves one layer's flat skill keys under [skill]. Each layer is
// normalized on its own, so a flat user layer merges over a sectioned bundle.
func sectioned(layer map[string]any) (map[string]any, error) {
	var flat []string
	for _, key := range skillKeys {
		if _, ok := layer[key]; ok {
			flat = append(flat, key)
		}
	}
	if len(flat) == 0 {
		return layer, nil
	}
	if _, ok := layer["skill"]; ok {
		return nil, fmt.Errorf("top-level %s next to [skill]; move them under [skill]", strings.Join(flat, ", "))
	}
	result, skill := maps.Clone(layer), map[string]any{}
	for _, key := range flat {
		skill[key] = layer[key]
		delete(result, key)
	}
	result["skill"] = skill
	return result, nil
}

// Section is the profile an artifact type renders with, or nil. A per-type
// profile written in the flat form serves agents through its skill section.
func (h Harness) Section(kind string, perType bool) *Profile {
	switch kind {
	case "skill":
		return h.Skill
	case "agent":
		if h.Agent == nil && perType {
			return h.Skill
		}
		return h.Agent
	}
	return nil
}

// Merge follows the scope's table/scalar/array precedence. Inputs stay immutable.
func Merge(base, override map[string]any) map[string]any { return merge(base, override, true) }

// Replace merges like Merge, except that a later array replaces an earlier one.
func Replace(base, override map[string]any) map[string]any { return merge(base, override, false) }

func merge(base, override map[string]any, concat bool) map[string]any {
	result := maps.Clone(base)
	if result == nil {
		result = map[string]any{}
	}
	for key, value := range override {
		if left, ok := result[key].(map[string]any); ok {
			if right, ok := value.(map[string]any); ok {
				result[key] = merge(left, right, concat)
				continue
			}
		}
		if left, ok := result[key].([]any); ok && concat {
			if right, ok := value.([]any); ok {
				result[key] = append(slices.Clone(left), right...)
				continue
			}
		}
		result[key] = value
	}
	return result
}

func NewCompiler(profile Profile) (*Compiler, error) {
	c := &Compiler{profile: profile, computed: map[string]*vm.Program{}, files: map[string]*vm.Program{}}
	if len(profile.Fields) == 0 {
		return nil, fmt.Errorf("profile requires a nonempty fields list")
	}
	for key, source := range profile.Computed {
		if !slices.Contains(profile.Fields, key) {
			return nil, fmt.Errorf("computed field %q is not in profile fields", key)
		}
		program, err := expression.Compile(source, environment{}, false)
		if err != nil {
			return nil, fmt.Errorf("computed %s: %w", key, err)
		}
		c.computed[key] = program
	}
	for key, file := range profile.Files {
		if file.Format != "yaml" && file.Format != "json" && file.Format != "text" {
			return nil, fmt.Errorf("file %s: format must be yaml, json or text", key)
		}
		if err := validPath(strings.ReplaceAll(file.Path, "{name}", "skill")); err != nil {
			return nil, err
		}
		program, err := expression.Compile(file.Value, environment{}, false)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", key, err)
		}
		c.files[key] = program
	}
	return c, nil
}

func validPath(path string) error {
	if !filepath.IsLocal(path) || filepath.Clean(path) != path || path == "." || strings.Contains(path, "\\") {
		return fmt.Errorf("invalid emitted file path %q", path)
	}
	return nil
}

func (c *Compiler) Compile(directory string, input map[string]any, context Context) (*Result, error) {
	r := &Result{Frontmatter: map[string]any{}, Files: map[string][]byte{}}
	settings, err := object(input["henia"], "henia")
	if err != nil {
		return nil, err
	}
	settings = maps.Clone(settings)
	if _, err := object(settings["variables"], "henia.variables"); err != nil {
		return nil, err
	}
	targets, err := object(settings["targets"], "henia.targets")
	if err != nil {
		return nil, err
	}
	native, err := object(targets[context.Profile], "henia.targets."+context.Profile)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"auto_invoke", "user_invocable"} {
		if value, exists := native[key]; exists {
			settings[key] = value
		}
		if value := settings[key]; value != nil {
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			if !slices.Contains(c.profile.Controls, key) {
				r.Warnings = append(r.Warnings, "no mapping for "+key+"; omitted")
			}
		}
	}
	resources, err := object(settings["resources"], "henia.resources")
	if err != nil {
		return nil, err
	}
	for key, value := range resources {
		if _, ok := value.(bool); key != "disclosure" || !ok {
			return nil, fmt.Errorf("henia.resources accepts only disclosure, a boolean")
		}
	}
	for key := range settings {
		if !slices.Contains([]string{"auto_invoke", "user_invocable", "variables", "targets", "resources", "lint", "slots", "provides", "layout"}, key) {
			return nil, fmt.Errorf("unknown henia field %q", key)
		}
	}
	for key := range native {
		if key != "frontmatter" && key != "auto_invoke" && key != "user_invocable" {
			if _, ok := c.profile.Files[key]; !ok {
				return nil, fmt.Errorf("unknown target option %q", key)
			}
		}
	}
	fm := maps.Clone(input)
	delete(fm, "henia")
	for _, key := range slices.Sorted(maps.Keys(c.profile.Aliases)) {
		value, exists := fm[key]
		if !exists {
			continue
		}
		alias := c.profile.Aliases[key]
		if _, exists := fm[alias]; exists {
			return nil, fmt.Errorf("conflicting fields %s and %s", key, alias)
		}
		fm[alias] = value
		delete(fm, key)
	}
	env := environment{input, settings, native, context, toolList, Merge, func(value any, message string) (any, error) {
		if value == nil || value == "" {
			return nil, fmt.Errorf("%s", message)
		}
		return value, nil
	}}
	for _, key := range slices.Sorted(maps.Keys(c.computed)) {
		value, err := expression.Run(c.computed[key], env)
		if err != nil {
			return nil, fmt.Errorf("computed %s: %w", key, err)
		}
		if value != nil {
			fm[key] = value
		}
	}
	for _, key := range c.profile.Consume {
		delete(fm, key)
	}
	overrides, err := object(native["frontmatter"], "target.frontmatter")
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		if value == nil {
			delete(fm, key)
		} else {
			fm[key] = value
		}
	}
	for _, key := range slices.Sorted(maps.Keys(fm)) {
		if !slices.Contains(c.profile.Fields, key) {
			r.Warnings = append(r.Warnings, "unsupported frontmatter "+key+"; omitted")
			continue
		}
		r.Frontmatter[key] = fm[key]
	}
	// Normalize list-shaped native tool declarations to the portable spelling;
	// native names are already vendor names and must not be translated again.
	for _, key := range []string{"allowed-tools", "disallowed-tools"} {
		if value, exists := r.Frontmatter[key]; exists {
			normalized, err := toolList(value, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			r.Frontmatter[key] = normalized
		}
	}
	if err := validate(r.Frontmatter, directory); err != nil {
		return nil, err
	}
	if _, err := canonical.Parse(r.Frontmatter); err != nil {
		return nil, fmt.Errorf("output: %w", err)
	}
	if _, err := json.Marshal(r.Frontmatter); err != nil {
		return nil, fmt.Errorf("frontmatter must contain serializable data: %w", err)
	}
	for _, key := range slices.Sorted(maps.Keys(c.files)) {
		value, err := expression.Run(c.files[key], env)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", key, err)
		}
		if value == nil {
			continue
		}
		if object, ok := value.(map[string]any); ok && len(object) == 0 {
			continue
		}
		if _, err := json.Marshal(value); err != nil {
			return nil, fmt.Errorf("file %s must contain serializable data: %w", key, err)
		}
		file := c.profile.Files[key]
		path := strings.ReplaceAll(file.Path, "{name}", directory)
		if err := validPath(path); err != nil {
			return nil, err
		}
		if strings.HasSuffix(path, "/agents/openai.yaml") {
			object, err := object(value, "openai sidecar")
			if err != nil {
				return nil, err
			}
			if err := validateOpenAI(object); err != nil {
				return nil, err
			}
		}
		var data []byte
		switch file.Format {
		case "yaml":
			data, err = yaml.Marshal(value)
		case "json":
			data, err = json.MarshalIndent(value, "", "  ")
			data = append(data, '\n')
		case "text":
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("text file %s must evaluate to string", key)
			}
			data = []byte(text)
		}
		if err != nil {
			return nil, err
		}
		if _, exists := r.Files[path]; exists {
			return nil, fmt.Errorf("duplicate emitted file %s", path)
		}
		r.Files[path] = data
	}
	return r, nil
}
