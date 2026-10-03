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
