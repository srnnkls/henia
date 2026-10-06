---
description: Links and references that point nowhere or at outdated names.
data:
  outdated: {}
  sigils: {"$": skill, "/": command, "@": agent, "#": file, "!": tool}
---

# References

## reference

A single-backtick code span names an artifact by its sigil: `$skill`,
`/command`, `@agent`, `!tool` and `#path`, where a file path holds a `.` or
`/`. `(reference @r ?kind ?name)` matches each one; lint and rendering read
references through it.

```hq
(define (reference @r ?kind ?name)
  [(code :ticks 1 :matches /^(?<sigil>[$\/@])(?<name>[a-z0-9][a-z0-9._-]*)$/)
   (code :ticks 1 :matches /^(?<sigil>!)(?<name>[a-zA-Z0-9._\/-]+)$/)
   (code :ticks 1 :matches /^(?<sigil>#)(?<name>[a-zA-Z0-9._\/-]*[.\/][a-zA-Z0-9._\/-]*)$/)] @r
  (row :table "sigils" :key ?sigil :value ?kind))
```

## broken-link

Local links, images and `henia show` references must name files and heading
anchors that exist. Targets whose anchors only exist once rendered are skipped.

```hq
(rule broken-link
  :message "invalid link {@l.dest}"
  (_ :valid "false") @l)

(rule broken-link
  :message "local reference does not exist: {@l.dest}"
  (_ :exists "false") @l)

(rule broken-link
  :severity error
  :message "{@l.problem}"
  (_ :problem /./) @l)

(rule broken-link
  :message "Markdown heading anchor does not exist: {@l.dest}"
  (_ :anchored "false") @l)
```

### Matches

```md
See [the guide](missing.md).
```

### Passes

```md
# Usage

See [usage](#usage).
```

## missing-reference

`$skill`, `/command` and `@agent` spans must name a scanned artifact, a skill
of a dependency source under `.henia/sources/`, or a command or agent built
into a configured harness.

```hq
(rule missing-reference
  :message "{@l.ref} reference \"{@l.name}\" is absent from scanned artifacts"
  (link :ref /^(skill|command|agent)$/ :artifact ?a) @l
  (not (file :artifact ?a))
  (not (row :table "builtin" :value ?a)))
```

### Matches

```md
---
name: example
description: An example skill.
---
Use `$missing`.
```

### Passes

```md
---
name: example
description: An example skill.
---
Use `$example`.
```

## outdated-reference

Names listed in the `outdated` table, mapped to their replacements, should not
appear in text, code spans or link destinations.

```hq
(rule outdated-reference
  :message "\"{@swap.key}\" is marked outdated; use \"{@swap.value}\""
  :at @text
  :focus ?old
  (row :table "outdated" :key ?old) @swap
  [(paragraph :text (contains ?old)) (heading :text (contains ?old)) (table :text (contains ?old))] @text)
```

### Matches

```md outdated.legacy-model=current-model
Use legacy-model here.
```

### Passes

```md outdated.legacy-model=current-model
Use current-model here.
```
