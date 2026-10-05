package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/srnnkls/henia/internal/artifact"
)

func Digest(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])[:12]
}

var headCommand = regexp.MustCompile(`^henia (show (\S+) --(head|toc)( --digest [0-9a-f]+)?|context (\S+))$`)

func HeadCommand(name, command string) bool {
	m := headCommand.FindStringSubmatch(command)
	return m != nil && (m[2] == name || m[5] == name)
}

func hybridHead(head, canonical *artifact.Artifact, native bool) error {
	path := canonical.SourcePath
	if canonical.IsDirectory {
		path = filepath.Join(path, artifact.MainFileName(canonical.Type))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	name, digest := canonical.Name, Digest(data)
	head.Resources = nil
	if native {
		head.Body = fmt.Sprintf("!`henia show %s --head --digest %s`\n", name, digest)
		return nil
	}
	head.Body += fmt.Sprintf("\n\n!`henia show %s --toc --digest %s`\n\n!`henia context %s`\n", name, digest, name)
	return nil
}
