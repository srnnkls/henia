package preload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/shell"
)

const (
	DefaultTimeout       = 10 * time.Second
	DefaultOutput        = 8000
	DefaultPolicyTimeout = 10 * time.Second
)

type Settings struct {
	Timeout       string `toml:"timeout,omitempty"`
	Policy        string `toml:"policy,omitempty"`
	PolicyTimeout string `toml:"policy_timeout,omitempty"`
	Output        int    `toml:"output,omitempty"`
	Unsandboxed   string `toml:"unsandboxed,omitempty"`
	Refuse        []Rule `toml:"refuse,omitempty"`
}

type Context struct {
	Dir     string
	Skill   string
	Package string
	Tier    string
	Caller  string
	// Resolve renders a command in-process as Markdown; ok reports whether it did.
	Resolve func(command string) (markdown string, ok bool)
}

type Runner struct {
	Timeout        time.Duration
	Policy         []string
	PolicyTimeout  time.Duration
	Output         int
	RunUnsandboxed bool
	Refuse         []Rule
	Sandbox        func(dir string, command []string) ([]string, bool)
}

func NewRunner(user, project Settings) (*Runner, error) {
	r := &Runner{Timeout: DefaultTimeout, PolicyTimeout: DefaultPolicyTimeout, Output: DefaultOutput, Sandbox: sandbox}
	for _, layer := range []struct {
		Settings
		user bool
	}{{user, true}, {project, false}} {
		for _, setting := range []struct {
			name, value string
			target      *time.Duration
		}{{"timeout", layer.Timeout, &r.Timeout}, {"policy_timeout", layer.PolicyTimeout, &r.PolicyTimeout}} {
			if setting.value == "" {
				continue
			}
			d, err := time.ParseDuration(setting.value)
			if err != nil || d <= 0 {
				return nil, fmt.Errorf("preload.%s %q is not a positive duration", setting.name, setting.value)
			}
			*setting.target = d
		}
		if layer.Output < 0 {
			return nil, fmt.Errorf("preload.output must be positive, got %d", layer.Output)
		}
		if layer.Output > 0 {
			r.Output = layer.Output
		}
		switch layer.Unsandboxed {
		case "", "skip":
		case "run":
			if !layer.user {
				return nil, errors.New(`preload.unsandboxed = "run" is honoured only in the user henia.toml`)
			}
			r.RunUnsandboxed = true
		default:
			return nil, fmt.Errorf(`preload.unsandboxed must be "skip" or "run", got %q`, layer.Unsandboxed)
		}
		if layer.Policy != "" {
			if !layer.user {
				return nil, errors.New("preload.policy is honoured only in the user henia.toml")
			}
			words, err := shell.Fields(layer.Policy, nil)
			if err != nil || len(words) == 0 {
				return nil, fmt.Errorf("preload.policy %q is not a command", layer.Policy)
			}
			r.Policy = words
		}
		r.Refuse = append(r.Refuse, layer.Refuse...)
	}
	return r, nil
}

func (r *Runner) Expand(ctx context.Context, body string, c Context) string {
	return splice(body, func(p Preload) (string, bool, bool) {
		if markdown, ok := c.resolve(p.Command); ok {
			return markdown, true, true
		}
		return fence(p.Indent, p.Command, r.Run(ctx, p.Command, c)), false, true
	})
}

func Substitute(body string, resolve func(command string) (string, bool)) string {
	return splice(body, func(p Preload) (string, bool, bool) {
		markdown, ok := resolve(p.Command)
		return markdown, true, ok
	})
}

func splice(body string, replace func(Preload) (text string, markdown, ok bool)) string {
	preloads := Find([]byte(body))
	if len(preloads) == 0 {
		return body
	}
	var b strings.Builder
	last := 0
	for _, p := range preloads {
		text, markdown, ok := replace(p)
		if !ok {
			b.WriteString(body[last:p.Stop])
			last = p.Stop
			continue
		}
		if span, isSpan := inlineSpan(text); markdown && !p.Block && isSpan && !ownLine(body, p) {
			b.WriteString(body[last:p.Start] + span)
			last = p.Stop
			continue
		}
		if markdown {
			text = indent(p.Indent, text)
		}
		segment := body[last:p.Start]
		if !p.Block {
			segment = strings.TrimRight(segment, " \t")
		}
		b.WriteString(segment)
		last = p.Stop
		if !p.Block {
			if text != "" && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteString("\n")
			}
			for last < len(body) && (body[last] == ' ' || body[last] == '\t') {
				last++
			}
			if last < len(body) && body[last] == '\n' {
				last++
			} else if last < len(body) && text != "" {
				text += p.Indent
			}
		}
		b.WriteString(text)
		if text == "" && strings.HasSuffix(b.String(), "\n\n") && strings.HasPrefix(body[last:], "\n") {
			last++
		}
	}
	b.WriteString(body[last:])
	return b.String()
}

func ownLine(body string, p Preload) bool {
	before := strings.TrimRight(body[:p.Start], " \t")
	after := strings.TrimLeft(body[p.Stop:], " \t")
	return (before == "" || strings.HasSuffix(before, "\n")) && (after == "" || strings.HasPrefix(after, "\n"))
}

