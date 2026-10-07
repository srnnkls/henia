package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadShadowsUserTemplates(t *testing.T) {
	project, user := t.TempDir(), t.TempDir()
	for path, content := range map[string]string{
		filepath.Join(project, ".henia", Dir, "head.md.tmpl"): "project",
		filepath.Join(user, Dir, "head.md.tmpl"):              "user",
		filepath.Join(user, Dir, "skill.md.tmpl"):             "layout",
		filepath.Join(user, Dir, "notes.md"):                  "ignored",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := Load(project, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded["head"] != "project" || loaded["skill"] != "layout" {
		t.Fatalf("loaded = %v", loaded)
	}
	files := Files(project, user)
	if files[0].Name != "head" || files[0].User || files[1].Name != "skill" || !files[1].User {
		t.Fatalf("files = %+v", files)
	}
}
