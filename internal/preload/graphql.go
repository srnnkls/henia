package preload

import (
	"errors"
	"fmt"
)

func graphQLMutation(document string) (bool, error) {
	braces, parens := 0, 0
	expectDefinition, definitions := true, 0
	for i := 0; i < len(document); {
		c := document[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			i++
		case c == '#':
			for i < len(document) && document[i] != '\n' {
				i++
			}
		case c == '"':
			end, err := graphQLString(document, i)
			if err != nil {
				return false, err
			}
			i = end
		case c == '{':
			if braces == 0 && parens == 0 && expectDefinition {
				expectDefinition = false
				definitions++
			}
			braces++
			i++
		case c == '}':
			braces--
			if braces < 0 {
				return false, errors.New("unbalanced braces")
			}
			if braces == 0 && parens == 0 {
				expectDefinition = true
			}
			i++
		case c == '(':
			parens++
			i++
		case c == ')':
			parens--
			if parens < 0 {
				return false, errors.New("unbalanced parentheses")
			}
			i++
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			start := i
			for i < len(document) && (document[i] == '_' || document[i] >= 'a' && document[i] <= 'z' || document[i] >= 'A' && document[i] <= 'Z' || document[i] >= '0' && document[i] <= '9') {
				i++
			}
			if braces == 0 && parens == 0 && expectDefinition {
				switch name := document[start:i]; name {
				case "mutation":
					return true, nil
				case "query", "subscription", "fragment":
					expectDefinition = false
					definitions++
				default:
					return false, fmt.Errorf("unexpected %q at the top level", name)
				}
			}
		default:
			i++
		}
	}
	switch {
	case braces != 0 || parens != 0:
		return false, errors.New("unbalanced document")
	case definitions == 0:
		return false, errors.New("empty document")
	}
	return false, nil
}

func graphQLString(document string, start int) (int, error) {
	if len(document) >= start+3 && document[start:start+3] == `"""` {
		for i := start + 3; i+3 <= len(document); i++ {
			if document[i] == '\\' && i+4 <= len(document) && document[i+1:i+4] == `"""` {
				i += 3
				continue
			}
			if document[i:i+3] == `"""` {
				return i + 3, nil
			}
		}
		return 0, errors.New("unterminated block string")
	}
	for i := start + 1; i < len(document); i++ {
		switch document[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		case '\n':
			return 0, errors.New("unterminated string")
		}
	}
	return 0, errors.New("unterminated string")
}
