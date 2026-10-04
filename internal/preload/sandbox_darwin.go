package preload

import (
	"os/exec"
	"sync"
)

const seatbelt = `(version 1)
(allow default)
(deny file-write*)
(allow file-write* (literal "/dev/null"))`

var probe = sync.OnceValue(func() string {
	path, err := exec.LookPath("sandbox-exec")
	if err != nil || exec.Command(path, "-p", seatbelt, "/usr/bin/true").Run() != nil {
		return ""
	}
	return path
})

func sandbox(_ string, command []string) ([]string, bool) {
	path := probe()
	if path == "" {
		return nil, false
	}
	return append([]string{path, "-p", seatbelt}, command...), true
}

func execSandboxed([]string) error { return errSandboxHelper }
