---
form: container
doc: Routes the skill's arguments.
attributes:
  ordered:
    type: boolean
    doc: Rows apply in order; the first match wins.
  protocol:
    type: boolean
    doc: Links the dispatch protocol.
body:
  kind: markdown
  children: [table, paragraph]
---
## Routes

{{if .ordered}}Apply the first row that matches `$ARGUMENTS`:

{{end}}::body
{{if .protocol}}
> **Protocol:** [../dispatch/protocol.md](../dispatch/protocol.md)
{{end}}
