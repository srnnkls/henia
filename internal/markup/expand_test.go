package markup

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

func describe(name string, args map[string]string, content string) (string, error) {
	var pairs []string
	for _, k := range slices.Sorted(maps.Keys(args)) {
		pairs = append(pairs, k+"="+args[k])
	}
	return fmt.Sprintf("%s [%s] %q", name, strings.Join(pairs, " "), content), nil
}

func only(names ...string) func(string) bool {
	return func(name string) bool { return slices.Contains(names, name) }
}

func TestExpand(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"explicit template with args", "Intro.\n\n:::note{template=\"banner\" title=\"Git\" tier=2}\nLead.\n:::\n\nTail.\n", "Intro.\n\n:::note\nbanner [tier=2 title=Git] \"Lead.\\n\"\n:::\n\nTail.\n"},
		{"bare directive uses its own template", ":::head\nLead.\n:::\n", ":::head\nhead [] \"Lead.\\n\"\n:::\n"},
		{"empty braces", ":::head{}\nLead.\n:::\n", ":::head\nhead [] \"Lead.\\n\"\n:::\n"},
		{"classes are flags", ":::head{.context .contents title=\"Git\"}\n:::\n", ":::head\nhead [contents=true context=true title=Git] \"\"\n:::\n"},
		{"blank body is empty", ":::head\n\n  \n:::\n", ":::head\nhead [] \"\"\n:::\n"},
		{"no template leaves the directive", ":::note{priority=high}\nNote.\n:::\n", ":::note{priority=high}\nNote.\n:::\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Expand(tc.source, only("head"), describe)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
