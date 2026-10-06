package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/srnnkls/henia/internal/artifact"
	"github.com/srnnkls/henia/internal/preload"
)

func Digest(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])[:12]
}

var headCommand = regexp.MustCompile(`^henia (show (\S+) --(head|toc)|context (\S+))$`)

func HeadCommand(name, command string) bool {
	m := headCommand.FindStringSubmatch(command)
	return m != nil && (m[2] == name || m[4] == name)
}

func hybridHead(head, canonical *artifact.Artifact, native bool) {
	name := canonical.Name
	head.Resources = nil
	if native {
		head.Body = fmt.Sprintf("!`henia show %s --head`\n", name)
		return
	}
	for _, command := range []string{"henia show " + name + " --toc", "henia context " + name} {
		if !slices.ContainsFunc(preload.Find([]byte(head.Body)), func(p preload.Preload) bool { return p.Command == command }) {
			head.Body = strings.TrimRight(head.Body, "\n") + "\n\n!`" + command + "`\n"
		}
	}
}
