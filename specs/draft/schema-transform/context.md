# Implementation context

The user selected TOML + Expr in place of the earlier Starlark architecture.
The current normative contract is [spec.md](spec.md).

- `internal/canonical`: typed canonical fields and round-trip extra data.
- `internal/expression`: shared Expr compilation and evaluation.
- `internal/vendor`: TOML profiles, resolution, metadata and file computations.
- `internal/config`: layered configuration with legacy compatibility.
- `internal/markup`: Goldmark rendering and source-aware lint views.
- `internal/lint`: built-in checks and declarative custom rules.
- `internal/transform`: templates, metadata compilation, markup and references.
- `internal/target`, `internal/build`: preflight plans and bounded output writes.

Vendor behavior is documented with primary sources in
[the research matrix](../../../docs/vendor-research.md). Older Starlark resources
remain on the historical schema-transform branch and are not executable contracts.

Phora is the deployment authority. Henia reads local canonical inputs and emits
artifacts under one build output directory; its Go module does not import Phora.
