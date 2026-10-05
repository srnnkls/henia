---
description: Size and heading structure of skills, commands and agents.
params:
  max-lines: 500
---

# Structure

## large-skill

Long artifacts crowd the context window; move reference material into resources.

```hq
(rule large-skill
  :message "{@file.lines} lines exceeds {$max-lines}; consider moving reference material into resources"
  (file :kind /^(skill|command|agent)$/ :lines (> $max-lines)) @file)
```

### Matches

```md max-lines=2
---
name: example
description: An example skill.
---
one
two
three
```

### Passes

```md
---
name: example
description: An example skill.
---
# Short
```

## duplicate-heading

A heading repeated in one document makes its anchors ambiguous.

```hq
(define (repeated-heading @first @repeat ?text)
  (file :node ?f (heading :norm ?text :text /\S/ :node ?n) @first)
  (file :node ?f (heading :norm ?text :node (after ?n)) @repeat)
  (not (file :node ?f (heading :norm ?text :node (before ?n)))))

(rule duplicate-heading
  :message "heading repeated; first occurrence on line {@first.line}"
  :at @repeat
  :related @first
  (repeated-heading @first @repeat ?text))
```

### Matches

```md
# Usage

## Setup

## setup
```

### Passes

```md
# Usage

## Setup

## Teardown
```
