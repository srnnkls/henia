package deps

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
)

//go:embed phora.version
var pinnedVersion string

//go:embed phora.sum
var pinnedSums string

var phoraVersion = strings.TrimSpace(pinnedVersion)

var phoraMinimum = [3]int{0, 4, 1}

var phoraReleases = "https://github.com/srnnkls/phora/releases/download"

var phoraArchives = archives(map[string]string{
	"darwin/arm64": "aarch64-apple-darwin",
	"darwin/amd64": "x86_64-apple-darwin",
	"linux/arm64":  "aarch64-unknown-linux-musl",
	"linux/amd64":  "x86_64-unknown-linux-musl",
}, pinnedSums)

func archives(targets map[string]string, sums string) map[string]struct{ target, sha256 string } {
	checksums := map[string]string{}
	for _, line := range strings.Split(sums, "\n") {
		if sum, file, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			checksums[strings.TrimPrefix(strings.TrimSpace(file), "*")] = sum
		}
	}
	found := map[string]struct{ target, sha256 string }{}
	for platform, target := range targets {
		if sum, ok := checksums["phora-"+target+".tar.xz"]; ok {
			found[platform] = struct{ target, sha256 string }{target, sum}
		}
	}
	return found
}

func Phora(ctx context.Context, tools string, log io.Writer) (string, error) {
	if path := os.Getenv("HENIA_PHORA"); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath("phora"); err == nil && compliant(ctx, path) {
		return path, nil
	}
	path := filepath.Join(tools, "phora", phoraVersion, "phora")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	archive, ok := phoraArchives[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok || tools == "" {
		return "", ErrNoPhora
	}
	if log != nil {
		fmt.Fprintf(log, "Fetching phora %s\n", phoraVersion)
	}
	if err := fetchPhora(ctx, archive.target, archive.sha256, path); err != nil {
		return "", fmt.Errorf("fetch phora %s: %w", phoraVersion, err)
	}
	return path, nil
}

func compliant(ctx context.Context, path string) bool {
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return false
	}
	var version [3]int
	if _, err := fmt.Sscanf(string(out), "phora %d.%d.%d", &version[0], &version[1], &version[2]); err != nil {
		return false
	}
	for i := range version {
		if version[i] != phoraMinimum[i] {
			return version[i] > phoraMinimum[i]
		}
	}
	return true
}

func fetchPhora(ctx context.Context, target, checksum, path string) error {
	url := fmt.Sprintf("%s/v%s/phora-%s.tar.xz", phoraReleases, phoraVersion, target)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 256<<20))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != checksum {
		return fmt.Errorf("%s: sha256 %s, want %s", url, got, checksum)
	}
	decompressed, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	archive := tar.NewReader(decompressed)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s holds no phora binary", url)
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "phora" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(path), ".phora-*")
		if err != nil {
			return err
		}
		_, err = io.Copy(temporary, archive)
		if closeErr := temporary.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Chmod(temporary.Name(), 0o755)
		}
		if err == nil {
			err = os.Rename(temporary.Name(), path)
		}
		if err != nil {
			os.Remove(temporary.Name())
		}
		return err
	}
}
