package pattern

import (
	"errors"
	"strings"
	"testing"

	"github.com/srnnkls/henia/internal/markup"
)

func tabled(t *testing.T) corpus {
	return newCorpus(t, map[string]map[string]string{
		"a": {"SKILL.md": "# A\n\n## Conventions\n\n| Axis | Rule |\n| --- | --- |\n| Errors | Wrap them. |\n\n## Invariants\n\n| Invariant | Check |\n| --- | --- |\n| Rows bound | `go test` |\n| Lines exact |  |\n"},
	}, nil)
}

func axes() *markup.Element {
	return DataRoot(map[string]Table{"axes": {List: true, Values: []string{"Errors", "Effects"}}})
}

func TestTableRowsAndCells(t *testing.T) {
	c := tabled(t)
	cases := []struct{ name, query, want string }{
		{"header rows", `(table (row :header true) @r)`, "r=| Axis | Rule |\nr=| Invariant | Check |"},
		{"empty cell by column", `(section :title "Invariants" (table (row (cell :column "Check" :text "")) @r))`, "r=| Lines exact |  |"},
		{"cells by index", `(row :index 1 (cell :index 0) @c)`, "c=Errors\nc=Rows bound"},
		{"data rows skip the header", `(row :header false (cell :column "Axis") @c)`, "c=Errors"},
		{"inline code in a cell", `(cell :column "Check" (code :inline true) @k)`, "k=go test"},
		{"row text", `(row :contains "wrap") @r`, "r=| Errors | Wrap them. |"},
		{"table text", `(table :matches /Rows bound/) @t`, "t=| Invariant | Check |\n| --- | --- |\n| Rows bound | `go test` |\n| Lines exact |  |"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(texts(run(t, c, tc.query)), "\n"); got != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestTableCellLines(t *testing.T) {
	c := tabled(t)
	rows := run(t, c, `(row (cell :column "Check" :text "") @c) @r`)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row, cell := rows[0].Cells[1].Elements[0], rows[0].Cells[0].Elements[0]
	if row.Line != 14 || row.Column != 1 || cell.Line != 14 || cell.Column != 18 {
		t.Errorf("row L%d:%d, cell L%d:%d, want L14:1 and L14:18", row.Line, row.Column, cell.Line, cell.Column)
	}
}

func TestDataRowsAndTableRowsStayApart(t *testing.T) {
	c := tabled(t)
	env := Environment{Resolve: c, Data: axes()}
	cases := []struct{ name, query, want string }{
		{"data rows carry a table", `(row :table "axes" :value ?v) @d (cell :text ?v) @c`, "d= c=Errors"},
		{"table rows carry a header", `(row :header false) @r`, "r=| Errors | Wrap them. |\nr=| Rows bound | `go test` |\nr=| Lines exact |  |"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Read(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := q.Run(c.root, env)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(texts(rows), "\n"); got != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestTableKeys(t *testing.T) {
	for _, tc := range []struct{ query, message, hint string }{
		{`(cell :colum "Axis")`, "cell has no key :colum", "did you mean column?"},
		{`(row :headr true)`, "row has no key :headr", "did you mean header?"},
	} {
		_, err := Read(tc.query)
		var pe *Error
		if !errors.As(err, &pe) || !strings.Contains(pe.Message, tc.message) || !strings.Contains(pe.Hint, tc.hint) {
			t.Errorf("%s: err = %v, want %q / %q", tc.query, err, tc.message, tc.hint)
		}
	}
}
