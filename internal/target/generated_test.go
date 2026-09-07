package target

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
)

func TestGeneratedFilesCannotEscapeRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	a := &artifact.Artifact{Name: "example", Type: artifact.TypeSkill, Body: "# Example", Files: map[string][]byte{"escape/file.yaml": []byte("no")}}
	if err := New(root).Write(a); err == nil {
		t.Fatal("wrote through escaping symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "file.yaml")); !os.IsNotExist(err) {
		t.Fatal("wrote outside output root")
	}
	a.Files = map[string][]byte{"../escape": []byte("no")}
	if err := New(root).Write(a); err == nil {
		t.Fatal("accepted traversal")
	}
}

func TestGeneratedFileConflictsWithResource(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "agents/openai.yaml"), []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	a := &artifact.Artifact{Name: "example", Type: artifact.TypeSkill, SourcePath: source, IsDirectory: true, Resources: []string{"agents"}, Files: map[string][]byte{"skills/example/agents/openai.yaml": []byte("generated")}}
	if _, err := OutputPaths(New(t.TempDir()), a); err == nil {
		t.Fatal("accepted sidecar/resource collision")
	}
}
