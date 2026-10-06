# Paragraph similarity

Henia finds repeated paragraphs three ways: exact copies of the normalized
Markdown, shared word shingles, and closeness under local Model2Vec embeddings.
The [lint rules](lint-rules.md#similarity) use all three, and
[henia query](query.md) exposes the last two as `near`, `overlap` and `similar`.

Two paragraphs that say nearly the same thing:

```bash
git init review-skills && cd review-skills
mkdir -p .henia/skills/review
cat > .henia/skills/review/SKILL.md <<'EOF'
---
name: review
description: Review changed code.
---

# Review

Inspect every changed function and report concrete failures with enough context to reproduce the problem.

## Checklist

Inspect every changed function and report each concrete failure with enough context to reproduce it.
EOF
```

```console
$ henia query '(paragraph :node ?n :text ?t) @a (paragraph :node (after ?n) :text (near ?t 0.3 ?s)) @b' --sort '-?s'
?s=0.3684210526315789
@a  review#review  L3  paragraph  Inspect every changed function and report concrete failures with enough context…
@b  review#checklist  L7  paragraph  Inspect every changed function and report each concrete failure with enough con…
$ henia query '(paragraph :node ?n :text ?t) @a (paragraph :node (after ?n) :text (overlap ?t 0.3 ?s)) @b' --sort '-?s'
?s=0.5384615384615384
@a  review#review  L3  paragraph  Inspect every changed function and report concrete failures with enough context…
@b  review#checklist  L7  paragraph  Inspect every changed function and report each concrete failure with enough con…
```

## Lexical measures

`near` is the Jaccard similarity of the two paragraphs' three-word shingles,
following the [shingling approach in Introduction to Information
Retrieval](https://nlp.stanford.edu/IR-book/html/htmledition/near-duplicates-and-shingling-1.html).
`overlap` is containment: the share of the smaller shingle set that the other
paragraph holds, which catches a paragraph copied into a longer one. Henia
intersects the shingle sets exactly, with no MinHash approximation. The book's
thresholds are tuned for web pages and are not Henia's paragraph defaults.

## Local model evidence

A smoke check ran the in-process backend on local weights, with no service and
no download:

- Model: `minishlab/potion-retrieval-32M`
- Snapshot: `6fc8051fab2a1e0ee76689cf08c853792ac285e7`
- Dimensions: 512
- Runtime: `github.com/townsendmerino/aikit/embed` v1.16.0, pure Go

Anchor paragraph:

> Inspect every changed function and report concrete failures with enough context to reproduce the problem.

| Comparison | Cosine with anchor |
|---|---:|
| “Examine modified routines, identify reproducible defects, and explain each issue clearly so another developer can investigate it.” | 0.3832 |
| “Prepare the vegetable broth by simmering chopped carrots and onions together in a large covered pot.” | -0.0973 |
| “Do not inspect changed functions or report failures, and never provide steps to reproduce the problem.” | 0.7039 |

Three pairs make a smoke test, not a benchmark. They show real local inference
separating a paraphrase from unrelated text, and they show the weakness: the
negated sentence scores highest. A threshold of 0.85 would miss the paraphrase
here. Thresholds do not carry over between embedding models, or from lexical
measures to cosine similarity.

## Reproduce

Point the optional test at a model directory holding the three Model2Vec files:

```bash
HENIA_TEST_MODEL=/absolute/path/to/model go test ./internal/lint -run TestSemanticPretrainedModel -v
```

Without `HENIA_TEST_MODEL` the test skips; it never downloads weights. The
ordinary tests use a tiny synthetic Model2Vec fixture that exercises the real
loader, tokenizer, pooling and diagnostics. It tests the integration, not
linguistic quality.

Lexical regression cases cover changed words, reordered sentences, containment
in both directions, unrelated prose, low-overlap paraphrases, Unicode, stable
ties, thresholds and exact-match precedence. Semantic cases cover model loading,
zero vectors, malformed files, cosine normalization, cancellation,
disabled-model loading and suppression of redundant diagnostics.

To tune thresholds for your own skills, keep accepted and rejected paragraph
pairs side by side and tune each measure separately. Among the rejected pairs,
include negation, changed tool restrictions, numbers, shared boilerplate and
short instructions. A semantic finding stays a review suggestion however high
it scores.
