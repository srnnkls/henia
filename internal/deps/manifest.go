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
	for _, field := range []struct{ key, value string }{{"git", d.Git}, {"path", d.Path}, {"branch", d.Branch}, {"tag", d.Tag}, {"rev", d.Rev}, {"root", d.Root}} {
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
