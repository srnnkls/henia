---
description: Slot declarations, providers and applications in skills.
---

# Slots

## invalid-slot

`henia.slots` and `henia.provides` entries and `:slot[...]` directives must
parse, and one slot must not be declared with different types.

```hq
(rule invalid-slot
  :severity error
  :message "{@s.error}"
  (slot :error /./) @s)

(rule invalid-slot
  :severity error
  :message "slot {?name} is declared with conflicting types {?type}, {@other.type}"
  :at @slot
  (slot :role declare :slot ?name :type ?type) @slot
  (slot :role declare :slot ?name :type (after ?type)) @other)

(rule invalid-slot
  :severity error
  :message "slot {?name} is declared with conflicting types {@other.type}, {?type}"
  :at @slot
  (slot :role declare :slot ?name :type ?type) @slot
  (slot :role declare :slot ?name :type (before ?type)) @other)
```

### Matches

```md
---
name: example
description: An example skill.
henia:
  slots:
    code.style: bogus(
---
# Example
```

### Passes

```md
---
name: example
description: An example skill.
henia:
  slots:
    code.style:
---
# Example
```

## unknown-slot

A provided or applied slot must lie within a slot that a scanned skill or a
skill package declares, so `code.style.go` needs `code.style` or
`code.style.go` declared.

```hq
(rule unknown-slot
  :severity error
  :message "\"{?name}\" is not declared by any scanned document"
  (slot :role /^(provide|apply)$/ :slot ?name) @ref
  (not (slot :role declare :slot (covers ?name))))
```

### Matches

```md
---
name: example
description: An example skill.
henia:
  provides:
    code.style.go:
---
# Example
```

```md
---
name: example
description: An example skill.
---
# Example

:slot[git.commits]
```

### Passes

```md
---
name: example
description: An example skill.
henia:
  slots:
    code.style:
  provides:
    code.style.go:
---
# Example
```
