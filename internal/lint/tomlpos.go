package lint

import (
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// keyLocator finds where a dotted henia.toml key is defined, searching the
// layers in precedence order.
type keyLocator struct {
	files []string
	keys  []map[string]Location
}

func newKeyLocator(files []string) *keyLocator {
	l := &keyLocator{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		l.files = append(l.files, path)
		l.keys = append(l.keys, tomlKeys(path, data))
	}
	return l
}

// locate returns the first definition of key, or of its nearest defined
// parent, in the highest-precedence layer that has one.
func (l *keyLocator) locate(key string) Location {
	for k := key; k != ""; k = parentKey(k) {
		for _, keys := range l.keys {
			if at, ok := keys[k]; ok {
				return at
			}
		}
	}
	if len(l.files) > 0 {
		return Location{Path: l.files[0], Line: 1, Column: 1}
	}
	return Location{}
}

func parentKey(key string) string {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return ""
	}
	return key[:i]
}

// tomlKeys maps every table header and key path of a document, and each of
// their parents, to its first definition. A document that fails to parse
// yields the keys before the error.
func tomlKeys(path string, data []byte) map[string]Location {
	keys := map[string]Location{}
	var p unstable.Parser
	p.Reset(data)
	var table []string
	for p.NextExpression() {
		expr := p.Expression()
		var parts []string
		var first *unstable.Node
		for it := expr.Key(); it.Next(); {
			node := it.Node()
			if first == nil {
				first = node
			}
			parts = append(parts, string(node.Data))
		}
		if first == nil {
			continue
		}
		switch expr.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = parts
		case unstable.KeyValue:
			parts = append(append([]string(nil), table...), parts...)
		default:
			continue
		}
		shape := p.Shape(first.Raw)
		at := Location{Path: path, Line: shape.Start.Line, Column: shape.Start.Column}
		for i := range parts {
			key := strings.Join(parts[:i+1], ".")
			if _, ok := keys[key]; !ok {
				keys[key] = at
			}
		}
	}
	return keys
}
