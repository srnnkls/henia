package pattern

import (
	"slices"
	"strings"
	"testing"
)

func selfLinked(t *testing.T) corpus {
	return newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\nIntro.\n"},
		"b": {"SKILL.md": "# B\n\nIntro.\n"},
		"c": {"SKILL.md": "# C\n\nIntro.\n\n## Usage\n\nC usage.\n"},
	}, map[string][]string{"a": {"a"}, "b": {"a", "c#usage"}})
}

func chained(t *testing.T) corpus {
	return newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\nIntro.\n"},
		"b": {"SKILL.md": "# B\n\nIntro.\n"},
		"c": {"SKILL.md": "# C\n\nIntro.\n\n## Usage\n\nC usage.\n"},
	}, map[string][]string{"a": {"b"}, "b": {"c#usage"}})
}

func idents(rows []Row) []string {
	var out []string
	for _, row := range rows {
		var cells []string
		for _, cell := range row.Cells {
			var parts []string
			for _, e := range cell.Elements {
				id := e.Attrs["id"]
				if e.Type == "link" {
					id = e.Text
				}
				parts = append(parts, id)
			}
			cells = append(cells, cell.Name+"="+strings.Join(parts, "+"))
		}
		out = append(out, strings.Join(cells, " "))
	}
	slices.Sort(out)
	return out
}

func TestOptionalRelationTargetFallsBackPerRow(t *testing.T) {
	c := chained(t)
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{"inbound", `(skill (inbound (skill)? @from)) @s`, []string{"from= s=a", "from=a s=b", "from=a s=c", "from=b s=c"}},
		{"from", `(skill (from (link)? @l)) @s`, []string{"l= s=a", "l= s=c", "l=b s=b"}},
		{"reaches", `(skill (reaches (skill)? @t)) @s`, []string{"t= s=c", "t=b s=a", "t=c s=a", "t=c s=b"}},
		{"to", `(link (to (section)? @t)) @l`, []string{"t= l=b", "t=usage l=c#usage"}},
		{"star target", `(skill (from (link)* @l)) @s`, []string{"l= s=a", "l= s=c", "l=b s=b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idents(run(t, c, tc.query))
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestOptionalSiblingFallsBackPerRow(t *testing.T) {
	c := selfLinked(t)
	got := idents(run(t, c, `(skill :id ?x (link :target (not ?x))? @l) @s`))
	want := []string{"l= s=a", "l= s=c", "l=a s=b", "l=c#usage s=b"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}
