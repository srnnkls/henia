package deps

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func release(t *testing.T, binary string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	writer, err := xz.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewWriter(writer)
	for name, content := range map[string]string{"phora-x/README.md": "readme", "phora-x/phora": binary} {
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}

func serve(t *testing.T, data []byte, checksum string) *int {
	t.Helper()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v"+phoraVersion+"/phora-test-target.tar.xz" {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(server.Close)
	savedReleases, savedArchives := phoraReleases, phoraArchives
	t.Cleanup(func() { phoraReleases, phoraArchives = savedReleases, savedArchives })
	phoraReleases = server.URL
	phoraArchives = map[string]struct{ target, sha256 string }{runtime.GOOS + "/" + runtime.GOARCH: {"test-target", checksum}}
	return &requests
}

func fakePhora(t *testing.T, version string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake phora is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "phora"), []byte("#!/bin/sh\necho 'phora "+version+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestPhoraFetchesPinnedRelease(t *testing.T) {
	t.Setenv("HENIA_PHORA", "")
	fakePhora(t, "0.3.9")
	data := release(t, "binary")
	sum := sha256.Sum256(data)
	requests := serve(t, data, hex.EncodeToString(sum[:]))
	tools := t.TempDir()
	var log bytes.Buffer
	path, err := Phora(t.Context(), tools, &log)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(tools, "phora", phoraVersion, "phora"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	if content, _ := os.ReadFile(path); string(content) != "binary" {
		t.Fatalf("binary = %q", content)
	}
	if info, _ := os.Stat(path); info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("mode = %v", info.Mode())
	}
	if !strings.Contains(log.String(), "Fetching phora "+phoraVersion) {
		t.Fatalf("log = %q", log.String())
	}
	if _, err := Phora(t.Context(), tools, nil); err != nil || *requests != 1 {
		t.Fatalf("cached lookup: %v, %d requests", err, *requests)
	}
}

func TestPhoraRejectsChecksumMismatch(t *testing.T) {
	t.Setenv("HENIA_PHORA", "")
	t.Setenv("PATH", "")
	serve(t, release(t, "tampered"), strings.Repeat("0", 64))
	tools := t.TempDir()
	if _, err := Phora(t.Context(), tools, nil); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tools, "phora", phoraVersion, "phora")); err == nil {
		t.Fatal("a tampered binary was installed")
	}
}

func TestPhoraPrefersCompliantPath(t *testing.T) {
	t.Setenv("HENIA_PHORA", "")
	fakePhora(t, "0.4.1")
	requests := serve(t, nil, "")
	path, err := Phora(t.Context(), t.TempDir(), nil)
	if err != nil || filepath.Base(path) != "phora" || *requests != 0 {
		t.Fatalf("path = %s, err = %v, %d requests", path, err, *requests)
	}
	t.Setenv("HENIA_PHORA", "/opt/phora")
	if path, _ := Phora(t.Context(), t.TempDir(), nil); path != "/opt/phora" {
		t.Fatalf("HENIA_PHORA ignored: %s", path)
	}
}

func TestPhoraUnsupportedPlatform(t *testing.T) {
	t.Setenv("HENIA_PHORA", "")
	t.Setenv("PATH", "")
	saved := phoraArchives
	t.Cleanup(func() { phoraArchives = saved })
	phoraArchives = nil
	if _, err := Phora(t.Context(), t.TempDir(), nil); err != ErrNoPhora {
		t.Fatalf("err = %v", err)
	}
}

func TestPinnedRelease(t *testing.T) {
	if len(phoraArchives) != 4 {
		t.Fatalf("phora %s checksums cover %d of 4 platforms: %v", phoraVersion, len(phoraArchives), phoraArchives)
	}
	if got := phoraArchives["darwin/arm64"]; got.target != "aarch64-apple-darwin" || len(got.sha256) != 64 {
		t.Fatalf("darwin/arm64 = %+v", got)
	}
}
