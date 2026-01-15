// CUE Context Schema
// This schema defines the structure of data available in CUE blocks

// Top-level: Frontmatter fields (user-defined)
// Example frontmatter keys available at top level:
// - name: string
// - enabled: bool
// - env: string
// - models: [string, ...]

// Reserved namespace: config (read-only, provided by Henia)
config: {
	// Harness configuration
	harness: {
		// Harness name (e.g., "claude", "cursor")
		name: string

		// Output format ("xml", "directives", etc.)
		format: string

		// User-defined variables from henia.toml
		variables: {
			[string]: _
		}
	}
}

// Example CUE block usage:
//
// %cue {{
// // Frontmatter fields available at top level
// env: "dev" | "staging" | "prod"
//
// // Define schema
// #Tool: {
//   name: string
//   safe: bool | *true
// }
//
// // Validate and compute
// tools: [...#Tool] & [
//   {name: "kubectl"},
//   {name: "rm", safe: false},
// ]
//
// has_unsafe: or([for t in tools { !t.safe }])
//
// // Access config
// output_format: config.harness.format
// }}

// Constraints:
// - Frontmatter must not define "config" key
// - CUE blocks may read but not override config
// - All frontmatter values unified with CUE source
// - Final value decoded to map[string]any for Go templates
