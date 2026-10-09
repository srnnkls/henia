package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	nestedInnermost = ":::a\nouter\n\n:::b\ninner\n:::\n\n:::\n"
	nestedCanonical = "::::a\nouter\n\n:::b\ninner\n:::\n\n::::\n"
)

func runFmt(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newFmtCommand()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFmtRewritesNestedFencesIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	fixture(t, path, nestedInnermost)
	if _, err := runFmt(t, path); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != nestedCanonical {
		t.Fatalf("first run:\n%s\nwant:\n%s", got, nestedCanonical)
	}
	if _, err := runFmt(t, path); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != nestedCanonical {
		t.Fatalf("second run changed the file:\n%s", got)
	}
}

func TestFmtCheckListsChangedFilesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "stale.md")
	canonical := filepath.Join(dir, "canonical.md")
	fixture(t, stale, nestedInnermost)
	fixture(t, canonical, nestedCanonical)
	out, err := runFmt(t, "--check", stale, canonical)
	if err == nil {
		t.Fatalf("--check succeeded on a non-canonical file: %s", out)
	}
	if !strings.Contains(out, "stale.md") {
		t.Errorf("--check output does not name the changed file: %q", out)
	}
	if strings.Contains(out, "canonical.md") {
		t.Errorf("--check output names an unchanged file: %q", out)
	}
	if got := read(t, stale); got != nestedInnermost {
		t.Errorf("--check rewrote the file:\n%s", got)
	}
	if out, err := runFmt(t, "--check", canonical); err != nil {
		t.Fatalf("--check failed on a canonical file: %v %s", err, out)
	}
}

func TestFmtLeavesCanonicalFilesUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	source := "---\nname: example\n---\n\n# Title\n\n::::outer{kind=x}\ntext  with  spacing\n\n:::inner\nbody\n:::\n\n::::\n\n:::leaf\n:::\n"
	fixture(t, path, source)
	if _, err := runFmt(t, path); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != source {
		t.Fatalf("canonical file changed:\n%s", got)
	}
}
