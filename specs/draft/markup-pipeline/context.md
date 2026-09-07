# Markup pipeline context

The normative contract is [spec.md](spec.md). Henia is a local compiler and
linter; Phora owns source acquisition, hook orchestration and deployment.

Body processing is Go templates → Goldmark directives → references. Metadata
uses typed canonical fields plus TOML + Expr vendor profiles. This replaces the
historical CUE preprocessing and Starlark proposals.

Compilation supports both directory skills with resources and legacy flat
canonical Markdown files. Each selected harness receives a variant beneath the
build output directory. A harness profile defines metadata and syntax, not an
installation location.

`internal/build` discovers local artifacts, transforms all selected variants,
preflights collisions and delegates bounded writes to `internal/target`.
`internal/markup` shares its Goldmark parsing with lint inspection. Tests cover
nested syntax, source spans, literal contexts and template composition.

See [implementation](resources/implementation.md),
[test coverage](resources/patterns/test-patterns.md) and the
[harness transform scope](../schema-transform/spec.md).
