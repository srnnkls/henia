package main

import (
	"os"

	"github.com/srnnkls/henia/internal/cli"
)

// version is set at release-build time via -ldflags "-X main.version=...";
// a local `go build` keeps "dev".
var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		os.Exit(1)
	}
}
