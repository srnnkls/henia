---
description: Shared templates that skills include and lay out.
---

# Templates

## unknown-template

A `{{template}}` call or `henia.layout` must name a template in
`.henia/templates/` or `$XDG_CONFIG_HOME/henia/templates/`, or a block the same
file defines.

```hq
(rule unknown-template
  :severity error
  :message "template \"{@t.name}\" is neither a shared template nor defined in this file"
  (template :role use :resolved false) @t)
```

### Matches

```md
---
name: example
description: An example skill.
---
{{template "missing" .}}
```

```md
---
name: example
description: An example skill.
henia:
  layout: missing
---
{{define "lead"}}Example.{{end}}
```

### Passes

```md
---
name: example
description: An example skill.
---
{{define "note"}}Note.{{end}}
{{template "note" .}}
```

## unused-template

A project template needs a scanned skill or another template that includes it,
lays out with it, or calls a block it defines.

```hq
(rule unused-template
  :severity warning
  :message "template {@t.name} is not used by any scanned skill or template"
  (template :role template :used false) @t)
```
