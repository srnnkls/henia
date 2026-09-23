package build

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/srnnkls/henia"
)

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func TestSupportFileLinkedOutsideSourceBuilds(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	outside := filepath.Join(root, "checkout/instructions/AGENTS.md")
	put(t, filepath.Join(source, "skills/code/SKILL.md"), "Skill.\n")
	put(t, outside, "Instructions.\n")
	link(t, outside, filepath.Join(source, "instructions/AGENTS.md"))
	output := filepath.Join(root, "output")
	harnesses := map[string]henia.Harness{"claude": {Files: map[string]henia.File{"CLAUDE.md": {Source: "instructions/AGENTS.md"}}}}
	result, err := RunClean(t.Context(), []string{source}, output, harnesses)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	path := filepath.Join(output, "claude/CLAUDE.md")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("support file is not a regular file: %v, %v", info, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "Instructions.\n" {
		t.Fatalf("support file: %q", data)
	}
}

func TestSupportFileSourceCannotLeaveSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	put(t, filepath.Join(source, "skills/code/SKILL.md"), "Skill.\n")
	put(t, filepath.Join(root, "outside.md"), "Outside.\n")
	for _, path := range []string{"../outside.md", "instructions/../../outside.md", filepath.Join(root, "outside.md")} {
		harnesses := map[string]henia.Harness{"claude": {Files: map[string]henia.File{"CLAUDE.md": {Source: path}}}}
		result, err := RunClean(t.Context(), []string{source}, filepath.Join(root, "output"), harnesses)
		if err == nil && len(result.Errors) == 0 {
			t.Fatalf("source %s outside the source directory accepted", path)
		}
	}
}

// mirror recreates tree at mirror with real directories and symlinked leaves,
// the layout phora prepares in link mode; linkedDir is linked as a whole.
func mirror(t *testing.T, tree, mirror, linkedDir string) {
	t.Helper()
	err := filepath.WalkDir(tree, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(tree, path)
		if err != nil {
			return err
		}
		if relative == linkedDir {
			link(t, path, filepath.Join(mirror, relative))
			return fs.SkipDir
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(mirror, relative), 0755)
		}
		link(t, path, filepath.Join(mirror, relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type outputFile struct {
	mode os.FileMode
	data []byte
}

func snapshot(t *testing.T, root string) map[string]outputFile {
	t.Helper()
	files := map[string]outputFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("output %s is not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		files[relative] = outputFile{info.Mode(), data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestLinkedSourceTreeBuildsLikeRealTree(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	put(t, filepath.Join(real, "skills/code/SKILL.md"), "---\nname: code\ndescription: Code carefully.\n---\n\nSkill.\n")
	put(t, filepath.Join(real, "skills/code/reference/guide.md"), "Guide.\n")
	put(t, filepath.Join(real, "skills/code/assets/template.txt"), "Template.\n")
	put(t, filepath.Join(real, "skills/code/run.sh"), "#!/bin/sh\necho run\n")
	if err := os.Chmod(filepath.Join(real, "skills/code/run.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(real, "agents/tester.md"), "---\nname: tester\ndescription: Prove failures.\n---\n\nRole contract.\n")
	put(t, filepath.Join(real, ".henia/harnesses/claude-agent/transform.toml"), "fields = [\"name\", \"description\"]\n")
	put(t, filepath.Join(real, "instructions/AGENTS.md"), "Read [code](../skills/code/SKILL.md).\n")
	linked := filepath.Join(root, "linked")
	mirror(t, real, linked, filepath.Join("skills", "code", "assets"))

	harnesses := func(source string) map[string]henia.Harness {
		return map[string]henia.Harness{
			"claude": {
				ProjectRoot:      source,
				ArtifactMappings: map[string]henia.ArtifactMapping{"agents": {Profile: "claude-agent", Structure: "flat"}},
				Files:            map[string]henia.File{"CLAUDE.md": {Source: "instructions/AGENTS.md", Replace: map[string]string{"](../": "]("}}},
			},
		}
	}
	build := func(source string) map[string]outputFile {
		t.Helper()
		output := filepath.Join(root, "output-"+filepath.Base(source))
		result, err := RunClean(t.Context(), []string{source}, output, harnesses(source))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) != 0 {
			t.Fatal(result.Errors)
		}
		return snapshot(t, output)
	}
	want, got := build(real), build(linked)
	if len(want) != 6 {
		t.Fatalf("real build files: %d", len(want))
	}
	if want["claude/skills/code/run.sh"].mode.Perm() != 0755 {
		t.Fatalf("script mode: %v", want["claude/skills/code/run.sh"].mode)
	}
	for path, file := range want {
		if got[path].mode != file.mode || !bytes.Equal(got[path].data, file.data) {
			t.Errorf("%s: got %v %q, want %v %q", path, got[path].mode, got[path].data, file.mode, file.data)
		}
	}
	if len(got) != len(want) {
		t.Errorf("linked build files: %d, want %d", len(got), len(want))
	}
}
