package reference

import (
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestRecognize_ReturnsEmptyForNoReferences(t *testing.T) {
	refs := recognize("plain text with no references")
	if len(refs) != 0 {
		t.Errorf("recognize() returned %d refs, want 0", len(refs))
	}
}

func TestRecognize_SkillReference(t *testing.T) {
	refs := recognize("Use `$my-skill` for this")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeSkill {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeSkill)
	}
	if refs[0].Name != "my-skill" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "my-skill")
	}
	if refs[0].Raw != "$my-skill" {
		t.Errorf("refs[0].Raw = %q, want %q", refs[0].Raw, "$my-skill")
	}
}

func TestRecognize_CommandReference(t *testing.T) {
	refs := recognize("Run `/deploy` now")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeCommand {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeCommand)
	}
	if refs[0].Name != "deploy" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "deploy")
	}
	if refs[0].Raw != "/deploy" {
		t.Errorf("refs[0].Raw = %q, want %q", refs[0].Raw, "/deploy")
	}
}

func TestRecognize_AgentReference(t *testing.T) {
	refs := recognize("Ask `@reviewer` for feedback")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeAgent {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeAgent)
	}
	if refs[0].Name != "reviewer" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "reviewer")
	}
	if refs[0].Raw != "@reviewer" {
		t.Errorf("refs[0].Raw = %q, want %q", refs[0].Raw, "@reviewer")
	}
}

func TestRecognize_FileReference(t *testing.T) {
	refs := recognize("See `#config.yaml` for details")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeFile {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeFile)
	}
	if refs[0].Name != "config.yaml" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "config.yaml")
	}
	if refs[0].Raw != "#config.yaml" {
		t.Errorf("refs[0].Raw = %q, want %q", refs[0].Raw, "#config.yaml")
	}
}

func TestRecognize_ToolReference(t *testing.T) {
	refs := recognize("Use `!grep` to search")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeTool {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeTool)
	}
	if refs[0].Name != "grep" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "grep")
	}
	if refs[0].Raw != "!grep" {
		t.Errorf("refs[0].Raw = %q, want %q", refs[0].Raw, "!grep")
	}
}

func TestRecognize_IgnoresReferencesOutsideBackticks(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"skill outside", "Use $my-skill directly"},
		{"command outside", "Run /deploy now"},
		{"agent outside", "Ask @reviewer for help"},
		{"file outside", "See #config.yaml"},
		{"tool outside", "Use !grep to search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := recognize(tt.input)
			if len(refs) != 0 {
				t.Errorf("recognize(%q) returned %d refs, want 0 (refs outside backticks should be ignored)", tt.input, len(refs))
			}
		})
	}
}

func TestRecognize_IgnoresVariablesAndPaths(t *testing.T) {
	for _, input := range []string{"`$ARGUMENTS`", "`$HOME`", "`/etc/nixos/idle-suspend.nix`", "`/usr/local/bin/wake-nix`", "`@Team/member`"} {
		if refs := recognize(input); len(refs) != 0 {
			t.Errorf("recognize(%q) = %+v, want no references", input, refs)
		}
	}
}

func TestRecognize_MultipleReferences(t *testing.T) {
	refs := recognize("Use `$skill-a` and `$skill-b` together with `/command`")

	if len(refs) != 3 {
		t.Fatalf("recognize() returned %d refs, want 3", len(refs))
	}

	if refs[0].Name != "skill-a" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "skill-a")
	}
	if refs[1].Name != "skill-b" {
		t.Errorf("refs[1].Name = %q, want %q", refs[1].Name, "skill-b")
	}
	if refs[2].Name != "command" {
		t.Errorf("refs[2].Name = %q, want %q", refs[2].Name, "command")
	}
}

func TestRecognize_MixedContent(t *testing.T) {
	input := "Start with `$setup`, outside $ignore this, then `/run` and @skip-this too"
	refs := recognize(input)

	if len(refs) != 2 {
		t.Fatalf("recognize() returned %d refs, want 2", len(refs))
	}
	if refs[0].Name != "setup" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "setup")
	}
	if refs[1].Name != "run" {
		t.Errorf("refs[1].Name = %q, want %q", refs[1].Name, "run")
	}
}

