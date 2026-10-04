//go:build !darwin && !linux

package preload

func sandbox(string, []string) ([]string, bool) { return nil, false }

func execSandboxed([]string) error { return errSandboxHelper }
