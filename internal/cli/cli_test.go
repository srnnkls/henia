package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpDescribesCompilerBoundary(t *testing.T) {
	out := &bytes.Buffer{}
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"build", "lint"} {
		if !strings.Contains(out.String(), word) {
			t.Fatalf("missing %s: %s", word, out)
		}
	}
	for _, command := range rootCmd.Commands() {
		switch command.Name() {
		case "add", "deploy", "sync", "update":
			t.Fatalf("deployment command still registered: %s", command.Name())
		}
	}
	if rootCmd.PersistentFlags().Lookup("data-dir") != nil {
		t.Fatal("compiler still exposes fetched-source cache")
	}
}
