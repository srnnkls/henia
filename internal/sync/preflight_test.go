package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srnnkls/henia"
)

func TestOutputCollision(t *testing.T) {
	root := t.TempDir()
	var sources []FetchedSource
	for _, name := range []string{"one", "two"} {
		source := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(source, "skills"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "skills/same.md"), []byte("# Skill\n"), 0644); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, FetchedSource{Name: name, LocalPath: source})
	}
	output := filepath.Join(root, "out")
	result, err := NewSyncer(nil, map[string]henia.Harness{"claude": {Path: output}}).Deploy(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error(), "collision") || result.Synced != 0 {
		t.Fatalf("expected collision: %+v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("collision wrote output")
	}
}

func TestOutputInsideSource(t *testing.T) {
	root := t.TempDir()
	skill := filepath.Join(root, "skills/example")
	if err := os.MkdirAll(skill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := NewSyncer(nil, map[string]henia.Harness{"claude": {Path: filepath.Join(skill, "output")}}).Deploy([]FetchedSource{{LocalPath: root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error(), "inside canonical") {
		t.Fatalf("expected source-overlap error: %+v", result)
	}
}
