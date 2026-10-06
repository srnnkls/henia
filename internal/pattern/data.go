package pattern

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/srnnkls/henia/internal/markup"
)

type Table struct {
	Keys   []string
	Values []string
	List   bool
}

func ParseTable(raw any) (Table, error) {
	scalar := func(v any) (string, error) {
		switch v := v.(type) {
		case string:
			return v, nil
		case int, int64, float64, bool:
			return fmt.Sprint(v), nil
		}
		return "", fmt.Errorf("rows must be strings, numbers or booleans, not %T", v)
	}
	switch rows := raw.(type) {
	case map[string]any:
		t := Table{}
		for _, key := range slices.Sorted(maps.Keys(rows)) {
			value, err := scalar(rows[key])
			if err != nil {
				return Table{}, err
			}
			t.Keys, t.Values = append(t.Keys, key), append(t.Values, value)
		}
		return t, nil
	case []any:
		t := Table{List: true}
		for _, row := range rows {
			value, err := scalar(row)
			if err != nil {
				return Table{}, err
			}
			t.Values = append(t.Values, value)
		}
		return t, nil
	case []string:
		return Table{List: true, Values: rows}, nil
	}
	return Table{}, fmt.Errorf("must be a table or a list, not %T", raw)
}

func DataRoot(tables map[string]Table) *markup.Element {
	if len(tables) == 0 {
		return nil
	}
	root := &markup.Element{Type: "data", Attrs: map[string]string{}}
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		t := tables[name]
		for i, value := range t.Values {
			attrs := map[string]string{"table": name, "value": value}
			if t.List {
				attrs["index"] = strconv.Itoa(i)
			} else {
				attrs["key"] = t.Keys[i]
			}
			root.Children = append(root.Children, &markup.Element{Type: "row", Attrs: attrs, Parent: root, Index: len(root.Children)})
		}
	}
	return root
}
