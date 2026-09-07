# Implementation map

The historical CUE and proposed fenceddiv code sketches are superseded. The
following packages implement the contracts in [the spec](../spec.md):

| Package | Responsibility |
|---|---|
| `internal/artifact` | Discover local source artifacts; parse YAML and Markdown |
| `internal/canonical` | Preserve typed canonical metadata and unknown fields |
| `internal/transform` | Go templates, metadata mapping, markup, then references |
| `internal/markup` | Goldmark directive nodes, rendering and lint source views |
| `internal/reference` | Parse and rewrite harness-specific artifact/tool references |
| `internal/vendor` | TOML profile loading, Expr mappings and generated sidecars |
| `internal/build` | Plan local compilations, detect collisions, protect source files |
| `internal/target` | Artifact layout and bounded filesystem writes |
| `internal/lint` | Canonical quality checks and TOML + Expr rules |

## Compiler entry point

`build.Run(ctx, sources, output, harnesses)` accepts local source directories and
an explicit output root. Harness names determine subdirectories beneath that
root. It returns the number built, warnings and errors. It does not fetch, install,
invoke Phora or maintain deployment locks.

The CLI exposes `henia build` and `henia lint`. `[build].output` or `--output`
chooses artifact staging. Profiles and render formats remain compilation settings.

## Source preservation

`markup.Render` uses parsed node spans to replace directive syntax while retaining
ordinary Markdown. `markup.Inspect` supplies the same node interpretation and
source offsets to lint rules. Template masking permits canonical linting without
executing templates.

`target.OutputPaths` enumerates main files, resources and generated files before
writing. Build preflight resolves existing symlink ancestors for collision and
source-boundary checks. `os.Root` constrains actual writes; resource symlinks are
rejected. A later filesystem failure can still leave partial output.

See [vendor profiles](../../../../docs/vendor-profiles.md),
[lint rules](../../../../docs/lint-rules.md) and
[Phora integration](../../../../docs/phora.md).
