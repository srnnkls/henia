//go:build !unix

package preload

import "os/exec"

func isolate(*exec.Cmd) {}
