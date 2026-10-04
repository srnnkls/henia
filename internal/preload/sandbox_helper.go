package preload

import (
	"fmt"
	"os"
)

const SandboxCommand = "__henia-preload-sandbox"

func RunSandboxHelper(args []string) bool {
	if len(args) < 2 || args[1] != SandboxCommand {
		return false
	}
	command := args[2:]
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	err := execSandboxed(command)
	fmt.Fprintf(os.Stderr, "henia: preload sandbox: %v\n", err)
	os.Exit(126)
	return true
}
