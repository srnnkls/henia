---
description: Frontmatter, templates and directives that skills, commands and agents must get right.
params:
  max-age-days: 180
---

# Metadata

## metadata

Skills, commands and agents need a `name` and a `description`, frontmatter
that parses, and a `last_verified` date in `YYYY-MM-DD` form when they carry
one.

```hq
(rule metadata
  :severity error
  :message "{@p.message}"
  (file (problem :kind frontmatter) @p))

(rule metadata
  :message "frontmatter requires a nonempty string name"
  :at @head @file
  (file :kind /^(skill|command|agent)$/ (frontmatter)? @head
    (not (problem :kind frontmatter))
    (not (entry :key "name" :tag "!!str" :value /\S/))) @file)

(rule metadata
  :message "frontmatter requires a nonempty string description"
  :at @head @file
  (file :kind /^(skill|command|agent)$/ (frontmatter)? @head
    (not (problem :kind frontmatter))
    (not (entry :key "description" :tag "!!str" :value /\S/))) @file)

(rule metadata
  :message "last_verified must be a YYYY-MM-DD date"
  (frontmatter (entry :key "last_verified" :value (not /^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/))) @head)
```

### Matches

```md
---
name: example
---
# Example
```

### Passes

```md
---
name: example
description: An example skill.
last_verified: 2026-01-31
---
# Example
```

## stale-review

Content verified long ago may describe tools and behavior that changed since.

```hq
(rule stale-review
  :message "last verified {@date.value}; review references older than {$max-age-days} days"
  :at @head
  (frontmatter (entry :key "last_verified" :value (older $max-age-days)) @date) @head)
```

### Matches

```md
---
name: example
description: An example skill.
last_verified: 2001-01-01
---
# Example
```

### Passes

```md
---
name: example
description: An example skill.
---
# Example
```

## invalid-template

A Go template that does not parse breaks every render of the artifact.

```hq
(rule invalid-template
  :severity error
  :message "{@p.message}"
  (file (problem :kind template) @p))
```

### Matches

```md
---
name: example
description: An example skill.
---
{{if .flavor}}unclosed
```

### Passes

```md
---
name: example
description: An example skill.
---
{{if .flavor}}closed{{end}}
```

## invalid-markup

A directive that does not parse breaks rendering for every harness.

```hq
(rule invalid-markup
  :severity error
  :message "{@p.message}"
  (file (problem :kind markup) @p))
```

### Matches

```md
:::note
unclosed
```

### Passes

```md
:::note
closed
:::
```
