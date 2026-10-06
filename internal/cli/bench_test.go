package cli

import (
	"io"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/srnnkls/henia/internal/benchdata"
	"github.com/srnnkls/henia/internal/library"
)

func benchmarkShow(b *testing.B, cold bool) {
	root := benchdata.Root(b)
	cache := b.TempDir()
	b.Setenv("XDG_CACHE_HOME", cache)
	identity := func(_ library.Entry, text string) string { return text }
	disclose := func(library.Entry) bool { return true }
	run := func() {
		lib := library.Open(root, nil)
		show(io.Discard, lib, newRenderer("claude", lib), "scope", showMode{}, identity, disclose)
	}
	run()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if cold {
			b.StopTimer()
			i++
			b.Setenv("XDG_CACHE_HOME", filepath.Join(cache, strconv.Itoa(i)))
			b.StartTimer()
		}
		run()
	}
}

func BenchmarkShowCold(b *testing.B) { benchmarkShow(b, true) }

func BenchmarkShowWarm(b *testing.B) { benchmarkShow(b, false) }
