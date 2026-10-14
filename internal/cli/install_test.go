package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/srnnkls/henia/internal/build"
	"github.com/srnnkls/henia/internal/config"
	"github.com/srnnkls/henia/internal/library"
)

func TestApplyOwnsOnlyWhatItInstalled(t *testing.T) {
	staging, target := t.TempDir(), t.TempDir()
	manifest := filepath.Join(t.TempDir(), "claude.json")
	fixture(t, filepath.Join(staging, "skills", "a", "SKILL.md"), "a\n")
	fixture(t, filepath.Join(staging, "skills", "b", "SKILL.md"), "b\n")
	fixture(t, filepath.Join(target, "skills", "mine", "SKILL.md"), "mine\n")
	files := map[string]string{
		filepath.Join("skills", "a", "SKILL.md"): filepath.Join(staging, "skills", "a", "SKILL.md"),
		filepath.Join("skills", "b", "SKILL.md"): filepath.Join(staging, "skills", "b", "SKILL.md"),
	}
	if written, removed, err := apply(target, files, manifest, false, nil); err != nil || written != 2 || removed != 0 {
		t.Fatalf("first install: %d %d %v", written, removed, err)
	}
	if _, _, err := apply(target, files, manifest, false, nil); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	fixture(t, filepath.Join(target, "skills", "a", "SKILL.md"), "edited\n")
	if _, _, err := apply(target, files, manifest, false, nil); err == nil || !strings.Contains(err.Error(), "skills/a/SKILL.md") {
		t.Fatalf("edited file: %v", err)
	}
	if _, _, err := apply(target, files, manifest, true, nil); err != nil {
		t.Fatalf("force: %v", err)
	}
	link := filepath.Join(target, "skills", "b", "SKILL.md")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(target, "skills", "mine", "SKILL.md"), link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := apply(target, files, manifest, false, nil); err == nil {
		t.Fatal("wrote through a symlink it does not own")
	}
	if _, _, err := apply(target, files, manifest, true, nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target, "skills", "mine", "SKILL.md")); string(data) != "mine\n" {
		t.Fatalf("symlink target overwritten: %q", data)
	}
	delete(files, filepath.Join("skills", "b", "SKILL.md"))
	if _, removed, err := apply(target, files, manifest, false, nil); err != nil || removed != 1 {
		t.Fatalf("prune: %d %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "b")); !os.IsNotExist(err) {
		t.Fatalf("pruned skill directory remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "mine", "SKILL.md")); err != nil {
		t.Fatalf("foreign skill removed: %v", err)
	}
}

func TestInstallTargets(t *testing.T) {
	home, _ := os.UserHomeDir()
	off := false
	env := map[string]string{"CODEX_HOME": "/srv/codex"}
	present := map[string]bool{filepath.Join(home, ".claude"): true, "/srv/codex": true}
	targets, err := installTargets(
		map[string]config.InstallTarget{"omp": {Enabled: &off}, "custom": {Path: "~/agents/custom"}},
		map[string]bool{"claude": true, "codex": true, "pi": true, "omp": true, "gemini": true},
		func(key string) string { return env[key] },
		func(path string) bool { return present[path] },
	)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, target := range targets {
		got = append(got, fmt.Sprintf("%s=%s %v", target.harness, target.path, target.explicit))
	}
	want := []string{"claude=" + filepath.Join(home, ".claude") + " false", "codex=/srv/codex false", "custom=" + filepath.Join(home, "agents/custom") + " true"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("targets:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, err := installTargets(map[string]config.InstallTarget{"gemini": {}}, nil, func(string) string { return "" }, func(string) bool { return true }); err == nil {
		t.Fatal("a listed harness without a known home needs a path")
	}
}

func TestStaleHead(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	skill := filepath.Join(t.TempDir(), "SKILL.md")
	fixture(t, skill, "# Route\n")
	entry := library.Entry{Name: "route", Path: skill}
	if stale("claude", entry) {
		t.Fatal("stale without an install record")
	}
	manifest := library.InstallManifest("claude")
	data, _ := json.Marshal(installRecord{Revisions: map[string]string{"route": build.Digest([]byte("# Route\n"))}})
	fixture(t, manifest, string(data))
	if stale("claude", entry) || stale("", entry) {
		t.Fatal("fresh install reported stale")
	}
	fixture(t, skill, "# Route\n\nEdited.\n")
	if !stale("claude", entry) {
		t.Fatal("edited library skill not reported stale")
	}
}

func projectFixture(t *testing.T, install string) string {
	t.Helper()
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dep := filepath.Join(library.PackagesDir(library.ProjectKey(project)), "tropos")
	fixture(t, filepath.Join(dep, "henia.toml"), "[harness.claude]\nartifacts = ['skills', 'agents']\n[harness.claude.files]\n'CLAUDE.md' = { source = 'AGENTS.md' }\n")
	fixture(t, filepath.Join(dep, "AGENTS.md"), "# Instructions\n")
	fixture(t, filepath.Join(dep, "skills", "library", "SKILL.md"), "---\nname: library\ndescription: Library skill.\n---\n\n# Library\n")
	fixture(t, filepath.Join(dep, "agents", "helper.md"), "---\nname: helper\ndescription: Helper agent.\n---\n\nHelp.\n")
	fixture(t, filepath.Join(project, "henia.toml"), "[harness.codex]\n"+install)
	fixture(t, filepath.Join(project, ".henia", "skills", "own", "SKILL.md"), "---\nname: own\ndescription: Own skill.\n---\n\n# Own\n")
	return project
}

func installCommand(t *testing.T, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	previous := configPath
	t.Cleanup(func() { configPath = previous })
	out := &bytes.Buffer{}
	root := &cobra.Command{Use: "henia", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&configPath, "config", "henia.toml", "Config file path")
	root.AddCommand(newInstallCommand(), newSyncCommand())
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(args)
	return root, out
}

func TestProjectInstall(t *testing.T) {
	project := projectFixture(t, "[install.codex]\npath = '.henia/codex'\n")
	home := filepath.Join(project, ".claude")
	fixture(t, filepath.Join(home, "settings.json"), "{}\n")
	fixture(t, filepath.Join(project, "elsewhere", "projects", "keep"), "keep\n")
	if err := os.Symlink(filepath.Join(project, "elsewhere", "projects"), filepath.Join(home, "projects")); err != nil {
		t.Fatal(err)
	}
	fixture(t, filepath.Join(project, ".henia", "codex", "auth.json"), "{}\n")
	run := func() {
		t.Helper()
		cmd, out := installCommand(t, "install")
		if err := cmd.Execute(); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
	}
	run()
	for _, path := range []string{
		".claude/CLAUDE.md", ".claude/agents/helper/AGENT.md", ".claude/skills/own/SKILL.md", ".claude/skills/henia/SKILL.md", ".claude/settings.json",
		".henia/codex/skills/own/SKILL.md", ".henia/codex/auth.json",
		".henia/state/install/claude.json", ".henia/state/install/codex.json",
	} {
		if _, err := os.Stat(filepath.Join(project, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".codex")); !os.IsNotExist(err) {
		t.Errorf("codex installed into its default directory despite its path: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(home, "projects")); err != nil || target != filepath.Join(project, "elsewhere", "projects") {
		t.Errorf("symlink changed: %q %v", target, err)
	}
	run()
	if err := os.RemoveAll(filepath.Join(project, ".henia", "skills", "own")); err != nil {
		t.Fatal(err)
	}
	run()
	if _, err := os.Stat(filepath.Join(home, "skills", "own")); !os.IsNotExist(err) {
		t.Errorf("stale skill kept: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(home, "settings.json")); string(data) != "{}\n" {
		t.Errorf("foreign file changed: %q", data)
	}
	fixture(t, filepath.Join(project, "henia.toml"), "[harness.codex]\n[install.codex]\npath = '.henia/codex'\n[install.claude]\nenabled = false\n")
	run()
	if _, err := os.Stat(filepath.Join(home, "skills")); !os.IsNotExist(err) {
		t.Errorf("disabled harness kept its files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "settings.json")); err != nil {
		t.Errorf("disabling removed a foreign file: %v", err)
	}
}

func TestProjectInstallNeedsProject(t *testing.T) {
	t.Chdir(t.TempDir())
	cmd, out := installCommand(t, "install")
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--global") {
		t.Fatalf("install outside a project: %v\n%s", err, out)
	}
}

func TestProjectSyncInstalls(t *testing.T) {
	project := projectFixture(t, "")
	phora := filepath.Join(t.TempDir(), "phora")
	fixture(t, phora, "#!/bin/sh\n")
	if err := os.Chmod(phora, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HENIA_PHORA", phora)
	cmd, out := installCommand(t, "sync", "--no-install")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("sync --no-install installed: %v", err)
	}
	cmd, out = installCommand(t, "sync")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, path := range []string{".claude/skills/own/SKILL.md", ".codex/skills/library/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(project, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

func TestProjectTargets(t *testing.T) {
	off := false
	targets, err := projectTargets("/p",
		map[string]config.InstallTarget{"codex": {Path: ".henia/codex"}, "omp": {Enabled: &off}, "abs": {Path: "/srv/abs"}},
		map[string]bool{"claude": true, "codex": true, "omp": true, "chatgpt": true, "abs": true, "github": true},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, target := range targets {
		got = append(got, fmt.Sprintf("%s=%s %v", target.harness, target.path, target.explicit))
	}
	want := []string{"abs=/srv/abs true", "claude=/p/.claude false", "codex=/p/.henia/codex true", "github=/p/.github false"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("targets:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, err := projectTargets("/p", map[string]config.InstallTarget{"pi": {}}, map[string]bool{"claude": true}, nil); err == nil {
		t.Fatal("a listed harness the build does not produce")
	}
	if _, err := projectTargets("/p", map[string]config.InstallTarget{"custom": {}}, map[string]bool{"custom": true}, nil); err == nil {
		t.Fatal("a listed harness without a project directory needs a path")
	}
}

func TestApplyRefusesSymlinkedDirectories(t *testing.T) {
	staging, target, outside := t.TempDir(), t.TempDir(), t.TempDir()
	fixture(t, filepath.Join(staging, "a.md"), "a\n")
	if err := os.Symlink(outside, filepath.Join(target, "skills")); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{filepath.Join("skills", "a", "SKILL.md"): filepath.Join(staging, "a.md")}
	if _, _, err := apply(target, files, filepath.Join(t.TempDir(), "m.json"), true, nil); err == nil {
		t.Fatal("wrote through a symlinked directory")
	}
	if entries, _ := os.ReadDir(outside); len(entries) > 0 {
		t.Fatalf("wrote into the symlink target: %v", entries)
	}
}

func TestGlobalInstallKeepsHarnessStrictnessToItsOwner(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	home := filepath.Join(t.TempDir(), "claude")
	global := library.PackagesDir("global")
	fixture(t, filepath.Join(global, "tropos", "henia.toml"), "[harness.claude]\nstrict = true\n")
	fixture(t, filepath.Join(global, "tropos", "skills", "pinned", "SKILL.md"), "---\nname: pinned\ndescription: Pinned skill.\n---\n\n# Pinned\n")
	fixture(t, filepath.Join(global, "imp", "skills", "ext", "SKILL.md"), "---\nname: ext\ndescription: External skill.\nversion: 1.0.0\n---\n\n# Ext\n")
	user := filepath.Join(config, "henia", "henia.toml")
	install := func() error {
		t.Helper()
		cmd, out := installCommand(t, "install", "--global", "--harness", "claude")
		err := cmd.Execute()
		if err != nil {
			t.Logf("%s", out)
		}
		return err
	}
	fixture(t, user, "install_henia_skill = false\n[install.claude]\npath = '"+home+"'\n")
	if err := install(); err != nil {
		t.Fatalf("another package's unsupported frontmatter failed the owner's strict harness: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "ext", "SKILL.md")); err != nil {
		t.Errorf("ext not installed: %v", err)
	}
	fixture(t, user, "install_henia_skill = false\n[install.claude]\npath = '"+home+"'\n[harness.claude]\nstrict = true\n")
	if err := install(); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("unsupported frontmatter passed the user's strict table: %v", err)
	}
}

func TestInstallLeavesConfigOut(t *testing.T) {
	for _, c := range []struct {
		in, want []string
		installs bool
	}{
		{nil, nil, true},
		{[]string{"skills", "config"}, []string{"skills"}, true},
		{[]string{"config"}, []string{}, false},
	} {
		got, installs := installable(c.in)
		if !slices.Equal(got, c.want) || installs != c.installs {
			t.Errorf("%v: got %v %v", c.in, got, installs)
		}
	}
}
