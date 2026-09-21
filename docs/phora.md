# Henia as a Phora compiler hook

Phora fetches and stages canonical inputs, including transitive dependencies.
Its ordinary `post_sync` hook runs `henia build`, then invokes Phora in a second
configuration directory to deploy the generated outputs. There is no source
`build` setting, generated Git package, or wrapper script.

```text
Phora source pins → canonical tree → Henia → harness trees → Phora targets
```

Run the repository-local example with both tools on PATH:

```bash
cd examples/phora/stage
phora sync
```

[The staging config](../examples/phora/stage/phora.toml) reads the committed
example inputs from this repository. Its hook calls Henia directly and chains
[the deployment config](../examples/phora/deploy/phora.toml) with `&&`, so a
compiler error prevents deployment. All generated paths stay under `.henia/`.
A global `post_sync` runs after pure removals too; `on_change` would miss them.

Enable `[build] clean = true` or pass `--clean` so a successful build removes
stale artifacts while preserving the previous output on failure. This setting
belongs to Henia. Phora owns installation paths, link ownership and pruning.
The deployment config declares one generated source per harness, bound to its target. Local generated directories use Phora's
native `deploy = "link"` mode and need no Git repository.

Phora's `pre_sync` also runs before source resolution. It can call Henia directly
when canonical inputs are already available locally. Use the two-phase form when
Phora must first acquire pinned inputs. The `--frozen --no-hooks` options replay
one phase without invoking compilers; generated links still require their build
directory. Rebuilding from cached canonical inputs recreates that directory.

Henia never reads deployment state or installs into harness home directories.
