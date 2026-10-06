package reference

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/lint/std"
	"github.com/srnnkls/henia/internal/markup"
)

const edgeCases = "# Use `$heading`\n\n" +
	"Plain `$skill`, `/command`, `@agent`, `!Read`, `#docs/guide.md`, `#anchor`, `$Upper`, `$a/b`.\n\n" +
	"Padded ` $x ` and `$x `, doubled ``$x``, adjacent `$x``$y`, split `$x\n$y`.\n\n" +
	"[link `$in-link`](u) and *emphasis `$in-em`* and :term[`$in-directive`]{k=v}.\n\n" +
	"| a | `$in-table` |\n|---|---|\n| `/cell` | x |\n\n" +
	"- item `@listed`\n  > quoted `$deep`\n\n" +
	"```\n`$fenced`\n```\n\n    `$indented`\n\n" +
	":::note\nInside `$block-directive`.\n:::\n\n" +
	"{{ if .x }}`$templated`{{ end }} and \\`$escaped\\`.\n"

func TestRecognizeAgreesWithParse(t *testing.T) {
	documents := map[string]string{"edge cases": edgeCases}
	_ = fs.WalkDir(std.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := std.FS.ReadFile(path)
			documents["std/"+path] = string(data)
		}
		return nil
	})
	roots := []string{"../../tests", "../../examples", "../../docs"}
	if corpus := os.Getenv("HENIA_BENCH_CORPUS"); corpus != "" {
		roots = append(roots, corpus)
	} else if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "projects", "tropos"))
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (slices.Contains([]string{".git", "node_modules", ".worktrees", "dist"}, d.Name()) || strings.HasSuffix(path, filepath.Join(".henia", "build"))) {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
				if data, err := os.ReadFile(path); err == nil {
					documents[path] = string(data)
				}
			}
			return nil
		})
	}
	total := 0
	for path, data := range documents {
		body := data
		if art, err := artifact.Parse([]byte(data)); err == nil {
			body = art.Body
		}
		want := Parse(body)
		tree, _ := markup.Tree([]byte(body))
		got, err := Recognize(tree)
		if err != nil {
			t.Fatal(err)
		}
		total += len(want)
		if missing, extra := difference(want, got), difference(got, want); len(missing)+len(extra) > 0 {
			t.Errorf("%s: Recognize disagrees with Parse\n  only Parse: %v\n  only Recognize: %v", path, missing, extra)
		}
	}
	t.Logf("%d documents, %d references", len(documents), total)
}

func difference(a, b []Reference) []string {
	var out []string
	for _, ref := range a {
		if !slices.Contains(b, ref) {
			out = append(out, fmt.Sprintf("%s %q %q @%d-%d", ref.Type, ref.Name, ref.Raw, ref.Start, ref.End))
		}
	}
	return out
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
