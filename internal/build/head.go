package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/srnnkls/henia/internal/artifact"
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
	head.Body += fmt.Sprintf("\n\n!`henia show %s --toc`\n\n!`henia context %s`\n", name, name)
}
