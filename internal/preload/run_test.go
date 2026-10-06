package preload

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	RunSandboxHelper(os.Args)
	os.Exit(m.Run())
}

func stubPolicy(t *testing.T, script string) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gate"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	return bin
}

func unsandboxed(t *testing.T, policy ...string) *Runner {
	t.Helper()
	user := Settings{Unsandboxed: "run"}
	if len(policy) > 0 {
		user.Policy = policy[0]
	}
	r, err := NewRunner(user, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	r.Sandbox = func(string, []string) ([]string, bool) { return nil, false }
	return r
}

func TestRunPolicy(t *testing.T) {
	c := Context{Dir: t.TempDir(), Skill: "s", Package: "tropos", Tier: "global", Caller: "claude"}
	for _, tc := range []struct{ name, script, command, want string }{
		{"allow", `echo '{"decision":"allow"}'`, "echo ran", "ran\n"},
		{"deny names the rule", `echo '{"decision":"deny","rule":"no-echo","reason":"echo is off"}'`, "echo ran", "henia: blocked by gate/no-echo: echo is off"},
		{"rewrite", `echo '{"decision":"allow","command":"echo rewritten"}'`, "echo ran", "rewritten\n"},
		{"rewrite is checked again", `echo '{"decision":"allow","command":"git push"}'`, "echo ran", "henia: blocked by henia/git: git push changes the repository or a remote (after gate rewrote it)"},
		{"failure blocks", `echo boom >&2; exit 3`, "echo ran", "henia: blocked: gate failed: exit status 3: boom"},
		{"garbage blocks", `echo nope`, "echo ran", `henia: blocked: gate failed: unreadable response "nope"`},
		{"builtin refusal skips the policy", `echo '{"decision":"allow"}'`, "git push", "henia: blocked by henia/git: git push changes the repository or a remote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubPolicy(t, tc.script)
			if got := unsandboxed(t, "gate check --caller henia").Run(context.Background(), tc.command, c); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	bin := stubPolicy(t, `echo "$@" > "$(dirname "$0")/args"; cat > "$(dirname "$0")/input"; echo '{"decision":"allow"}'`)
	unsandboxed(t, `gate check --caller 'henia preload'`).Run(context.Background(), "echo hi", c)
	args, _ := os.ReadFile(filepath.Join(bin, "args"))
	input, _ := os.ReadFile(filepath.Join(bin, "input"))
	want := `{"caller":"claude","command":"echo hi","cwd":"` + c.Dir + `","package":"tropos","skill":"s","tier":"global"}`
	if string(args) != "check --caller henia preload\n" || string(input) != want {
		t.Fatalf("policy got args %q and input %s, want input %s", args, input, want)
	}
}

func TestRunWithoutPolicyAsksNoOne(t *testing.T) {
	stubPolicy(t, `echo '{"decision":"deny","rule":"all"}'`)
	if got := unsandboxed(t).Run(context.Background(), "echo ran", Context{Dir: t.TempDir()}); got != "ran\n" {
		t.Fatalf("got %q", got)
	}
}

func TestPolicySettings(t *testing.T) {
	for _, tc := range []struct {
		name          string
		user, project Settings
		want          string
	}{
		{"project policy", Settings{}, Settings{Policy: "gate check"}, "preload.policy is honoured only in the user henia.toml"},
		{"empty command", Settings{Policy: "  "}, Settings{}, `preload.policy "  " is not a command`},
		{"unclosed quote", Settings{Policy: "gate 'check"}, Settings{}, `preload.policy "gate 'check" is not a command`},
	} {
		if _, err := NewRunner(tc.user, tc.project); err == nil || err.Error() != tc.want {
			t.Errorf("%s: error %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestRunPolicyTimeout(t *testing.T) {
	stubPolicy(t, "sleep 5; echo late")
	r := unsandboxed(t, "gate check")
	r.PolicyTimeout = 200 * time.Millisecond
	start := time.Now()
	if got := r.Run(context.Background(), "echo ran", Context{Dir: t.TempDir()}); got != "henia: blocked: gate failed: timed out after 200ms" {
		t.Fatalf("got %q", got)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("policy timeout took %s", elapsed)
	}
}

func TestRunLimits(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	c := Context{Dir: t.TempDir()}
	r := unsandboxed(t)
	r.Timeout, r.Output = 200*time.Millisecond, 10
	start := time.Now()
	if got := r.Run(context.Background(), "echo started; sleep 5", c); got != "started\nhenia: timed out after 200ms" {
		t.Fatalf("timeout: %q", got)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	if got := r.Run(context.Background(), "printf 0123456789abcdef", c); got != "0123456789\nhenia: output truncated at 10 bytes" {
		t.Fatalf("budget: %q", got)
	}
	if got := r.Run(context.Background(), "echo out; echo err >&2; exit 4", c); got != "out\nerr\nhenia: exit status 4" {
		t.Fatalf("failure: %q", got)
	}
}

func TestRunSandbox(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	dir := t.TempDir()
	r, err := NewRunner(Settings{}, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sandbox(dir, []string{"true"}); !ok {
		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			t.Skip("no sandbox available in this environment")
		}
	}
	got := r.Run(context.Background(), `awk 'BEGIN { print "x" > "written.txt" }'; echo after`, Context{Dir: dir})
	if _, err := os.Stat(filepath.Join(dir, "written.txt")); !os.IsNotExist(err) {
		t.Fatalf("sandbox let a write through: %q", got)
	}
	if !strings.Contains(got, "after") {
		t.Fatalf("command did not run: %q", got)
	}
	r.Sandbox = func(string, []string) ([]string, bool) { return nil, false }
	if got := r.Run(context.Background(), "echo hi", Context{Dir: dir}); !strings.HasPrefix(got, "henia: not run: no write sandbox") {
		t.Fatalf("unsandboxed default: %q", got)
	}
}

func TestNewRunner(t *testing.T) {
	r, err := NewRunner(Settings{Timeout: "3s", PolicyTimeout: "2s", Refuse: []Rule{{Command: "a"}}}, Settings{Output: 5, PolicyTimeout: "4s", Refuse: []Rule{{Command: "b"}}})
	if err != nil || r.Timeout != 3*time.Second || r.PolicyTimeout != 4*time.Second || r.Output != 5 || len(r.Refuse) != 2 || r.RunUnsandboxed {
		t.Fatalf("merge: %+v %v", r, err)
	}
	for _, bad := range []struct{ user, project Settings }{
		{Settings{Timeout: "soon"}, Settings{}},
		{Settings{}, Settings{Timeout: "-1s"}},
		{Settings{PolicyTimeout: "0s"}, Settings{}},
		{Settings{}, Settings{PolicyTimeout: "later"}},
		{Settings{}, Settings{Output: -1}},
		{Settings{}, Settings{Unsandboxed: "run"}},
		{Settings{Unsandboxed: "always"}, Settings{}},
	} {
		if _, err := NewRunner(bad.user, bad.project); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestExpand(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	r := unsandboxed(t)
	body := "# S\n\n- Branch: !`echo main`\n- Next\n\nInline !`echo a` and text.\n\n```!\necho one\necho two\n```\n\nKeep `!date` and `` !`date` ``.\n"
	want := "# S\n\n- Branch:\n  ```text\n  $ echo main\n  main\n  ```\n- Next\n\nInline\n```text\n$ echo a\na\n```\nand text.\n\n```text\n$ echo one\n> echo two\none\ntwo\n```\n\nKeep `!date` and `` !`date` ``.\n"
	if got := r.Expand(context.Background(), body, Context{Dir: t.TempDir()}); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := r.Expand(context.Background(), "!`echo '```'`\n", Context{Dir: t.TempDir()}); got != "\n````text\n$ echo '```'\n```\n````\n" {
		t.Fatalf("fence: %q", got)
	}
}