func inlineSpan(markdown string) (string, bool) {
	line := strings.TrimSpace(markdown)
	if line == "" || strings.Contains(line, "\n") || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "#") {
		return "", false
	}
	return strings.TrimPrefix(line, "- "), true
}

func (r *Runner) Block(ctx context.Context, command string, c Context) string {
	if markdown, ok := c.resolve(command); ok {
		return indent("", markdown)
	}
	return Show(command, r.Run(ctx, command, c))
}

func (c Context) resolve(command string) (string, bool) {
	if c.Resolve == nil {
		return "", false
	}
	return c.Resolve(command)
}

func indent(prefix, markdown string) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(markdown, "\n"), "\n") {
		if line != "" {
			b.WriteString(prefix)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func Show(command, output string) string { return fence("", command, output) }

func fence(indent, command, output string) string {
	ticks := 3
	for run := strings.Repeat("`", ticks); strings.Contains(command+output, run); run += "`" {
		ticks++
	}
	delimiter := indent + strings.Repeat("`", ticks)
	var b strings.Builder
	b.WriteString(delimiter + "text\n")
	for i, line := range strings.Split(command, "\n") {
		prompt := "$ "
		if i > 0 {
			prompt = "> "
		}
		b.WriteString(indent + prompt + line + "\n")
	}
	if output != "" {
		for line := range strings.SplitSeq(strings.TrimRight(output, "\n"), "\n") {
			b.WriteString(indent + line + "\n")
		}
	}
	b.WriteString(delimiter + "\n")
	return b.String()
}

func (r *Runner) Run(ctx context.Context, command string, c Context) string {
	if refusal := Check(command, c.Dir, r.Refuse); refusal != nil {
		return "henia: " + refusal.String()
	}
	command, refusal, err := r.consultPolicy(ctx, command, c)
	switch {
	case err != nil:
		return "henia: blocked: " + r.policyName() + " failed: " + err.Error()
	case refusal != nil:
		return "henia: " + refusal.String()
	}
	if refusal := Check(command, c.Dir, r.Refuse); refusal != nil {
		return "henia: " + refusal.String() + " (after " + r.policyName() + " rewrote it)"
	}
	shell, err := exec.LookPath("bash")
	if err != nil {
		return "henia: not run: bash not found"
	}
	argv, sandboxed := r.Sandbox(c.Dir, []string{shell, "-c", command})
	if !sandboxed {
		if !r.RunUnsandboxed {
			return `henia: not run: no write sandbox is available here; set [preload] unsandboxed = "run" in the user henia.toml to run preloads without one`
		}
		argv = []string{shell, "-c", command}
	}
	return r.execute(ctx, argv, c.Dir)
}

func (r *Runner) execute(ctx context.Context, argv []string, dir string) string {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	if self, err := os.Executable(); err == nil {
		cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(self)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	out := &limited{max: r.Output}
	cmd.Stdout, cmd.Stderr = out, out
	isolate(cmd)
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	text := out.buf.String()
	var notes []string
	if out.dropped > 0 {
		notes = append(notes, fmt.Sprintf("henia: output truncated at %d bytes", r.Output))
	}
	var exit *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		notes = append(notes, "henia: timed out after "+r.Timeout.String())
	case errors.As(err, &exit):
		notes = append(notes, fmt.Sprintf("henia: exit status %d", exit.ExitCode()))
	case err != nil:
		notes = append(notes, "henia: "+err.Error())
	}
	if len(notes) == 0 {
		return text
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + strings.Join(notes, "\n")
}

type limited struct {
	buf     bytes.Buffer
	max     int
	dropped int
}

func (l *limited) Write(p []byte) (int, error) {
	keep := min(len(p), max(l.max-l.buf.Len(), 0))
	l.buf.Write(p[:keep])
	l.dropped += len(p) - keep
	return len(p), nil
}

type policyResponse struct {
	Decision string `json:"decision"`
	Command  string `json:"command"`
	Rule     string `json:"rule"`
	Reason   string `json:"reason"`
}

func (r *Runner) policyName() string { return filepath.Base(r.Policy[0]) }

func (r *Runner) consultPolicy(ctx context.Context, command string, c Context) (string, *Refusal, error) {
	if len(r.Policy) == 0 {
		return command, nil, nil
	}
	input, err := json.Marshal(map[string]string{
		"command": command, "skill": c.Skill, "package": c.Package, "tier": c.Tier, "caller": c.Caller, "cwd": c.Dir,
	})
	if err != nil {
		return "", nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.PolicyTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Policy[0], r.Policy[1:]...)
	cmd.Dir = c.Dir
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	isolate(cmd)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", nil, fmt.Errorf("timed out after %s", r.PolicyTimeout)
	}
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var resp policyResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", nil, fmt.Errorf("unreadable response %q", strings.TrimSpace(string(out)))
	}
	switch resp.Decision {
	case "allow":
		if resp.Command != "" {
			return resp.Command, nil, nil
		}
		return command, nil, nil
	case "deny":
		rule := r.policyName()
		if resp.Rule != "" {
			rule += "/" + resp.Rule
		}
		return "", &Refusal{Rule: rule, Reason: resp.Reason}, nil
	}
	return "", nil, fmt.Errorf("unknown decision %q", resp.Decision)
}