func TestRecognize_ReferenceWithDashes(t *testing.T) {
	refs := recognize("`$my-complex-skill-name`")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Name != "my-complex-skill-name" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "my-complex-skill-name")
	}
}

func TestRecognize_ReferenceWithUnderscores(t *testing.T) {
	refs := recognize("`$my_skill_name`")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Name != "my_skill_name" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "my_skill_name")
	}
}

func TestRecognize_ReferenceWithNumbers(t *testing.T) {
	refs := recognize("`$skill123`")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Name != "skill123" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "skill123")
	}
}

func TestRecognize_MalformedReference_EmptySigil(t *testing.T) {
	refs := recognize("Use `$` here")

	if len(refs) != 0 {
		t.Errorf("recognize() returned %d refs for malformed '$', want 0", len(refs))
	}
}

func TestRecognize_MalformedReference_SigilWithSpace(t *testing.T) {
	refs := recognize("Use `$ skill` here")

	if len(refs) != 0 {
		t.Errorf("recognize() returned %d refs for '$ skill', want 0", len(refs))
	}
}

func TestRecognize_BacktickWithNonReference(t *testing.T) {
	refs := recognize("Regular `code block` here")

	if len(refs) != 0 {
		t.Errorf("recognize() returned %d refs for non-reference backtick content, want 0", len(refs))
	}
}

func TestRecognize_AllSigilTypes(t *testing.T) {
	input := "`$skill` `/command` `@agent` `#file.md` `!tool`"
	refs := recognize(input)

	if len(refs) != 5 {
		t.Fatalf("recognize() returned %d refs, want 5", len(refs))
	}

	types := map[Type]bool{
		TypeSkill:   false,
		TypeCommand: false,
		TypeAgent:   false,
		TypeFile:    false,
		TypeTool:    false,
	}

	for _, ref := range refs {
		types[ref.Type] = true
	}

	for typ, found := range types {
		if !found {
			t.Errorf("Type %v not found in parsed references", typ)
		}
	}
}

func TestRecognize_NestedBackticks(t *testing.T) {
	refs := recognize("Use `$skill` in ``code`` blocks")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Name != "skill" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "skill")
	}
}

func TestRecognize_FileReferenceWithPath(t *testing.T) {
	refs := recognize("See `#internal/config/config.go` for details")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeFile {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeFile)
	}
	if refs[0].Name != "internal/config/config.go" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "internal/config/config.go")
	}
}

func TestRecognize_CommandWithSubcommand(t *testing.T) {
	refs := recognize("Run `/config.edit` to modify")

	if len(refs) != 1 {
		t.Fatalf("recognize() returned %d refs, want 1", len(refs))
	}
	if refs[0].Type != TypeCommand {
		t.Errorf("refs[0].Type = %v, want %v", refs[0].Type, TypeCommand)
	}
	if refs[0].Name != "config.edit" {
		t.Errorf("refs[0].Name = %q, want %q", refs[0].Name, "config.edit")
	}
}

func TestReferenceType_String(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{TypeSkill, "skill"},
		{TypeCommand, "command"},
		{TypeAgent, "agent"},
		{TypeFile, "file"},
		{TypeTool, "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("Type.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecognize_HashNeedsAPath(t *testing.T) {
	refs := recognize("Issue `#172`, PR `#N`, file `#notes.md`, dir `#reference/guide`")
	var names []string
	for _, ref := range refs {
		names = append(names, string(ref.Type)+":"+ref.Name)
	}
	if got := strings.Join(names, " "); got != "file:notes.md file:reference/guide" {
		t.Fatalf("refs = %s", got)
	}
}

func recognize(body string) []Reference {
	tree, _ := markup.Tree([]byte(body))
	refs, _ := Recognize(tree)
	return refs
}

func TestRecognizeLeavesHTMLBlocksRaw(t *testing.T) {
	body := "<details>\nUse `$inside`.\n</details>\n\nUse `$outside`.\n"
	tree, _ := markup.Tree([]byte(body))
	refs, err := Recognize(tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Name != "outside" {
		t.Errorf("refs = %+v, want only $outside", refs)
	}
}
