package reference

import (
	"testing"
)

func TestRenderPrecedenceAndProvenance(t *testing.T) {
	body := "Use `$own`, `$lib`, `!read`, `!other` and `@scout`.\n"
	out, rendered, dropped, err := Render(body, Rendering{
		Tools:      map[string]string{"read": "Read"},
		Served:     map[string]bool{"lib": true},
		References: map[string]string{"skill": "/{?name}", "tool": "{{.Name}}!"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Use `/own`, `henia show lib`, `Read`, `other!` and `@scout`.\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if len(dropped) > 0 {
		t.Errorf("dropped = %v", dropped)
	}
	want := map[string]string{"own": "skill-reference", "lib": "served-skill", "read": "mapped-tool", "other": "tool-reference", "scout": ""}
	for _, r := range rendered {
		if rule, ok := want[r.Name]; !ok || rule != r.Rewrite {
			t.Errorf("%s rendered by %q, want %q", r.Name, r.Rewrite, rule)
		}
		if r.Name == "scout" && out[r.Start:r.End] != "`@scout`" || r.Name == "own" && out[r.Start:r.End] != "`/own`" {
			t.Errorf("%s spans %q", r.Name, out[r.Start:r.End])
		}
	}
	if len(rendered) != len(want) {
		t.Errorf("%d rendered references, want %d", len(rendered), len(want))
	}
}
