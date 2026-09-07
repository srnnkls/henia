# Henia as a Phora compiler hook

Henia owns compilation and linting. Phora owns fetching sources, selecting files,
installation paths, deployment state, pruning and hook execution. The tools
communicate through local directories and process exit status.

```text
canonical sources → Henia build → per-harness artifacts → Phora deployment
```

Configure the compilation destination in `henia.toml`:

```toml
[build]
output = ".henia/build"

[harness.claude]
profile = "claude"
format = "xml"

[harness.codex]
profile = "codex"
format = "directives"
```

The artifacts are `.henia/build/claude/skills/...` and
`.henia/build/codex/skills/...`. Phora can consume each directory as a local source
and project it into `.claude` or `.agents` using its own target configuration.
Generated OpenAI sidecars travel with their skill directories.

## Hook phase order

Build before Phora resolves the compiled output as a source. In the inspected
Phora checkout, source resolution and projection precede `pre_sync` and
`pre_deploy`. Building in those phases does not refresh the source snapshot
already selected for that run.

The examples therefore use two phases, invoked from the Henia repository root:

```bash
# Phora stages canonical skills; its post_sync hook runs Henia.
phora --config examples/phora-build.toml sync && \
  phora --config examples/phora-deploy.toml sync
```

The first config's global `post_sync` hook runs after every sync, including pure
removals; `on_change` alone would miss that case. Both example configurations use
`.henia/` staging locations, including the demonstration deployment destinations.
Replace the latter with live harness directories in your own Phora configuration.
The examples require the Rust Phora configuration schema from `~/projects/phora`;
an older Go Phora binary may expose a different CLI and schema.

Henia's build directory is an artifact staging area. Rebuilds overwrite outputs
but currently do not remove stale files: clean that staging directory when
canonical skills or sidecar declarations are removed. Phora controls pruning at
the installation destination. Compiler failure returns a nonzero exit status so
Phora reports a failed hook and the `&&` chain skips the deployment phase.

The hook is an ordinary `henia build` command. Henia neither calls Phora nor reads
Phora's deployment registry or hook environment to choose installation locations.
