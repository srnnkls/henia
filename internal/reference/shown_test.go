package reference

import (
	"fmt"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func TestAddresses(t *testing.T) {
	body := "Run `henia show code#tests`, `henia show tropos:git.reference.worktree`, `henia show x/scripts/run.sh --head` and ``henia show doubled``.\n\n" +
		"Quoted `` `henia show quoted` `` and fenced:\n\n```bash\n`henia show fenced`\nhenia show bare\n```\n\n```bash\nhenia show block-first-line\n```\n"
	tree, _ := markup.Tree([]byte(body))
	refs, err := Addresses(tree)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range refs {
		if body[r.Start] != '`' || body[r.Start+1:r.End] != r.Raw {
			t.Errorf("%q spans %q", r.Raw, body[r.Start:r.End])
		}
		got = append(got, fmt.Sprintf("%s|%s|%s|%s", r.Name, r.Module, r.Resource, r.Anchor))
	}
	want := "code|||tests tropos:git|reference.worktree|| x||scripts/run.sh| doubled|||"
	if strings.Join(got, " ") != want {
		t.Errorf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}
