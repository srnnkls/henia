package build_test

import (
	"path/filepath"
	"testing"

	"github.com/srnnkls/henia/internal/benchdata"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
)

func BenchmarkRender(b *testing.B) {
	root := benchdata.Root(b)
	cfg, err := config.Load(filepath.Join(root, "henia.toml"))
	if err != nil {
		b.Fatal(err)
	}
	art := benchdata.LargestSkill(b)
	served := map[string]bool{"code": true, "git": true, "review": true}
	for _, name := range []string{"claude", "codex", "pi"} {
		harness := cfg.Harness[name]
		harness.ProjectRoot = root
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(art.Body)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := build.Render(art, name, harness, served, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBuildAll(b *testing.B) {
	root := benchdata.Root(b)
	cfg, err := config.Load(filepath.Join(root, "henia.toml"))
	if err != nil {
		b.Fatal(err)
	}
	for name, harness := range cfg.Harness {
		if len(harness.Artifacts) == 0 {
			harness.Artifacts = cfg.Artifacts
		}
		cfg.Harness[name] = harness
	}
	output := b.TempDir()
	b.ReportAllocs()
	for b.Loop() {
		result, err := build.RunClean(b.Context(), []string{root}, filepath.Join(output, "build"), cfg.Harness)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Errors) > 0 {
			b.Fatal(result.Errors)
		}
	}
}
