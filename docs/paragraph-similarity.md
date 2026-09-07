# Paragraph similarity evaluation

Henia checks three kinds of repetition: exact normalized Markdown, word-shingle
Jaccard/containment, and semantic candidates from local Model2Vec embeddings.
Paragraph edit distance is no longer used.

The lexical implementation follows the [shingling and Jaccard approach described
in Introduction to Information Retrieval](https://nlp.stanford.edu/IR-book/html/htmledition/near-duplicates-and-shingling-1.html).
Its web-page example thresholds are not adopted as paragraph defaults. Henia uses
exact shingle intersections, rather than a probabilistic MinHash approximation.
Containment adds coverage of the smaller shingle set for partial copies.

## Local model evidence

The in-process backend was exercised with the same model family used by Anax's
`src/embed.rs` and Memex's Potion backend. Anax uses
`minishlab/potion-retrieval-32M`; Memex also supports `minishlab/potion-base-8M`.
The reference check used existing local weights, without a service or download:

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

These are a small smoke evaluation, not a quality benchmark or threshold study.
They demonstrate real local inference and separation of a paraphrase from
unrelated content, and expose the negation limitation. A threshold of 0.85 would
miss the paraphrase in this example. Do not carry thresholds between embedding
models or from lexical measures to cosine similarity.

## Reproduce

Set the path to an installed model directory containing the three Model2Vec files:

```bash
HENIA_TEST_MODEL=/absolute/path/to/model go test ./internal/lint -run TestSemanticPretrainedModel -v
```

Ordinary tests use a tiny synthetic Model2Vec fixture to exercise the actual local
loader, tokenizer, pooling and diagnostic path. That fixture tests integration,
not linguistic quality. The optional pretrained test skips when no model path is
provided; it never downloads weights.

Lexical regression cases cover changed words, reordered sentences, containment in
both directions, unrelated prose, low-overlap paraphrases, Unicode, stable ties,
thresholds, and exact-match precedence. Semantic cases cover local model loading,
zero vectors, malformed files, cosine normalization, cancellation, disabled-model
loading, and suppression of redundant diagnostics.

For your own corpus, keep accepted and rejected paragraph pairs together and tune
the measures separately. Include negation, changed tool restrictions, numbers,
shared boilerplate and short instructions among the rejected equivalence cases.
Semantic findings remain review suggestions even when their score is high.
