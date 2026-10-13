package reference

import "testing"

func TestParseAddress(t *testing.T) {
	for in, want := range map[string]string{
		"git":                         "skill:git",
		"$git#commits":                "skill:git#commits",
		"skill:git#commits":           "skill:git#commits",
		"tropos:git/reference/x.md#a": "skill:tropos:git/reference/x.md#a",
		"git.reference.worktree":      "skill:git.reference.worktree",
		"@reviewer#output":            "agent:reviewer#output",
		"agent:reviewer":              "agent:reviewer",
		"@tropos:reviewer":            "agent:tropos:reviewer",
		"agent:tropos:reviewer":       "agent:tropos:reviewer",
		"/plan#steps":                 "command:plan#steps",
		"tropos.io:git.reference":     "skill:tropos.io:git.reference",
	} {
		a, err := ParseAddress(in)
		if err != nil || a.String() != want {
			t.Errorf("ParseAddress(%q) = %q, %v; want %q", in, a.String(), err, want)
		}
	}
	for _, in := range []string{"!bash", "#notes.md", "", "Git", "a:b:c:d", "@reviewer/x.md"} {
		if a, err := ParseAddress(in); err == nil {
			t.Errorf("ParseAddress(%q) = %q, want an error", in, a)
		}
	}
}
