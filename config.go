package henia

import "slices"

// File adds a supporting document to a harness output, relative to the source root.
type File struct {
	Source  string            `toml:"source"`
	Replace map[string]string `toml:"replace,omitempty"`
}

type Frontmatter struct {
	Rename map[string]string            `toml:"rename,omitempty"`
	Values map[string]map[string]string `toml:"values,omitempty"`
}

type Artifacts struct {
	Profile     string      `toml:"profile,omitempty"`
	Layout      string      `toml:"layout,omitempty"`
	Frontmatter Frontmatter `toml:"frontmatter,omitempty"`
	Default     string      `toml:"default,omitempty"`
	Static      []string    `toml:"static,omitempty"`
	Dynamic     []string    `toml:"dynamic,omitempty"`
	Hybrid      []string    `toml:"hybrid,omitempty"`
	Catalog     *bool       `toml:"catalog,omitempty"`
}

const (
	Static  = "static"
	Dynamic = "dynamic"
	Hybrid  = "hybrid"
)

type Harness struct {
	Files       map[string]File   `toml:"files,omitempty"`
	Profile     string            `toml:"profile,omitempty"`
	Strict      bool              `toml:"strict,omitempty"`
	Directives  string            `toml:"directives,omitempty"`
	Layout      string            `toml:"layout,omitempty"`
	Artifacts   []string          `toml:"artifacts,omitempty"`
	Frontmatter Frontmatter       `toml:"frontmatter,omitempty"`
	Variables   map[string]string `toml:"variables,omitempty"`
	Tools       map[string]string `toml:"tools,omitempty"`
	References  map[string]string `toml:"references,omitempty"`
	Skills      Artifacts         `toml:"skills,omitempty"`
	Agents      Artifacts         `toml:"agents,omitempty"`
	Commands    Artifacts         `toml:"commands,omitempty"`
	ProjectRoot string            `toml:"-"`
	UserRoot    string            `toml:"-"`
}

func (h Harness) Type(dir string) Artifacts {
	switch dir {
	case "skills":
		return h.Skills
	case "agents":
		return h.Agents
	case "commands":
		return h.Commands
	}
	return Artifacts{}
}

func (h Harness) Mode(dir, name string) string {
	a := h.Type(dir)
	switch {
	case slices.Contains(a.Static, name):
		return Static
	case slices.Contains(a.Hybrid, name):
		return Hybrid
	case slices.Contains(a.Dynamic, name):
		return Dynamic
	case a.Default != "":
		return a.Default
	case len(a.Static) > 0 || len(a.Hybrid) > 0:
		return Dynamic
	}
	return Static
}

func (h Harness) Builds(dir, name string) bool { return h.Mode(dir, name) != Dynamic }

func (h Harness) Catalog() bool {
	return h.Skills.Catalog == nil || *h.Skills.Catalog
}
