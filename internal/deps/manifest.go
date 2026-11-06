package deps

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func Add(manifest, key string, d Dependency) error {
	if err := d.validate(key); err != nil {
		return err
	}
	declared, err := Read(manifest)
	if err != nil {
		return err
	}
	if _, ok := declared[key]; ok {
		return fmt.Errorf("dependency %s is already declared in %s", key, manifest)
	}
	data, err := os.ReadFile(manifest)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		b.WriteString("\n")
	}
	if len(data) > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "[dependencies.%s]\n", key)
	for _, field := range []struct{ key, value string }{{"host", d.Host}, {"repo", d.Repo}, {"git", d.Git}, {"path", d.Path}, {"branch", d.Branch}, {"tag", d.Tag}, {"rev", d.Rev}, {"root", d.Root}} {
		if field.value != "" {
			fmt.Fprintf(&b, "%s = %s\n", field.key, strconv.Quote(field.value))
		}
	}
	if len(d.Skills) > 0 {
		var quoted []string
		for _, skill := range d.Skills {
			quoted = append(quoted, strconv.Quote(skill))
		}
		fmt.Fprintf(&b, "skills = [%s]\n", strings.Join(quoted, ", "))
	}
	if d.Transitive {
		b.WriteString("transitive = true\n")
	}
	return os.WriteFile(manifest, []byte(b.String()), 0o644)
}

func Remove(manifest, key string) error {
	data, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	header := regexp.MustCompile(`(?m)^\[dependencies\.` + regexp.QuoteMeta(key) + `\][ \t]*(#.*)?\n`)
	location := header.FindIndex(data)
	if location == nil {
		if declared, err := Read(manifest); err == nil {
			if _, ok := declared[key]; ok {
				return fmt.Errorf("dependency %s is declared inline in %s; remove it there", key, manifest)
			}
		}
		return fmt.Errorf("no dependency %s in %s", key, manifest)
	}
	end := len(data)
	if next := regexp.MustCompile(`(?m)^\[`).FindIndex(data[location[1]:]); next != nil {
		end = location[1] + next[0]
	}
	head := strings.TrimRight(string(data[:location[0]]), "\n")
	rest := strings.TrimLeft(string(data[end:]), "\n")
	switch {
	case head != "" && rest != "":
		head += "\n\n"
	case head != "":
		head += "\n"
	}
	return os.WriteFile(manifest, []byte(head+rest), 0o644)
}

var forges = map[string]string{
	"github.com":    "github",
	"gitlab.com":    "gitlab",
	"codeberg.org":  "codeberg",
	"git.sr.ht":     "sr.ht",
	"bitbucket.org": "bitbucket",
}

// Forge reads the shorthands phora 0.5.0 add accepts; plain owner/repo names no host.
func Forge(shorthand string) (host, repo, root string, err error) {
	if strings.Contains(shorthand, "://") || strings.Contains(shorthand, "@") {
		return "", "", "", fmt.Errorf("%s is a URL, not a forge shorthand; pass it as git", shorthand)
	}
	host, rest, aliased := strings.Cut(shorthand, ":")
	if !aliased {
		host, rest = "", shorthand
	}
	if aliased && host == "" {
		return "", "", "", fmt.Errorf("%s names no forge before the colon", shorthand)
	}
	segments := strings.FieldsFunc(rest, func(r rune) bool { return r == '/' })
	if len(segments) > 0 && !aliased {
		if forge, ok := forges[segments[0]]; ok {
			host, segments = forge, segments[1:]
		}
	}
	if len(segments) < 2 {
		return "", "", "", fmt.Errorf("%s is not <owner>/<repo>, <host>:<owner>/<repo> or <domain>/<owner>/<repo>", shorthand)
	}
	return host, segments[0] + "/" + segments[1], strings.Join(segments[2:], "/"), nil
}
