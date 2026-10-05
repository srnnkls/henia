---
description: Paragraphs that repeat each other in other words. Off until a threshold is set.
params:
  min-words: 12
  similarity: 0
  containment: 0
  threshold: 0
---

# Similarity

Each rule compares a paragraph of at least `min-words` words with the earliest
earlier paragraph above the threshold; exact copies are left to
`duplicate-content`. Set `similarity` (word-shingle Jaccard), `containment`
(shared shingles over the shorter paragraph) or `threshold` (cosine under the
`[lint.semantic]` model) under `[lint.config.<rule>]` to turn a check on.

```hq
(define (paragraph-at @p ?text ?norm ?position)
  (paragraph :text ?text :norm ?norm :position ?position :words (>= $min-words) :dynamic (not "true")) @p
  (not (paragraph :norm ?norm :position (before ?position) :words (>= $min-words) :dynamic (not "true"))))
```

## similar-content

```hq
(rule similar-content
  :message "paragraph has {?score.percent} word-shingle jaccard with {@original}; shared phrases: {shared}"
  :at @copy :related @original :score ?score :method jaccard :shared @copy @original
  (paragraph-at @original ?text ?norm ?position)
  (paragraph :text ?copy :text (near ?text $similarity ?score) :norm ?copy-norm :position ?at :position (after ?position)
    :words (>= $min-words) :dynamic (not "true")) @copy
  (not (paragraph :norm ?copy-norm :position (before ?at) :words (>= $min-words) :dynamic (not "true")))
  (not (paragraph :text (near ?copy $similarity) :position (before ?position) :words (>= $min-words) :dynamic (not "true"))))

(rule similar-content
  :message "paragraph has {?score.percent} word-shingle containment with {@original}; shared phrases: {shared}"
  :at @copy :related @original :score ?score :method containment :shared @copy @original
  (paragraph-at @original ?text ?norm ?position)
  (paragraph :text ?copy :text (overlap ?text $containment ?score) :norm ?copy-norm :position ?at :position (after ?position)
    :words (>= $min-words) :dynamic (not "true")) @copy
  (not (paragraph :norm ?copy-norm :position (before ?at) :words (>= $min-words) :dynamic (not "true")))
  (not (paragraph :text (near ?copy $similarity) :position (before ?at) :words (>= $min-words) :dynamic (not "true")))
  (not (paragraph :text (overlap ?copy $containment) :position (before ?position) :words (>= $min-words) :dynamic (not "true"))))
```

### Matches

```md min-words=4 similarity=0.5
Repair the broken program and review the changes.

Repair the broken program and review the fixes.
```

### Passes

```md min-words=4 similarity=0.5
Repair the broken program and review the changes.

Simmer the vegetable soup slowly over low heat.
```

## semantic-content

Needs `[lint.semantic]` with `enabled = true` and a `model_path`; paragraphs
that already match lexically are left out.

```hq
(rule semantic-content
  :message "paragraph may express similar instructions to {@original} (cosine {?score.3}); review meaning and constraints"
  :at @copy :related @original :score ?score :method cosine
  (paragraph-at @original ?text ?norm ?position)
  (paragraph :text ?copy :text (similar ?text $threshold ?score) :norm ?copy-norm :position ?at :position (after ?position)
    :words (>= $min-words) :dynamic (not "true")) @copy
  (not (paragraph :norm ?copy-norm :position (before ?at) :words (>= $min-words) :dynamic (not "true")))
  (not (paragraph :text (near ?copy $similarity) :position (before ?at) :words (>= $min-words) :dynamic (not "true")))
  (not (paragraph :text (overlap ?copy $containment) :position (before ?at) :words (>= $min-words) :dynamic (not "true")))
  (not (paragraph :text (similar ?copy $threshold) :position (before ?position) :words (>= $min-words) :dynamic (not "true"))))
```
