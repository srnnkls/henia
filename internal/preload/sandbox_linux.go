package preload

import (
	"os/exec"
	"sync"
)

var probe = sync.OnceValue(func() string {
	path, err := exec.LookPath("bwrap")
	if err != nil || exec.Command(path, bwrapArgs("/")...).Run() != nil {
		return ""
	}
	return path
})

func bwrapArgs(dir string, command ...string) []string {
	if len(command) == 0 {
		command = []string{"true"}
	}
	return append([]string{"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--die-with-parent", "--chdir", dir, "--"}, command...)
}

func sandbox(dir string, command []string) ([]string, bool) {
	path := probe()
	if path == "" {
		return nil, false
	}
	return append([]string{path}, bwrapArgs(dir, command...)...), true
}
