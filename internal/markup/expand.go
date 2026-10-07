package markup

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	TemplateAttr = "template"
	HeadBlock    = "head"
)

type TemplateUse struct {
	Name     string
	Offset   int
	Explicit bool
}

func Expand(source string, shared func(name string) bool, expand func(template string, args map[string]string, content string) (string, error)) (string, error) {
	if !strings.Contains(source, ":::") || shared == nil && !strings.Contains(source, TemplateAttr) {
		return source, nil
	}
	if shared == nil {
		shared = func(string) bool { return false }
	}
	root, _ := Tree([]byte(source))
	var found []*Element
	var visit func(*Element)
	visit = func(e *Element) {
		if e.Type == "directive" && e.Attrs["inline"] != "true" && (e.Attrs[TemplateAttr] != "" || shared(e.Attrs["name"])) {
			found = append(found, e)
			return
		}
		for _, child := range e.Children {
			visit(child)
		}
	}
	visit(root)
	var out strings.Builder
	last := 0
	for _, e := range found {
		block := source[e.Start:e.End]
		open, rest, _ := strings.Cut(block, "\n")
		content, fence := "", rest
		if i := strings.LastIndexByte(rest, '\n'); i >= 0 {
			content, fence = rest[:i+1], rest[i+1:]
		}
		head, args := open, map[string]string{}
		name := e.Attrs["name"]
		if brace := strings.IndexByte(open, '{'); brace >= 0 {
			attrs, _, err := attributes([]byte(open[brace:]))
			if err != nil {
				return "", fmt.Errorf("directive %s: %w", e.Attrs["name"], err)
			}
			head = strings.TrimRight(open[:brace], " ")
			for _, a := range attrs {
				switch a.name {
				case TemplateAttr:
					name = a.value
				case "class":
					for _, flag := range strings.Fields(a.value) {
						args[flag] = "true"
					}
				default:
					args[a.name] = a.value
				}
			}
		}
		content, err := Expand(content, shared, expand)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(content) == "" {
			content = ""
		}
		rendered, err := expand(name, args, content)
		if err != nil {
			return "", fmt.Errorf("directive %s: %w", e.Attrs["name"], err)
		}
		if rendered != "" && !strings.HasSuffix(rendered, "\n") {
			rendered += "\n"
		}
		out.WriteString(source[last:e.Start])
		if e.Attrs["name"] == HeadBlock {
			out.WriteString(head)
			out.WriteString("\n")
			out.WriteString(rendered)
			out.WriteString(fence)
		} else {
			out.WriteString(strings.TrimSuffix(rendered, "\n"))
		}
		last = e.End
	}
	out.WriteString(source[last:])
	return out.String(), nil
}

func TemplateDirectives(source []byte) []TemplateUse {
	if !bytes.Contains(source, []byte(":::")) {
		return nil
	}
	var uses []TemplateUse
	root, _ := Tree(source)
	root.Walk(func(e *Element) bool {
		if e.Type == "directive" && e.Attrs["inline"] != "true" {
			if name := e.Attrs[TemplateAttr]; name != "" {
				uses = append(uses, TemplateUse{name, e.Start, true})
			} else {
				uses = append(uses, TemplateUse{e.Attrs["name"], e.Start, false})
			}
		}
		return true
	})
	return uses
}
