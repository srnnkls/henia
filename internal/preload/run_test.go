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

func stubFAS(t *testing.T, script string) string {
	t.Helper()
	bin := t.TempDir()
	if script != "" {
		if err := os.WriteFile(filepath.Join(bin, "fas"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	return bin
}

func unsandboxed(t *testing.T) *Runner {
	t.Helper()
	r, err := NewRunner(Settings{Unsandboxed: "run"}, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	r.Sandbox = func(string, []string) ([]string, bool) { return nil, false }
	return r
}

func TestRunFAS(t *testing.T) {
	c := Context{Dir: t.TempDir(), Skill: "s", Source: "tropos", Tier: "global", Caller: "claude"}
	for _, tc := range []struct{ name, fas, command, want string }{
		{"no fas runs", "", "echo ran", "ran\n"},
		{"allow", `echo '{"decision":"allow"}'`, "echo ran", "ran\n"},
		{"deny names the rule", `echo '{"decision":"deny","rule":"no-echo","reason":"echo is off"}'`, "echo ran", "henia: blocked by fas/no-echo: echo is off"},
		{"rewrite", `echo '{"decision":"allow","command":"echo rewritten"}'`, "echo ran", "rewritten\n"},
		{"rewrite is checked again", `echo '{"decision":"allow","command":"git push"}'`, "echo ran", "henia: blocked by henia/git: git push changes the repository or a remote (after fas rewrote it)"},
		{"failure blocks", `echo boom >&2; exit 3`, "echo ran", "henia: blocked: fas failed: exit status 3: boom"},
		{"garbage blocks", `echo nope`, "echo ran", `henia: blocked: fas failed: unreadable response "nope"`},
		{"builtin refusal skips fas", `echo '{"decision":"allow"}'`, "git push", "henia: blocked by henia/git: git push changes the repository or a remote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubFAS(t, tc.fas)
			if got := unsandboxed(t).Run(context.Background(), tc.command, c); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	bin := stubFAS(t, `cat > "$(dirname "$0")/input"; echo '{"decision":"allow"}'`)
	unsandboxed(t).Run(context.Background(), "echo hi", c)
	fas, _ := os.ReadFile(filepath.Join(bin, "input"))
	want := `{"caller":"claude","command":"echo hi","cwd":"` + c.Dir + `","skill":"s","source":"tropos","tier":"global"}`
	if string(fas) != want {
		t.Fatalf("fas input %s, want %s", fas, want)
	}
}

func TestRunFASTimeout(t *testing.T) {
	stubFAS(t, "sleep 5; echo late")
	r := unsandboxed(t)
	r.FASTimeout = 200 * time.Millisecond
	start := time.Now()
	if got := r.Run(context.Background(), "echo ran", Context{Dir: t.TempDir()}); got != "henia: blocked: fas failed: timed out after 200ms" {
		t.Fatalf("got %q", got)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("fas timeout took %s", elapsed)
	}
}

func TestRunLimits(t *testing.T) {
	stubFAS(t, "")
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
	stubFAS(t, "")
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
	r, err := NewRunner(Settings{Timeout: "3s", FASTimeout: "2s", Refuse: []Rule{{Command: "a"}}}, Settings{Output: 5, FASTimeout: "4s", Refuse: []Rule{{Command: "b"}}})
	if err != nil || r.Timeout != 3*time.Second || r.FASTimeout != 4*time.Second || r.Output != 5 || len(r.Refuse) != 2 || r.RunUnsandboxed {
		t.Fatalf("merge: %+v %v", r, err)
	}
	for _, bad := range []struct{ user, project Settings }{
		{Settings{Timeout: "soon"}, Settings{}},
		{Settings{}, Settings{Timeout: "-1s"}},
		{Settings{FASTimeout: "0s"}, Settings{}},
		{Settings{}, Settings{FASTimeout: "later"}},
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
	stubFAS(t, "")
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
