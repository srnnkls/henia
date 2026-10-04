package preload

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/landlock-lsm/go-landlock/landlock"
	llsyscall "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

var probe = sync.OnceValue(func() []string {
	if self, err := os.Executable(); err == nil {
		if version, err := llsyscall.LandlockGetABIVersion(); err == nil && version >= 1 {
			if exec.Command(self, SandboxCommand, "--", "/bin/sh", "-c", "true").Run() == nil {
				return []string{self, SandboxCommand, "--"}
			}
		}
	}
	if path, err := exec.LookPath("bwrap"); err == nil && exec.Command(path, bwrapArgs("/")...).Run() == nil {
		return []string{path, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--die-with-parent", "--"}
	}
	return nil
})

func bwrapArgs(dir string, command ...string) []string {
	if len(command) == 0 {
		command = []string{"true"}
	}
	return append([]string{"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--die-with-parent", "--chdir", dir, "--"}, command...)
}

func sandbox(_ string, command []string) ([]string, bool) {
	prefix := probe()
	if prefix == nil {
		return nil, false
	}
	return append(append([]string(nil), prefix...), command...), true
}

func execSandboxed(command []string) error {
	if len(command) == 0 {
		return errors.New("no command")
	}
	if err := landlock.V5.BestEffort().RestrictPaths(landlock.RODirs("/"), landlock.RWFiles("/dev/null")); err != nil {
		return err
	}
	path, err := exec.LookPath(command[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, command, os.Environ())
}
