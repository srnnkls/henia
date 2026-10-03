package caller

import "testing"

func TestHarness(t *testing.T) {
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	nested := []string{"/bin/zsh -lc henia show x", "/opt/codex/bin/codex exec", "-zsh", "/usr/local/bin/claude -p"}
	for _, c := range []struct {
		name      string
		flag      string
		vars      map[string]string
		ancestors []string
		want      string
	}{
		{"flag wins", "pi", map[string]string{"HENIA_HARNESS": "codex"}, nested, "pi"},
		{"explicit neutral", "", map[string]string{"HENIA_HARNESS": "none"}, nested, ""},
		{"nearest ancestor over leaked markers", "", map[string]string{"AI_AGENT": "claude-code_2-1-285_agent", "CLAUDECODE": "1", "CODEX_THREAD_ID": "t"}, nested, "codex"},
		{"node-hosted agent", "", nil, []string{"bash -c henia", "node /usr/lib/node_modules/pi/dist/pi.js"}, "pi"},
		{"AI_AGENT without agent ancestors", "", map[string]string{"AI_AGENT": "claude-code_2-1-285_agent"}, []string{"-zsh"}, "claude"},
		{"unknown AI_AGENT names itself", "", map[string]string{"AI_AGENT": "copilot_1_agent"}, nil, "copilot"},
		{"codex marker", "", map[string]string{"CODEX_THREAD_ID": "t", "CLAUDECODE": "1"}, nil, "codex"},
		{"nothing", "", nil, []string{"-zsh", "tmux"}, ""},
	} {
		if got := Harness(c.flag, env(c.vars), c.ancestors); got != c.want {
			t.Errorf("%s: Harness = %q; want %q", c.name, got, c.want)
		}
	}
}
