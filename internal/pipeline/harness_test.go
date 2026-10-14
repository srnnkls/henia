package pipeline

import (
	"testing"

	"github.com/srnnkls/henia"
	"github.com/srnnkls/henia/internal/artifact"
)

func TestTransformerProfiles(t *testing.T) {
	h := henia.Harness{Profile: "claude", ProjectRoot: t.TempDir()}
	for kind, want := range map[artifact.Type]string{artifact.TypeSkill: "claude", artifact.TypeAgent: "", artifact.TypeCommand: ""} {
		effective, tr, err := Transformer("claude", "", h, kind)
		if err != nil || effective.Profile != want || (tr.Compiler != nil) != (want != "") {
			t.Errorf("%s: %q %v", kind, effective.Profile, err)
		}
		if effective.Directives != "xml" {
			t.Errorf("%s: directives %q", kind, effective.Directives)
		}
	}
	h.Commands.Profile = "claude"
	effective, tr, err := Transformer("claude", "", h, artifact.TypeCommand)
	if err != nil || effective.Profile != "claude" || tr.Compiler != nil {
		t.Errorf("a per-type command profile no longer reaches preloads: %q %v", effective.Profile, err)
	}
}
