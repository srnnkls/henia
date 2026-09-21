package henia

type ArtifactMapping struct {
	Profile   string                       `toml:"profile,omitempty"`
	Structure string                       `toml:"structure,omitempty"`
	Keys      map[string]string            `toml:"keys,omitempty"`
	Values    map[string]map[string]string `toml:"values,omitempty"`
}

// File adds a supporting document to a harness output, relative to the source root.
type File struct {
	Source  string            `toml:"source"`
	Replace map[string]string `toml:"replace,omitempty"`
}

type ReferenceConfig struct {
	Output string `toml:"output,omitempty"`
}

type Harness struct {
	Files                      map[string]File              `toml:"files,omitempty"`
	Profile                    string                       `toml:"profile,omitempty"`
	Strict                     bool                         `toml:"strict,omitempty"`
	Format                     string                       `toml:"format,omitempty"`
	Structure                  string                       `toml:"structure,omitempty"` // "flat" or "nested" (default)
	GenerateCommandsFromSkills bool                         `toml:"generate_commands_from_skills,omitempty"`
	Artifacts                  []string                     `toml:"artifacts,omitempty"`
	Keys                       map[string]string            `toml:"keys,omitempty"`
	Values                     map[string]map[string]string `toml:"values,omitempty"`
	ArtifactMappings           map[string]ArtifactMapping   `toml:"artifact_mappings,omitempty"`
	Variables                  map[string]string            `toml:"variables,omitempty"`
	Tools                      map[string]string            `toml:"tools,omitempty"`
	References                 map[string]ReferenceConfig   `toml:"references,omitempty"`
	Include                    []string                     `toml:"include,omitempty"`
	Exclude                    []string                     `toml:"exclude,omitempty"`
	ProjectRoot                string                       `toml:"-"`
	UserRoot                   string                       `toml:"-"`
}
