---
description: Content and names repeated across the scanned artifacts.
params:
  min-words: 12
---

# Duplicates

## duplicate-skill

Two artifacts of one kind cannot share a name; the later one shadows the first.

```hq
(rule duplicate-skill
  :message "duplicate {@dup.kind} name \"{@dup.name}\"; first defined in {@original.file}"
  :at @dup-head @dup
  :related @original-head @original
  (file :artifact ?a :position ?p (frontmatter)? @original-head) @original
  (file :artifact ?a :position (after ?p) (frontmatter)? @dup-head) @dup
  (not (file :artifact ?a :position (before ?p))))
```

## duplicate-content

A paragraph of at least `min-words` words copied verbatim elsewhere; Markdown
syntax counts, so equal labels with different destinations differ.

```hq
(rule duplicate-content
  :message "paragraph duplicates {@original}"
  :at @copy
  :related @original
  (paragraph :norm ?t :position ?p :words (>= $min-words) :dynamic (not "true")) @original
  (paragraph :norm ?t :position (after ?p)) @copy
  (not (paragraph :norm ?t :position (before ?p))))
```

### Matches

```md min-words=3
One two three four.

One two three four.
```

### Passes

```md min-words=3
One two three four.

Five six seven eight.
```
