# Compiler validation

Use behavior-level cases against the implemented packages. The previous examples
for CUE extraction and proposed fenceddiv APIs are superseded.

| Area | Required cases |
|---|---|
| Markup | Nested blocks/inline nodes, attributes, escapes, literal code, malformed input, source locations |
| Rendering | XML escaping, directive normalization, Markdown labels, preservation of surrounding Markdown |
| Templates | if/range, named composition, nested metadata, harness overrides, input immutability |
| Profiles | Vendor field/type/value mapping, sidecars, native overrides, unsupported behavior |
| Build | Multiple local sources/harnesses, filtering, output override, deterministic output, cancellation |
| Filesystem | Collisions, source overlap, symlink aliases, resource/sidecar conflicts, root escapes |
| Configuration | Build output precedence, relative paths, rejection of deployment settings |
| CLI | Build/lint only, diagnostics and exit codes, no fetched-source cache or deployment commands |

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
mise run test:integration
```

Scrut fixtures compile directly from local directories. They do not create Git
repositories or invoke deployment. Phora hook commands use the same CLI entry
point and output tree as ordinary builds.
