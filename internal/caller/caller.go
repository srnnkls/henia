// Package caller identifies the agent harness a command runs under.
package caller

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var agents = map[string]string{"claude": "claude", "claude-code": "claude", "codex": "codex", "pi": "pi", "omp": "omp", "opencode": "opencode"}

func Harness(flag string, getenv func(string) string, ancestors []string) string {
	if flag != "" {
		return neutral(flag)
	}
	if harness := getenv("HENIA_HARNESS"); harness != "" {
		return neutral(harness)
	}
	for _, command := range ancestors {
		if harness, ok := agent(command); ok {
			return harness
		}
	}
	if name, _, _ := strings.Cut(getenv("AI_AGENT"), "_"); name != "" {
		if harness, ok := agents[name]; ok {
			return harness
		}
		return name
	}
	switch {
	case getenv("CODEX_THREAD_ID") != "":
		return "codex"
	case getenv("CLAUDECODE") != "":
		return "claude"
	}
	return ""
}

func neutral(harness string) string {
	if harness == "none" {
		return ""
	}
	return harness
}

func agent(command string) (string, bool) {
	fields := strings.Fields(command)
	for _, field := range fields[:min(2, len(fields))] {
		name := strings.TrimSuffix(filepath.Base(field), filepath.Ext(field))
		if harness, ok := agents[name]; ok {
			return harness, true
		}
	}
	return "", false
}

func Ancestors() []string {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,args=").Output()
	if err != nil {
		return nil
	}
	parents := make(map[int]int)
	commands := make(map[int]string)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		parents[pid] = ppid
		commands[pid] = strings.Join(fields[2:], " ")
	}
	var ancestors []string
	for pid, depth := parents[os.Getpid()], 0; pid > 1 && depth < 64; pid, depth = parents[pid], depth+1 {
		ancestors = append(ancestors, commands[pid])
	}
	return ancestors
}
