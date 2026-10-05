---
description: Slot declarations, providers and applications in skill metadata.
---

# Slots

## invalid-slot

`metadata.slots`, `metadata.provides` and `metadata.applies` entries must parse,
and one slot must not be declared with different types.

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
metadata:
  slots: "code.style:bogus("
---
# Example
```

### Passes

```md
---
name: example
description: An example skill.
metadata:
  slots: "code.style"
---
# Example
```

## unknown-slot

A provided or applied slot must lie within a slot some scanned skill declares,
so `code.style.go` needs `code.style` or `code.style.go` declared.

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
metadata:
  provides: "code.style.go"
---
# Example
```

### Passes

```md
---
name: example
description: An example skill.
metadata:
  slots: "code.style"
  provides: "code.style.go"
---
# Example
```
