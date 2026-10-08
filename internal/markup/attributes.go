package markup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type attribute struct{ name, value string }

type problem struct {
	offset      int
	id, message string
}

func attributes(input []byte) ([]attribute, int, error) {
	attrs, consumed, problems, _ := tokenize(input)
	if len(problems) > 0 {
		return nil, 0, errors.New(problems[0].message)
	}
	return attrs, consumed, nil
}

func tokenize(input []byte) (attrs []attribute, consumed int, problems []problem, ok bool) {
	invalid := func(offset int, format string, args ...any) ([]attribute, int, []problem, bool) {
		return nil, 0, append(problems, problem{offset, "", fmt.Sprintf(format, args...)}), false
	}
	index := map[string]int{}
	add := func(at int, name, value string) {
		i, seen := index[name]
		switch {
		case seen && name == "class":
			attrs[i].value += " " + value
		case seen:
			problems = append(problems, problem{at, "duplicate-attribute", fmt.Sprintf("attribute %q is repeated", name)})
		default:
			index[name] = len(attrs)
			attrs = append(attrs, attribute{name, value})
		}
	}
	if len(input) == 0 || input[0] != '{' {
		return invalid(0, "invalid directive attributes")
	}
	for i := 1; ; {
		i = skipBlanks(input, i)
		if i >= len(input) || input[i] == '\n' || input[i] == '\r' {
			return invalid(i, "unclosed directive attributes")
		}
		at := i
		switch c := input[i]; {
		case c == '}':
			return attrs, i + 1, problems, true
		case c == '#' || c == '.':
			n := bareLength(input[i+1:])
			if n == 0 {
				return invalid(i, "missing %c value", c)
			}
			name := "id"
			if c == '.' {
				name = "class"
			}
			add(at, name, string(input[i+1:i+1+n]))
			i += 1 + n
		default:
			n := nameLength(input[i:])
			if n == 0 {
				return invalid(i, "invalid attribute name")
			}
			name := string(input[i : i+n])
			i += n
			value := "true"
			if j := skipBlanks(input, i); j < len(input) && input[j] == '=' {
				j = skipBlanks(input, j+1)
				v, length, err := attributeValue(input[j:])
				if err != nil {
					return invalid(j, "attribute %q: %v", name, err)
				}
				value, i = v, j+length
			}
			if err := validValue(value); err != nil {
				return invalid(at, "attribute %q %v", name, err)
			}
			add(at, name, value)
		}
		if i < len(input) && input[i] != '}' && input[i] != ' ' && input[i] != '\t' {
			return invalid(i, "unexpected %q in directive attributes", input[i])
		}
	}
}

func skipBlanks(input []byte, i int) int {
	for i < len(input) && (input[i] == ' ' || input[i] == '\t') {
		i++
	}
	return i
}

func bareLength(input []byte) int {
	n := bytes.IndexFunc(input, func(r rune) bool { return r <= ' ' || strings.ContainsRune("\"'=<>`{}[]", r) })
	if n < 0 {
		return len(input)
	}
	return n
}

func attributeValue(input []byte) (string, int, error) {
	if len(input) == 0 {
		return "", 0, fmt.Errorf("missing value")
	}
	switch input[0] {
	case '"':
		for i := 1; i < len(input) && input[i] != '\n'; i++ {
			switch input[i] {
			case '\\':
				i++
			case '"':
				var value string
				if err := json.Unmarshal(input[:i+1], &value); err != nil {
					return "", 0, fmt.Errorf("invalid quoted value")
				}
				return value, i + 1, nil
			}
		}
		return "", 0, fmt.Errorf("unclosed quoted value")
	case '\'':
		end := bytes.IndexAny(input[1:], "'\n")
		if end < 0 || input[1+end] != '\'' {
			return "", 0, fmt.Errorf("unclosed quoted value")
		}
		return string(input[1 : 1+end]), end + 2, nil
	}
	n := bareLength(input)
	if n == 0 {
		return "", 0, fmt.Errorf("missing value")
	}
	return string(input[:n]), n, nil
}

func validValue(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("is not valid UTF-8")
	}
	for _, r := range value {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' || r == 0xfffe || r == 0xffff {
			return fmt.Errorf("contains an invalid XML character")
		}
	}
	return nil
}
