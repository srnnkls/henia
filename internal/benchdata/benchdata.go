package benchdata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
)

func Root(tb testing.TB) string {
	tb.Helper()
	root := os.Getenv("HENIA_BENCH_CORPUS")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "projects", "tropos")
	}
	if _, err := os.Stat(filepath.Join(root, "skills")); err != nil {
		tb.Skipf("benchmark corpus unavailable: %v", err)
	}
	return root
}

func LargestSkill(tb testing.TB) *artifact.Artifact {
	tb.Helper()
	path := filepath.Join(Root(tb), "skills", "scope", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	art, err := artifact.Parse(data)
	if err != nil {
		tb.Fatal(err)
	}
	art.Name, art.Type, art.SourcePath, art.IsDirectory = "scope", artifact.TypeSkill, filepath.Dir(path), true
	return art
}
