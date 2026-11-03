package lint_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/srnnkls/henia/internal/lint"
)

const conventionsModule = `---
stage: source
data:
  axes: [Errors, Effects]
---

## missing-axis

A scope lists every axis in its Conventions table.

~~~hq
(rule missing-axis :message "Conventions has no {?axis} row" :at @s
  (row :table axes :value ?axis)
  (file :path ?p (frontmatter :issue_type /^(Initiative|Feature|Task)$/) (section :title "Conventions") @s)
  (not (file :path ?p (section :title "Conventions" (table (row (cell :column "Axis" :text ?axis)))))))
~~~

### Matches

~~~md
---
issue_type: Task
---

## Conventions

| Axis | Rule |
| --- | --- |
| Errors | Wrap them. |
~~~

### Passes

~~~md
---
issue_type: Task
---

## Conventions

| Axis | Rule |
| --- | --- |
| Errors | Wrap them. |
| Effects | Name them. |
~~~

## unchecked-invariant

Every invariant names its check.

~~~hq
(rule unchecked-invariant :message "invariant has no check" :at @r
  (section :title "Invariants" (table (row (cell :column "Check" :text "")) @r)))
~~~

### Matches

~~~md
## Invariants

| Invariant | Check |
| --- | --- |
| Lines exact |  |
~~~

### Passes

~~~md
## Invariants

| Invariant | Check |
| --- | --- |
| Lines exact | go test |
~~~
`

func TestTableRulesThroughNegation(t *testing.T) {
	root := t.TempDir()
	modules := filepath.Join(root, ".henia", "lint")
	write(t, modules, "team/conventions.md", conventionsModule)
	options := lint.Options{Modules: []lint.ModuleDir{{Dir: modules}}}
	plan, err := lint.Compile(options)
	if err != nil {
		t.Fatal(err)
	}
	failures, _, err := plan.Test()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range failures {
		t.Error(f)
	}
	partial := write(t, root, "scopes/draft/x/scope.md", "---\nissue_type: Task\n---\n\n# X\n\n## Conventions\n\n| Axis | Rule |\n| --- | --- |\n| Errors | Wrap them. |\n\n## Invariants\n\n| Invariant | Check |\n| --- | --- |\n| Rows bound | go test |\n| Lines exact |  |\n")
	complete := write(t, root, "scopes/draft/y/scope.md", "---\nissue_type: Feature\n---\n\n# Y\n\n## Conventions\n\n| Axis | Rule |\n| --- | --- |\n| Errors | Wrap them. |\n| Effects | Name them. |\n")
	diagnostics, err := lint.Run(t.Context(), []string{partial, complete}, options)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diagnostics {
		if d.Rule == "missing-axis" || d.Rule == "unchecked-invariant" {
			got = append(got, fmt.Sprintf("%s:%d:%d:%s", d.Path, d.Line, d.Column, d.Message))
		}
	}
	want := []string{
		partial + ":7:1:Conventions has no Effects row",
		partial + ":18:1:invariant has no check",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
